(() => {
  const markup = document.getElementById("level-markup");
  const form = document.getElementById("submit-form");
  if (!markup || !form) return;

  const levelObj = window.__LEVEL__ || window.__LEVEL__ || {};
  const md =
    levelObj.markup || markup.dataset.markup || markup.textContent || "";
  markup.innerHTML = marked.parse(md);
  markup.querySelectorAll("a").forEach((link) => {
    link.target = "_blank";
    link.rel = "noreferrer";
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const body = new URLSearchParams(new FormData(form));
    const qp = new URLSearchParams(window.location.search);
    const levelType = qp.get("type") || "cryptic";
    body.append("type", levelType);
    const { res: response, parsed } = await window.sudo.fetchWithCSRF(
      "/api/submit",
      { method: "POST", body },
    );
    const payload = parsed.json || (parsed.text ? { error: parsed.text } : {});
    if (!response.ok || payload.error) {
      if (window.sudoAudio) window.sudoAudio.playAttempt();
      window.sudo.flashMessage(
        "play-message",
        payload.error || "Could not submit answer.",
        "error",
      );
      window.sudo.toast(payload.error || "Could not submit answer.", "error");
      return;
    }
    if (payload.success) {
      const level = window.__LEVEL__ || {};
      const answerHash = level.answer_hash || level.AnswerHash || null;
      if (answerHash) {
        const normalized =
          new URLSearchParams(new FormData(form)).get("answer") || "";
        const normalizedLower = normalized.trim().toLowerCase();
        const buf = new TextEncoder().encode(normalizedLower);
        const digest = await crypto.subtle.digest("SHA-256", buf);
        const hashArray = Array.from(new Uint8Array(digest));
        const hashHex = hashArray
          .map((b) => b.toString(16).padStart(2, "0"))
          .join("");
        if (hashHex !== answerHash) {
          if (window.sudoAudio) window.sudoAudio.playAttempt();
          window.sudo.flashMessage(
            "play-message",
            "Client verification failed for answer.",
            "error",
          );
          window.sudo.toast("Client verification failed for answer.", "error");
          return;
        }
      }
      window.sudo.flashMessage(
        "play-message",
        "Correct answer. Loading the next level...",
        "success",
      );
      window.sudo.toast("Correct answer. Loading the next level...", "success");
      if (window.sudoConfetti) window.sudoConfetti();
      setTimeout(() => window.location.reload(), 2500);      return;
    }
    if (window.sudoAudio) window.sudoAudio.playAttempt();
    window.sudo.flashMessage(
      "play-message",
      "Incorrect answer. Try again.",
      "error",
    );
    window.sudo.toast("Incorrect answer. Try again.", "error");
  });

  const chatToggle = document.getElementById("chatToggleBtn");
  const chatPopup = document.getElementById("chatPopupContainer");
  const chatClose = document.getElementById("chatPopupCloseBtn");
  if (chatToggle && chatPopup) {
    chatToggle.addEventListener("click", () =>
      chatPopup.classList.toggle("hidden"),
    );
  }
  if (chatClose && chatPopup) {
    chatClose.addEventListener("click", () =>
      chatPopup.classList.add("hidden"),
    );
  }

  const decor = document.getElementById("decorSvg");
  const headerEl = document.querySelector(".site-header");
  function updateDecor() {
    if (!decor || !headerEl) return;
    const headerRect = headerEl.getBoundingClientRect();
    const top = Math.max(8, headerRect.bottom + 8);
    decor.style.top = `${top}px`;
  }
  updateDecor();
  window.addEventListener("scroll", updateDecor, { passive: true });
  window.addEventListener("resize", updateDecor);
})();
