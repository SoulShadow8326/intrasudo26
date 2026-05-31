window.sudo = (() => {
  const audioAssets = {
    btn: new Audio("/assets/btn.mp3"),
    attempt: new Audio("/assets/attempt.mp3"),
    confetti: new Audio("/assets/confetti.mp3"),
  };
  audioAssets.btn.preload = "auto";
  audioAssets.attempt.preload = "auto";
  audioAssets.confetti.preload = "auto";

  function _play(a) {
    try {
      a.currentTime = 0;
      const p = a.play();
      if (p && p.catch) p.catch(() => {});
    } catch (e) {}
  }

  window.sudoAudio = {
    playButton() {
      _play(audioAssets.btn);
    },
    playAttempt() {
      _play(audioAssets.attempt);
    },
    playConfetti() {
      _play(audioAssets.confetti);
    },
    assets: audioAssets,
  };

  const panel = document.getElementById("announcements-panel");
  const dot = document.getElementById("announcements-dot");
  const toggle = document.getElementById("announcements-toggle");
  const notyf =
    typeof Notyf !== "undefined"
      ? new Notyf({
          duration: 2600,
          position: { x: "right", y: "top" },
          dismissible: true,
        })
      : null;

  function toggleAnnouncements(force) {
    if (!panel) return;
    const next =
      typeof force === "boolean" ? force : !panel.classList.contains("is-open");
    if (next) {
      panel.classList.add("is-open");
      if (dot) dot.classList.add("is-muted");
    } else {
      panel.classList.remove("is-open");
    }
  }

  function flashMessage(targetId, message, tone = "info") {
    const el = document.getElementById(targetId);
    if (!el) return;
    el.textContent = message;
    el.classList.remove("is-error", "is-success");
    if (tone === "error") el.classList.add("is-error");
    else if (tone === "success") el.classList.add("is-success");
  }

  function toast(message, tone = "success") {
    if (!message) return;
    if (notyf) {
      if (tone === "error") notyf.error(message);
      else notyf.success(message);
      return;
    }
    const fallback = document.querySelector("[id$='message']");
    if (fallback?.id) flashMessage(fallback.id, message, tone);
  }

  document.addEventListener("click", (event) => {
    if (!panel) return;
    const trigger = event.target.closest("button");
    if (panel.contains(event.target) || (trigger && trigger === toggle)) return;
    toggleAnnouncements(false);
  });

  if (toggle) {
    toggle.addEventListener("click", () => toggleAnnouncements());
  }

  const countdown = document.getElementById("countdown");
  if (countdown) {
    const target = Number(countdown.dataset.target) * 1000;
    const map = {
      days: document.getElementById("days"),
      hours: document.getElementById("hours"),
      minutes: document.getElementById("minutes"),
      seconds: document.getElementById("seconds"),
    };
    const tick = () => {
      const diff = Math.max(0, target - Date.now());
      const second = 1000;
      const minute = second * 60;
      const hour = minute * 60;
      const day = hour * 24;
      map.days.textContent = Math.floor(diff / day);
      map.hours.textContent = Math.floor((diff % day) / hour);
      map.minutes.textContent = Math.floor((diff % hour) / minute);
      map.seconds.textContent = Math.floor((diff % minute) / second);
    };
    tick();
    setInterval(tick, 1000);
  }

  const annList = document.getElementById("announcements-list");
  let lastChecksum = null;

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

  async function computeDOMChecksum() {
    if (!annList) return null;
    const items = Array.from(annList.querySelectorAll(".announcement-item"));
    const arr = items.map((item) => ({
      id: item.dataset.id || "",
      content: (item.querySelector("p")?.textContent || "").trim(),
      time: Number(item.dataset.time) || 0,
    }));
    const raw = JSON.stringify(arr);
    const buf = new TextEncoder().encode(raw);
    const digest = await crypto.subtle.digest("SHA-256", buf);
    const hashArray = Array.from(new Uint8Array(digest));
    return hashArray.map((b) => b.toString(16).padStart(2, "0")).join("");
  }

  const getCookie = (name) => {
    const m = document.cookie
      .split("; ")
      .find((row) => row.trim().startsWith(name + "="));
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

  const fetchWithCSRF = async (url, opts = {}) => {
    opts.headers = opts.headers || {};
    if (!opts.headers["X-CSRF-Token"] && !opts.headers["x-csrf-token"]) {
      const tk = getCookie("csrf");
      if (tk) opts.headers["X-CSRF-Token"] = tk;
    }
    const res = await fetch(url, opts);
    const parsed = await readResponse(res);
    return { res, parsed };
  };

  function initTransitions() {
    document.body.classList.add("page-loading");
    setTimeout(() => {
      document.body.classList.remove("page-loading");
    }, 700);

    document.addEventListener("click", (e) => {
      const link = e.target.closest("a");
      if (
        link &&
        link.href &&
        !link.target &&
        !link.hasAttribute("download") &&
        link.origin === window.location.origin &&
        !e.ctrlKey &&
        !e.metaKey &&
        !link.href.includes("#")
      ) {
        e.preventDefault();
        document.body.classList.add("is-leaving");
        setTimeout(() => {
          window.location.href = link.href;
        }, 800);
      }
    });
  }
  initTransitions();

  function renderAnnouncements(items) {
    if (!annList) return;
    if (!items || items.length === 0) {
      annList.innerHTML = '<p class="empty-state">No announcements yet.</p>';
      return;
    }
    annList.innerHTML = items
      .map((a) => {
        const t = new Date(Number(a.time) * 1000).toLocaleString();
        return `<article class="announcement-item" data-id="${escapeHtml(a.id)}" data-time="${escapeHtml(a.time)}"><p>${escapeHtml(a.content)}</p><p class="chat-message-meta">${escapeHtml(t)}</p></article>`;
      })
      .join("");
  }

  async function fetchAnnouncements() {
    if (!annList) return;
    try {
      let url = "/api/announcements";
      if (lastChecksum) url += "?checksum=" + encodeURIComponent(lastChecksum);
      const { res: resp, parsed } = await fetchWithCSRF(url, {
        cache: "no-store",
      });
      if (resp.status === 304) {
        const header = resp.headers.get("X-Announcements-Checksum");
        if (header) lastChecksum = header;
        return;
      }
      if (!resp.ok) return;
      const header = resp.headers.get("X-Announcements-Checksum");
      const payload = parsed.json || {};
      const items =
        payload && payload.announcements ? payload.announcements : [];
      renderAnnouncements(items);
      if (header) lastChecksum = header;
      if (dot) dot.classList.remove("is-muted");
    } catch (e) {}
  }

  (async () => {
    if (!annList) return;
    lastChecksum = await computeDOMChecksum();
    if (dot) dot.classList.add("is-muted");
    await fetchAnnouncements();
    setInterval(fetchAnnouncements, 15000);
    const annRoot = document.querySelector(".announcement");
    const headerEl = document.querySelector(".site-header");
    function updatePosition() {
      if (!annRoot) return;
      const headerRect = headerEl
        ? headerEl.getBoundingClientRect()
        : { bottom: 0 };
      const margin = 8;
      let top = margin;
      if (headerRect.bottom > margin)
        top = Math.max(margin, headerRect.bottom + margin);
      annRoot.classList.add("is-fixed");
      annRoot.style.top = top + "px";
    }
    let ticking = false;
    function onScrollOrResize() {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(() => {
        updatePosition();
        ticking = false;
      });
    }
    updatePosition();
    window.addEventListener("scroll", onScrollOrResize, { passive: true });
    window.addEventListener("resize", onScrollOrResize);
  })();

  return {
    toggleAnnouncements,
    flashMessage,
    toast,
    getCookie,
    readResponse,
    fetchWithCSRF,
  };
})();
