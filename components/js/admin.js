(() => {
  const levels = window.__LEVELS__ || [];
  const popup = document.getElementById("popupContainer");
  if (!popup) return;

  const displayBox = document.getElementById("displayBox");
  const inputField = document.getElementById("inputField");
  const levelTitle = document.getElementById("level_id");
  const sourceHintField = document.getElementById("sourceHintField");
  const answerField = document.getElementById("answerField");
  const levelIdField = document.getElementById("levelId");

  function sanitizeHtml(md) {
    return marked.parse(md || "");
  }

  function openPopupWith(level) {
    popup.classList.remove("hidden");
    fillPopup(level || {});
  }

  function closePopup() {
    popup.classList.add("hidden");
  }

  function fillPopup(level) {
    levelTitle.textContent = "Level " + (level.id || "");
    displayBox.innerHTML = sanitizeHtml(level.markup || "");
    inputField.value = level.markup || "";
    sourceHintField.value = level.source_hint || level.SourceHint || "";
    answerField.value = level.answer || "";
    levelIdField.value = (level.id || "").toString();
  }

  window.submitForm = async function submitForm() {
    const id = levelIdField.value.trim();
    const body = new URLSearchParams();
    if (id) body.append("level", id);
    body.append("markup", inputField.value || "");
    body.append("source", sourceHintField.value || "");
    body.append("answer", answerField.value || "");

    const resp = await fetch("/api/admin/levels", { method: "POST", body });
    const payload = await resp.json();
    if (!resp.ok || payload.error) {
      window.IntraSudo.toast(payload.error || "Could not save level.", "error");
      return;
    }
    window.IntraSudo.toast("Level saved. Reloading...", "success");
    setTimeout(() => window.location.reload(), 600);
  };

  window.deleteLevel = async function deleteLevel() {
    const id = levelIdField.value.trim();
    if (!id) {
      window.IntraSudo.toast("Choose a level first.", "error");
      return;
    }
    const resp = await fetch(`/api/admin/levels/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
    const payload = await resp.json();
    if (!resp.ok || payload.error) {
      window.IntraSudo.toast(
        payload.error || "Could not delete level.",
        "error",
      );
      return;
    }
    window.IntraSudo.toast("Level deleted. Reloading...", "success");
    setTimeout(() => window.location.reload(), 600);
  };

  function attachCardHandlers() {
    document.querySelectorAll(".level-card").forEach((card) => {
      card.addEventListener("click", () =>
        openPopupWith(JSON.parse(card.dataset.level)),
      );
    });
  }

  const newLevelBtn = document.getElementById("new-level-btn");
  if (newLevelBtn)
    newLevelBtn.addEventListener("click", () => openPopupWith({}));
  attachCardHandlers();

  inputField.addEventListener("input", () => {
    displayBox.innerHTML = sanitizeHtml(inputField.value || "");
  });

  window.closePopup = closePopup;

  window.submitAnnouncement = async function submitAnnouncement() {
    const id =
      (document.getElementById("announcementId") || {}).value?.trim() || "";
    const content =
      (document.getElementById("announcementContent") || {}).value?.trim() ||
      "";
    if (!content) {
      window.IntraSudo.toast("Content is required.", "error");
      return;
    }
    const body = new URLSearchParams();
    if (id) body.append("id", id);
    body.append("content", content);
    const resp = await fetch("/api/admin/announcements", {
      method: "POST",
      body,
    });
    const payload = await resp.json();
    if (!resp.ok || payload.error) {
      window.IntraSudo.toast(
        payload.error || "Could not save announcement.",
        "error",
      );
      return;
    }
    window.IntraSudo.toast("Announcement saved. Reloading...", "success");
    setTimeout(() => window.location.reload(), 600);
  };

  window.deleteAnnouncement = async function deleteAnnouncement(id) {
    if (!id) {
      window.IntraSudo.toast("Invalid announcement id.", "error");
      return;
    }
    const resp = await fetch(
      `/api/admin/announcements/${encodeURIComponent(id)}`,
      { method: "DELETE" },
    );
    const payload = await resp.json();
    if (!resp.ok || payload.error) {
      window.IntraSudo.toast(
        payload.error || "Could not delete announcement.",
        "error",
      );
      return;
    }
    window.IntraSudo.toast("Announcement deleted. Reloading...", "success");
    setTimeout(() => window.location.reload(), 600);
  };

  document.querySelectorAll(".delete-announcement").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const id = btn.dataset.id || "";
      if (!id) return;
      if (!confirm("Delete this announcement?")) return;
      deleteAnnouncement(id);
    });
  });
})();
