(() => {
  const storageKey = "intrasudo26.play.tour.done";
  if (window.location.pathname !== "/play") return;
  if (localStorage.getItem(storageKey) === "1") return;

  const waitFor = (predicate, timeoutMs = 4000) =>
    new Promise((resolve) => {
      const start = Date.now();
      const tick = () => {
        let ok = false;
        try {
          ok = Boolean(predicate());
        } catch (e) {
          ok = false;
        }
        if (ok) {
          resolve(true);
          return;
        }
        if (Date.now() - start >= timeoutMs) {
          resolve(false);
          return;
        }
        setTimeout(tick, 60);
      };
      tick();
    });

  const ensureChatClosed = () => {
    const popup = document.getElementById("chatPopupContainer");
    const toggle = document.getElementById("chatToggleBtn");
    if (popup) popup.classList.add("hidden");
    if (toggle) toggle.style.setProperty("display", "inline-flex", "important");
  };

  const ensureChatOpen = async () => {
    const popup = document.getElementById("chatPopupContainer");
    const toggle = document.getElementById("chatToggleBtn");
    if (!popup) return false;
    if (!popup.classList.contains("hidden")) return true;
    if (toggle) toggle.click();
    else popup.classList.remove("hidden");
    await waitFor(() => !popup.classList.contains("hidden"), 2000);
    return !popup.classList.contains("hidden");
  };

  const onReady = (fn) => {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", fn, { once: true });
      return;
    }
    fn();
  };

  onReady(async () => {
    await waitFor(
      () =>
        (window.driver &&
          window.driver.js &&
          typeof window.driver.js.driver === "function") ||
        (window.driverjs && typeof window.driverjs.driver === "function") ||
        typeof window.Driver === "function" ||
        typeof window.DriverJS === "function",
      5000,
    );
    const driverFactory =
      (window.driver &&
        window.driver.js &&
        typeof window.driver.js.driver === "function" &&
        window.driver.js.driver) ||
      (window.driverjs && typeof window.driverjs.driver === "function"
        ? window.driverjs.driver
        : null) ||
      (typeof window.Driver === "function" && window.Driver) ||
      (typeof window.DriverJS === "function" && window.DriverJS) ||
      null;
    if (!driverFactory) return;

    ensureChatClosed();

    let driverObj = null;

    const steps = [
      {
        popover: {
          title: "Hi",
          description:
            "Intra Sudo is Exun Clan's annual intra school cryptic hunt, which is an online treasure hunt.",
          side: "bottom",
          align: "center",
        },
      },
      {
        element: "h1.heading-xl",
        popover: {
          title: "Level",
          description:
            "This is the level number. It shows which level you are on.",
          side: "bottom",
          align: "center",
        },
      },
      {
        element: "#level-markup",
        popover: {
          title: "Puzzle",
          description: "This is the main puzzle you have to solve.",
          side: "top",
          align: "center",
        },
      },
      {
        element: ".send-row",
        popover: {
          title: "Answer",
          description: "Here is where you are supposed to answer.",
          side: "top",
          align: "center",
        },
      },
      {
        element: "#chatToggleBtn",
        popover: {
          title: "Chat",
          description:
            "You can open this to ask admins for leads and chat with them while you play.",
          side: "left",
          align: "center",
          onNextClick: async () => {
            await ensureChatOpen();
            if (driverObj) driverObj.moveNext();
          },
        },
      },
      {
        element: ".popup-content",
        onHighlightStarted: async () => {
          await ensureChatOpen();
        },
        popover: {
          title: "Chat Panel",
          description:
            "This is the chat panel where you can talk while solving.",
          side: "left",
          align: "center",
        },
      },
      {
        element: "#chat-tab-hints",
        onHighlightStarted: async () => {
          await ensureChatOpen();
        },
        popover: {
          title: "Hints",
          description:
            "Click Hints to see hints that are released globally to all users for this level.",
          side: "bottom",
          align: "center",
        },
      },
      {
        element: "#toolsContainer",
        popover: {
          title: "Useful Tools",
          description:
            "Here are some useful tools that may assist you in your journey—Google, ChatGPT, and the Cryptic Hunt resources.",
          side: "left",
          align: "center",
        },
      },
    ];

    const hasEl = (selector) => {
      try {
        return Boolean(document.querySelector(selector));
      } catch (e) {
        return false;
      }
    };

    const effectiveSteps = steps.filter((s) => {
      if (!s.element) return true;
      if (typeof s.element !== "string") return true;
      if (s.element === ".popup-content") return true;
      return hasEl(s.element);
    });

    if (effectiveSteps.length <= 1) {
      localStorage.setItem(storageKey, "1");
      return;
    }

    driverObj = driverFactory({
      animate: true,
      smoothScroll: true,
      allowClose: true,
      showProgress: true,
      stagePadding: 8,
      popoverClass: "sudo-tour",
      nextBtnText: "NEXT",
      prevBtnText: "BACK",
      doneBtnText: "DONE",
      onDestroyStarted: () => {
        if (!driverObj) return;
        if (!driverObj.hasNextStep() || confirm("Exit the tutorial?")) {
          localStorage.setItem(storageKey, "1");
          driverObj.destroy();
        }
      },
      onDestroyed: () => {
        localStorage.setItem(storageKey, "1");
      },
      steps: effectiveSteps,
    });

    driverObj.drive();
  });
})();
