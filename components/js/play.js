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
    if (window.sudoAudio) window.sudoAudio.playAttempt();
    const body = new URLSearchParams(new FormData(form));
    const { res: response, parsed } = await window.sudo.fetchWithCSRF(
      "/api/submit",
      { method: "POST", body },
    );
    const payload = parsed.json || (parsed.text ? { error: parsed.text } : {});
    if (!response.ok || payload.error) {
      window.sudo.flashMessage(
        "play-message",
        payload.error || "Could not submit answer.",
        "error",
      );
      window.sudo.toast(payload.error || "Could not submit answer.", "error");
      return;
    }
    if (payload.success) {
      window.sudo.flashMessage(
        "play-message",
        "Correct answer. Loading the next level...",
        "success",
      );
      window.sudo.toast("Correct answer. Loading the next level...", "success");
      if (window.sudoConfetti) window.sudoConfetti();
      setTimeout(() => window.location.reload(), 1200);
      return;
    }
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
