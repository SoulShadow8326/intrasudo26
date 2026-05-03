(() => {
  const toggle = document.getElementById("chatToggleBtn");
  const popup = document.getElementById("chatPopupContainer");
  const closeBtn = document.getElementById("chatPopupCloseBtn");
  const thread = document.getElementById("chat-thread");
  const form = document.getElementById("chat-form");
  const input = document.getElementById("chat-input");
  if (!toggle || !popup || !thread || !form || !input) return;

  let submitBtn = form.querySelector('button[type="submit"]');

  let userEmail = null;
  let lastChecksum = null;
  let pollTimer = null;
  let isOpen = !popup.classList.contains("hidden");
  const isPlayPage = window.location.pathname === "/play";
  let cooldownUntil = 0;
  const openInterval = 1500;
  const closedInterval = 10000;

  function renderMessages(messages) {
    if (!messages || messages.length === 0) {
      thread.innerHTML = '<p class="empty-state">No chat history yet.</p>';
      return;
    }
    thread.innerHTML = messages
      .map((msg) => {
        const time = new Date(Number(msg.time) * 1000).toLocaleString();
        const cls = msg.kind === "hint" ? "chat-hint" : "chat-message";
        return `<div class="${cls}"><p class="chat-message-body">${escapeHtml(msg.content)}</p><p class="chat-message-meta">${escapeHtml(msg.author)} • ${escapeHtml(time)}</p></div>`;
      })
      .join("");
    thread.scrollTop = thread.scrollHeight;
  }

  function escapeHtml(s) {
    return String(s).replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
  }

  async function loadMe() {
    try {
      const { res: r, parsed } = await window.sudo.fetchWithCSRF("/api/me", {
        cache: "no-store",
      });
      if (!r.ok) return;
      const p = parsed.json || {};
      userEmail = p.email || null;
      window.__userEmail = userEmail;
    } catch (e) {
      console.error("loadMe error:", e);
    }
  }

  function initialRender() {
    const initial = [
      ...(window.__HINTS__ || []),
      ...(window.__MESSAGES__ || []),
    ].sort((a, b) => a.time - b.time);
    renderMessages(initial);
  }

  async function pollOnce() {
    try {
      const qp = new URLSearchParams(window.location.search);
      const levelType = qp.get("type") || "cryptic";
      let url = "/api/chats/checksum?type=" + encodeURIComponent(levelType);
      if (lastChecksum) url += "?checksum=" + encodeURIComponent(lastChecksum);
      const { res: resp, parsed } = await window.sudo.fetchWithCSRF(url, {
        cache: "no-store",
      });
      if (resp.status === 304) {
        const header = resp.headers.get("X-Chats-Checksum");
        if (header) lastChecksum = header;
        return;
      }
      if (!resp.ok) return;
      const payload = parsed.json || {};
      const checksum =
        payload.checksum || resp.headers.get("X-Chats-Checksum") || null;
      if (checksum) lastChecksum = checksum;
      const chats = payload.chats || [];
      const hints = payload.hints || [];
      const combined = [...hints, ...chats].sort((a, b) => a.time - b.time);
      renderMessages(combined);
      if (!isOpen) showNotification();
      if (payload.leads === false) {
        disableInput();
      } else if (payload.leads) {
        enableInput();
      }
    } catch (e) {
      console.error("pollOnce error:", e);
    }
  }

  function startPolling() {
    if (pollTimer) clearInterval(pollTimer);
    const interval = isOpen ? openInterval : closedInterval;
    pollTimer = setInterval(pollOnce, interval);
  }

  function showNotification() {
    if (!toggle) return;
    if (!toggle.querySelector(".chat-dot")) {
      const d = document.createElement("span");
      d.className = "chat-dot";
      d.style.cssText =
        "position:absolute;right:8px;top:8px;width:10px;height:10px;border-radius:999px;background:#f43;z-index:1200";
      toggle.appendChild(d);
    }
  }

  function clearNotification() {
    const d = toggle.querySelector(".chat-dot");
    if (d) d.remove();
  }

  function disableInput() {
    input.disabled = true;
    const btn = submitBtn || form.querySelector('button[type="submit"]');
    if (btn) btn.disabled = true;
  }

  function enableInput() {
    input.disabled = false;
    const btn = submitBtn || form.querySelector('button[type="submit"]');
    if (btn) btn.disabled = false;
  }

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const content = (input.value || "").trim();
    if (!content) return;
    const now = Date.now();
    if (now < cooldownUntil) return;
    cooldownUntil = now + 5000;
    const optimistic = {
      id: "tmp-" + now,
      author: userEmail || "You",
      content,
      time: Math.floor(now / 1000),
      kind: "user",
    };
    thread.insertAdjacentHTML(
      "beforeend",
      `<div class="chat-message"><p class="chat-message-body">${escapeHtml(optimistic.content)}</p><p class="chat-message-meta">${escapeHtml(optimistic.author)} • ${escapeHtml(new Date(optimistic.time * 1000).toLocaleString())}</p></div>`,
    );
    thread.scrollTop = thread.scrollHeight;
    input.value = "";
    const body = new URLSearchParams();
    body.append("content", content);
    try {
      const { res: resp, parsed } = await window.sudo.fetchWithCSRF(
        "/api/messages",
        { method: "POST", body },
      );
      if (resp.ok) {
        const payload =
          parsed.json || (parsed.text ? { error: parsed.text } : null);
        if (!payload || payload.error) {
          window.sudo.toast(
            payload && payload.error
              ? payload.error
              : "Could not send message.",
            "error",
          );
        } else {
          await pollOnce();
        }
      } else {
        const parsedErr =
          parsed.json || (parsed.text ? { error: parsed.text } : null);
        const msg =
          parsedErr && parsedErr.error
            ? parsedErr.error
            : "Could not send message.";
        window.sudo.toast(msg, "error");
      }
    } catch (e) {
      console.error(e);
      window.sudo.toast("Could not send message.", "error");
    } finally {
      setTimeout(() => {
        if (Date.now() >= cooldownUntil) enableInput();
      }, 5000);
    }
  });

  const mo = new MutationObserver(() => {
    const open = !popup.classList.contains("hidden");
    if (open !== isOpen) {
      isOpen = open;
      if (isOpen) {
        clearNotification();
        pollOnce();
      }
      startPolling();
    }
    if (isPlayPage && toggle) {
      toggle.style.display = isOpen ? "none" : "inline-flex";
    }
  });
  mo.observe(popup, { attributes: true, attributeFilter: ["class"] });

  if (toggle)
    toggle.addEventListener("click", () => {
      setTimeout(() => {
        const open = !popup.classList.contains("hidden");
        if (open !== isOpen) {
          isOpen = open;
          if (isOpen) {
            clearNotification();
            pollOnce();
          }
          startPolling();
        }
        if (isPlayPage) {
          toggle.style.display = isOpen ? "none" : "inline-flex";
        }
      }, 0);
    });
  if (closeBtn)
    closeBtn.addEventListener("click", () => {
      setTimeout(() => {
        const open = !popup.classList.contains("hidden");
        if (open !== isOpen) {
          isOpen = open;
          startPolling();
        }
      }, 0);
    });

  (async () => {
    await loadMe();
    initialRender();
    await pollOnce();
    startPolling();
  })();
})();
