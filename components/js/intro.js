(function () {
  const introOverlay = document.getElementById("intro-overlay");
  const introVideo = document.getElementById("intro-video");
  const skipBtn = document.getElementById("intro-skip");

  if (!introOverlay || !introVideo) return;

  const hasSeenIntro = localStorage.getItem("hasSeenIntro");
  const isMobile = window.innerWidth <= 768;

  if (hasSeenIntro || isMobile) {
    introOverlay.style.display = "none";
    return;
  }

  function closeIntro() {
    introOverlay.classList.add("is-hidden");
    introVideo.pause();
    localStorage.setItem("hasSeenIntro", "true");
    setTimeout(() => {
      introOverlay.style.display = "none";
    }, 500);
  }

  if (skipBtn) {
    skipBtn.addEventListener("click", closeIntro);
  }

  introVideo.addEventListener("ended", () => {
    if (window.sudoAudio) window.sudoAudio.playConfetti();
    closeIntro();
  });

  const playPromise = introVideo.play();
  if (playPromise !== undefined) {
    playPromise.catch((error) => {
      console.log(
        "Autoplay prevented. Showing skip button for manual play or interaction.",
      );
      if (!introVideo.muted) {
        introVideo.muted = true;
        const mutedPlayPromise = introVideo.play();
        if (mutedPlayPromise !== undefined) {
          mutedPlayPromise.catch(() => {
            if (skipBtn) skipBtn.textContent = "Enter";
          });
        }
        return;
      }

      if (skipBtn) skipBtn.textContent = "Enter";
    });
  }
})();
