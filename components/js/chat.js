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
      const r = await fetch("/api/me", { cache: "no-store" });
      if (!r.ok) return;
      const p = await r.json();
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
      let url = "/api/chats/checksum";
      if (lastChecksum) url += "?checksum=" + encodeURIComponent(lastChecksum);
      const resp = await fetch(url, { cache: "no-store" });
      if (resp.status === 304) {
        const header = resp.headers.get("X-Chats-Checksum");
        if (header) lastChecksum = header;
        return;
      }
      if (!resp.ok) return;
      const payload = await resp.json();
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
      const resp = await fetch("/api/messages", { method: "POST", body });
      if (resp.ok) {
        let payload = null;
        try {
          payload = await resp.json();
        } catch (err) {
          console.error(
            "/api/messages: failed to parse JSON on ok response",
            err,
          );
          await pollOnce();
          return;
        }
        if (!payload || payload.error) {
          window.IntraSudo.toast(
            payload && payload.error
              ? payload.error
              : "Could not send message.",
            "error",
          );
        } else {
          await pollOnce();
        }
      } else {
        let msg = "Could not send message.";
        try {
          const parsed = await resp.json();
          if (parsed && parsed.error) msg = parsed.error;
          else msg = JSON.stringify(parsed);
        } catch (err) {
          try {
            const txt = await resp.text();
            if (txt) msg = txt;
          } catch (err2) {
            console.error(
              "/api/messages: failed to parse non-OK response",
              err,
              err2,
            );
          }
        }
        window.IntraSudo.toast(msg || "Could not send message.", "error");
      }
    } catch (e) {
      console.error(e);
      window.IntraSudo.toast("Could not send message.", "error");
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
