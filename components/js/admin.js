(() => {
  const duck = document.getElementById("duck-pet");
  if (duck) {
    let frame = 0;
    const totalFrames = 11;
    const cols = 3;
    const frameSize = 22;

    setInterval(() => {
      const col = frame % cols;
      const row = Math.floor(frame / cols);
      duck.style.backgroundPosition = `-${col * frameSize}px -${row * frameSize}px`;
      frame = (frame + 1) % totalFrames;
    }, 150);

    let posX = window.innerWidth - 100;
    let direction = -1;
    const speed = 1.5;

    function move() {
      posX += speed * direction;

      if (posX < 20) {
        direction = 1;
        duck.style.transform = "scale(3) scaleX(-1)";
      } else if (posX > window.innerWidth - 60) {
        direction = -1;
        duck.style.transform = "scale(3) scaleX(1)";
      }

      duck.style.left = `${posX}px`;
      requestAnimationFrame(move);
    }

    duck.style.right = 'auto';
    move();
  }

  const levels = window.__LEVELS__ || [];
  const popup = document.getElementById("popupContainer");
  if (!popup) return;

  const displayBox = document.getElementById("displayBox");
  const inputField = document.getElementById("inputField");
  const levelTitle = document.getElementById("level_id");
  const sourceHintField = document.getElementById("sourceHintField");
  const answerField = document.getElementById("answerField");
  const levelIdField = document.getElementById("levelId");
  const getCookie = (name) => {
    const m = document.cookie
      .split("; ")
      .find((row) => row.startsWith(name + "="));
    return m ? decodeURIComponent(m.split("=")[1]) : "";
  };
  const readResponse = async (res) => {
    const ct = res.headers.get("content-type") || "";
    if (ct.includes("application/json")) {
      try {
        const j = await res.json();
        return { json: j };
      } catch (e) {
        const t = await res.text();
        return { text: t };
      }
    }
    const t = await res.text();
    return { text: t };
  };
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
    if (id) {
      const ok = /^[A-Za-z0-9_-]+-\d+$/.test(id);
      if (!ok) {
        window.sudo.toast('Level id must be in format <type>-<n>', 'error');
        return;
      }
    }
    const body = new URLSearchParams();
    if (id) body.append("level", id);
    body.append("markup", inputField.value || "");
    body.append("source", sourceHintField.value || "");
    body.append("answer", answerField.value || "");

    const { res: resp, parsed } = await window.sudo.fetchWithCSRF(
      "/api/admin/levels",
      { method: "POST", body },
    );
    const payload = parsed.json || (parsed.text ? { error: parsed.text } : {});
    if (!resp.ok || payload.error) {
      window.sudo.toast(payload.error || "Could not save level.", "error");
      return;
    }
    window.sudo.toast("Level saved. Reloading...", "success");
    setTimeout(() => window.location.reload(), 600);
  };

  window.deleteLevel = async function deleteLevel() {
    const id = levelIdField.value.trim();
    if (!id) {
      window.sudo.toast("Choose a level first.", "error");
      return;
    }
    const { res: resp, parsed } = await window.sudo.fetchWithCSRF(
      `/api/admin/levels/${encodeURIComponent(id)}`,
      { method: "DELETE" },
    );
    const payload = parsed.json || (parsed.text ? { error: parsed.text } : {});
    if (!resp.ok || payload.error) {
      window.sudo.toast(payload.error || "Could not delete level.", "error");
      return;
    }
    window.sudo.toast("Level deleted. Reloading...", "success");
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
      window.sudo.toast("Content is required.", "error");
      return;
    }
    const body = new URLSearchParams();
    if (id) body.append("id", id);
    body.append("content", content);
    const { res: resp, parsed } = await window.sudo.fetchWithCSRF(
      "/api/admin/announcements",
      { method: "POST", body },
    );
    const payload = parsed.json || (parsed.text ? { error: parsed.text } : {});
    if (!resp.ok || payload.error) {
      window.sudo.toast(
        payload.error || "Could not save announcement.",
        "error",
      );
      return;
    }
    window.sudo.toast("Announcement saved. Reloading...", "success");
    setTimeout(() => window.location.reload(), 600);
  };

  window.deleteAnnouncement = async function deleteAnnouncement(id) {
    if (!id) {
      window.sudo.toast("Invalid announcement id.", "error");
      return;
    }
    const { res: resp, parsed } = await window.sudo.fetchWithCSRF(
      `/api/admin/announcements/${encodeURIComponent(id)}`,
      { method: "DELETE" },
    );
    const payload = parsed.json || (parsed.text ? { error: parsed.text } : {});
    if (!resp.ok || payload.error) {
      window.sudo.toast(
        payload.error || "Could not delete announcement.",
        "error",
      );
      return;
    }
    window.sudo.toast("Announcement deleted. Reloading...", "success");
    setTimeout(() => window.location.reload(), 600);
  };

  const confirmModal = document.getElementById("confirmDeleteModal");
  const confirmDeleteBtn = document.getElementById("confirmDeleteBtn");
  const cancelDeleteBtn = document.getElementById("cancelDeleteBtn");
  let pendingDeleteId = "";

  function openConfirmModal(id) {
    pendingDeleteId = id || "";
    const msg = document.getElementById("confirmDeleteMessage");
    msg.textContent = "Are you sure you want to delete this announcement?";
    confirmModal.classList.remove("hidden");
  }

  function closeConfirmModal() {
    pendingDeleteId = "";
    confirmModal.classList.add("hidden");
  }

  confirmDeleteBtn.addEventListener("click", async () => {
    if (!pendingDeleteId) return closeConfirmModal();
    await deleteAnnouncement(pendingDeleteId);
    closeConfirmModal();
  });

  cancelDeleteBtn.addEventListener("click", () => closeConfirmModal());

  document.querySelectorAll(".delete-announcement").forEach((btn) => {
    btn.addEventListener("click", (e) => {
      const id = btn.dataset.id || "";
      if (!id) return;
      openConfirmModal(id);
    });
  });
})();
