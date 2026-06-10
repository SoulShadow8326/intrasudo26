(() => {
  const toggle = document.getElementById("chatToggleBtn");
  const popup = document.getElementById("chatPopupContainer");
  const closeBtn = document.getElementById("chatPopupCloseBtn");
  const thread = document.getElementById("chat-thread");
  const form = document.getElementById("chat-form");
  const input = document.getElementById("chat-input");
  const leadsIndicator = document.getElementById("leads-indicator");
  const chatTab = document.getElementById("chat-tab-chat");
  const hintsTab = document.getElementById("chat-tab-hints");
  if (!toggle || !popup || !thread || !form || !input) return;

  let submitBtn = form.querySelector('button[type="submit"]');

  let userEmail = null;
  let lastChecksum = null;
  let hasSyncedOnce = false;
  let pollTimer = null;
  let isOpen = !popup.classList.contains("hidden");
  const isPlayPage = window.location.pathname === "/play";
  let leadsEnabled = form.dataset.leadsEnabled !== "false";
  let cooldownUntil = 0;
  let cooldownTimer = null;
  let sendingMessage = false;
  const openInterval = 1500;
  const closedInterval = 10000;
  const defaultInputPlaceholder = input.getAttribute("placeholder") || "";
  let activeThread = "chat";
  let cachedMessages = [];
  let cachedHints = [];
  let optimisticMessages = [];
  let stickToBottom = true;
  let messageOffset = 0;
  let hasMoreMessages = true;
  let isLoadingMore = false;

  function updateToggleVisualState() {
    if (!toggle) return;
    if (isOpen) {
      toggle.style.setProperty("display", "none", "important");
    } else {
      toggle.style.setProperty("display", "inline-flex", "important");
    }
    toggle.style.opacity = isOpen ? "0.35" : "1";
  }

  function renderMessages(messages, emptyMessage) {
    const distanceFromBottom =
      thread.scrollHeight - thread.scrollTop - thread.clientHeight;
    const keepBottom = stickToBottom || distanceFromBottom <= 20;
    if (!messages || messages.length === 0) {
      thread.innerHTML = `<p class="empty-state">${escapeHtml(emptyMessage)}</p>`;
      return;
    }
    thread.innerHTML = messages
      .map((msg, idx) => {
        const isOwn =
          msg.kind !== "hint" &&
          userEmail &&
          String(msg.author || "").toLowerCase() ===
            String(userEmail).toLowerCase();
        const prevMsg = idx > 0 ? messages[idx - 1] : null;
        const isSameAuthor = prevMsg && prevMsg.kind === msg.kind && prevMsg.author === msg.author;
        const hasReply = (msg.kind === "hint" && msg.reply_to) || (prevMsg && prevMsg.kind === "hint" && prevMsg.reply_to);
        const isGrouped = isSameAuthor && !hasReply;
        const cls =
          msg.kind === "hint"
            ? `chat-hint${isGrouped ? " is-grouped" : ""}`
            : `chat-message${isOwn ? " is-own" : ""}${isGrouped ? " is-grouped" : ""}`;
        const replyBlock = msg.kind === "hint" && msg.reply_to
          ? `<div class="chat-reply-ctx"><p class="chat-reply-body">${escapeHtml(msg.reply_to)}</p></div>`
          : "";
        return `<div class="${cls}">${replyBlock}<p class="chat-message-body">${escapeHtml(msg.content)}</p></div>`;
      })
      .join("");
    if (keepBottom) {
      thread.scrollTop = thread.scrollHeight;
    } else {
      const nextTop =
        thread.scrollHeight - thread.clientHeight - distanceFromBottom;
      thread.scrollTop = Math.max(0, nextTop);
    }
  }

  function setLeadsIndicator(isOn) {
    if (!leadsIndicator) return;
    leadsIndicator.textContent = isOn ? "Leads On" : "Leads Off";
    leadsIndicator.classList.toggle("is-off", !isOn);
  }

  function renderActiveThread() {
    const isHints = activeThread === "hints";
    thread.dataset.activeThread = activeThread;
    form.classList.toggle("is-hidden", isHints);

    if (chatTab) {
      chatTab.classList.toggle("is-active", !isHints);
      chatTab.setAttribute("aria-selected", String(!isHints));
    }
    if (hintsTab) {
      hintsTab.classList.toggle("is-active", isHints);
      hintsTab.setAttribute("aria-selected", String(isHints));
    }

    if (isHints) {
      renderMessages(cachedHints, "No hints for this level yet.");
      return;
    }
    renderMessages(cachedMessages, "No chat history yet.");
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
    cachedMessages = [...(window.__MESSAGES__ || [])].sort(
      (a, b) => a.time - b.time,
    );
    cachedHints = [...(window.__HINTS__ || [])].sort((a, b) => a.time - b.time);
    messageOffset = 0;
    hasMoreMessages = cachedMessages.length >= 20 || cachedHints.length >= 20;
    renderActiveThread();
  }

  async function loadMoreMessages() {
    if (isLoadingMore || !hasMoreMessages || activeThread !== "chat") return;
    isLoadingMore = true;
    try {
      const qp = new URLSearchParams(window.location.search);
      const levelType = qp.get("type") || "cryptic";
      const nextOffset = messageOffset + 20;
      let url = `/api/chats?type=${encodeURIComponent(levelType)}&limit=20&offset=${nextOffset}`;
      const { res: resp, parsed } = await window.sudo.fetchWithCSRF(url, {
        cache: "no-store",
      });
      if (!resp.ok) {
        isLoadingMore = false;
        return;
      }
      const payload = parsed.json || {};
      const newChats = (payload.chats || []).sort((a, b) => a.time - b.time);
      if (newChats.length < 20) {
        hasMoreMessages = false;
      }
      if (newChats.length > 0) {
        const scrollPrevious = thread.scrollHeight - thread.scrollTop;
        cachedMessages = [...newChats, ...cachedMessages];
        messageOffset = nextOffset;
        renderActiveThread();
        thread.scrollTop = thread.scrollHeight - scrollPrevious;
      }
    } catch (e) {
      console.error("loadMoreMessages error:", e);
    } finally {
      isLoadingMore = false;
    }
  }

  async function pollOnce() {
    try {
      const qp = new URLSearchParams(window.location.search);
      const levelType = qp.get("type") || "cryptic";
      let url = "/api/chats/checksum?type=" + encodeURIComponent(levelType);
      if (lastChecksum) url += "&checksum=" + encodeURIComponent(lastChecksum);
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
      const checksumChanged = checksum && checksum !== lastChecksum;
      if (checksum) lastChecksum = checksum;
      const chats = payload.chats || [];
      const hints = payload.hints || [];
      const serverMessages = [...chats].sort((a, b) => a.time - b.time);
      if (optimisticMessages.length > 0) {
        const remainingOptimistic = [];
        for (const opt of optimisticMessages) {
          const matched = serverMessages.some(
            (msg) =>
              String(msg.content || "") === String(opt.content || "") &&
              String(msg.author || "").toLowerCase() ===
                String(opt.author || "").toLowerCase() &&
              Math.abs(Number(msg.time || 0) - Number(opt.time || 0)) <= 15,
          );
          if (!matched) {
            remainingOptimistic.push(opt);
          }
        }
        optimisticMessages = remainingOptimistic;
      }
      cachedMessages = [...serverMessages, ...optimisticMessages].sort(
        (a, b) => a.time - b.time,
      );
      cachedHints = [...hints].sort((a, b) => a.time - b.time);
      renderActiveThread();
      if (hasSyncedOnce && checksumChanged && !isOpen) showNotification();
      hasSyncedOnce = true;
      if (payload.leads === false) {
        setLeadsIndicator(false);
        disableInput();
      } else if (payload.leads) {
        setLeadsIndicator(true);
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

  function getCooldownRemainingMs() {
    return Math.max(0, cooldownUntil - Date.now());
  }

  function updateComposerState() {
    const cooldownActive = getCooldownRemainingMs() > 0;
    const btn = submitBtn || form.querySelector('button[type="submit"]');

    input.disabled = !leadsEnabled;
    if (btn) {
      btn.disabled = !leadsEnabled || cooldownActive || sendingMessage;
      btn.classList.toggle("is-cooldown", cooldownActive);
      btn.classList.toggle("is-sending", sendingMessage);
    }

    if (!leadsEnabled) {
      input.placeholder = "Leads are closed";
      return;
    }

    if (sendingMessage) {
      input.placeholder = "Sending...";
      return;
    }

    if (cooldownActive) {
      input.placeholder = `Wait ${Math.max(1, Math.ceil(getCooldownRemainingMs() / 1000))}s to send`;
      return;
    }

    input.placeholder = defaultInputPlaceholder;
  }

  function startCooldown(durationMs) {
    cooldownUntil = Date.now() + durationMs;
    if (cooldownTimer) clearInterval(cooldownTimer);
    updateComposerState();
    cooldownTimer = setInterval(() => {
      if (Date.now() >= cooldownUntil) {
        clearInterval(cooldownTimer);
        cooldownTimer = null;
        cooldownUntil = 0;
        updateComposerState();
        return;
      }
      updateComposerState();
    }, 250);
  }

  function disableInput() {
    leadsEnabled = false;
    updateComposerState();
  }

  function enableInput() {
    leadsEnabled = true;
    updateComposerState();
  }

  setLeadsIndicator(leadsEnabled);
  updateComposerState();

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (sendingMessage || getCooldownRemainingMs() > 0) {
      updateComposerState();
      return;
    }
    const content = (input.value || "").trim();
    if (!content) return;
    const now = Date.now();
    sendingMessage = true;
    updateComposerState();
    const optimistic = {
      id: "tmp-" + now,
      author: userEmail || "You",
      content,
      time: Math.floor(now / 1000),
      kind: "user",
    };
    stickToBottom = true;
    cachedMessages.push(optimistic);
    optimisticMessages.push(optimistic);
    renderActiveThread();
    input.value = "";
    const body = new URLSearchParams();
    body.append("content", content);
    const qp = new URLSearchParams(window.location.search);
    body.append("type", qp.get("type") || "cryptic");
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
          startCooldown(5000);
          await pollOnce();
        }
      } else {
        const parsedErr =
          parsed.json || (parsed.text ? { error: parsed.text } : null);
        const msg =
          parsedErr && parsedErr.error
            ? parsedErr.error
            : "Could not send message.";
        cachedMessages = cachedMessages.filter(
          (msgItem) => msgItem.id !== optimistic.id,
        );
        optimisticMessages = optimisticMessages.filter(
          (msgItem) => msgItem.id !== optimistic.id,
        );
        renderActiveThread();
        if (resp.status === 429) {
          startCooldown(5000);
        } else {
          updateComposerState();
        }
        window.sudo.toast(msg, "error");
      }
    } catch (e) {
      console.error(e);
      cachedMessages = cachedMessages.filter(
        (msgItem) => msgItem.id !== optimistic.id,
      );
      optimisticMessages = optimisticMessages.filter(
        (msgItem) => msgItem.id !== optimistic.id,
      );
      renderActiveThread();
      updateComposerState();
      window.sudo.toast("Could not send message.", "error");
    } finally {
      sendingMessage = false;
      updateComposerState();
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
    if (toggle) {
      if (isOpen) toggle.style.setProperty("display", "none", "important");
      else toggle.style.setProperty("display", "inline-flex", "important");
    }
    if (isPlayPage) updateToggleVisualState();
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
        if (isPlayPage) updateToggleVisualState();
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

  if (chatTab) {
    chatTab.addEventListener("click", () => {
      if (activeThread === "chat") return;
      activeThread = "chat";
      renderActiveThread();
    });
  }
  if (hintsTab) {
    hintsTab.addEventListener("click", () => {
      if (activeThread === "hints") return;
      activeThread = "hints";
      renderActiveThread();
    });
  }

  thread.addEventListener("scroll", () => {
    const distanceFromBottom =
      thread.scrollHeight - thread.scrollTop - thread.clientHeight;
    stickToBottom = distanceFromBottom <= 20;
    if (thread.scrollTop < 50) {
      loadMoreMessages();
    }
  });

  (async () => {
    await loadMe();
    initialRender();
    updateComposerState();
    await pollOnce();
    startPolling();
    if (toggle) {
      if (isOpen) toggle.style.setProperty("display", "none", "important");
      else toggle.style.setProperty("display", "inline-flex", "important");
    }
    if (isPlayPage) updateToggleVisualState();
  })();
})();
