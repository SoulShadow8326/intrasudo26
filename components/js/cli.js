(() => {
  function init() {
    const overlay = document.getElementById("retro-cli-overlay");
    const cli = document.getElementById("retro-cli");
    const header = document.getElementById("cli-header");
    const body = document.getElementById("cli-body");
    const output = document.getElementById("cli-output");
    const input = document.getElementById("cli-input");
    const promptEl = document.getElementById("cli-prompt");

    if (!overlay || !cli || !header || !body || !output || !input || !promptEl) return;

    let isDragging = false;
    let currentX, currentY, initialX, initialY, xOffset = 0, yOffset = 0;

    header.addEventListener("mousedown", dragStart);
    document.addEventListener("mousemove", drag);
    document.addEventListener("mouseup", dragEnd);

    function dragStart(e) {
      initialX = e.clientX - xOffset;
      initialY = e.clientY - yOffset;
      if (e.target === header || header.contains(e.target)) isDragging = true;
    }

    function drag(e) {
      if (isDragging) {
        e.preventDefault();
        currentX = e.clientX - initialX;
        currentY = e.clientY - initialY;
        xOffset = currentX;
        yOffset = currentY;
        cli.style.transform = `translate3d(${currentX}px, ${currentY}px, 0)`;
      }
    }

    function dragEnd() {
      initialX = currentX;
      initialY = currentY;
      isDragging = false;
    }

    const seq = ["up", "up", "down", "down", "left", "right", "left", "right", "b", "a"];
    const keyMap = { "arrowup": "up", "arrowdown": "down", "arrowleft": "left", "arrowright": "right" };
    let pos = 0;
    window.addEventListener("keydown", (e) => {
      const key = e.key.toLowerCase();
      const normalizedKey = keyMap[key] || key;
      if (normalizedKey === seq[pos]) {
        pos++;
        if (pos === seq.length) {
          pos = 0;
          showCLI();
        }
      } else {
        pos = normalizedKey === seq[0] ? 1 : 0;
      }
    });

    function focusInput() {
      input.focus();
      const range = document.createRange();
      range.selectNodeContents(input);
      range.collapse(false);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    }

    body.addEventListener("click", focusInput);

    function showCLI() {
      overlay.style.display = "flex";
      overlay.setAttribute("aria-hidden", "false");
      output.innerHTML = "";
      const now = new Date();
      const dateStr = now.toDateString().replace(/^\w+ /, "") + " " + now.toLocaleTimeString("en-US", { hour12: false });
      appendOutput(`Last login: ${dateStr} on ttys000`, "last-login");
      focusInput();
    }

    function hideCLI() {
      overlay.style.display = "none";
      overlay.setAttribute("aria-hidden", "true");
    }

    overlay.addEventListener("mousedown", (e) => {
      if (e.target === overlay) hideCLI();
    });

    const FS = {
      type: "dir",
      entries: {
        secret: {
          type: "dir",
          entries: {
            "clue.txt": { type: "file", remote: true, path: "/assets/clue.txt" }
          }
        }
      }
    };
    let cwdParts = [];

    function getPromptText() {
      return (cwdParts.length === 0 ? "~" : cwdParts[cwdParts.length - 1]) + " %";
    }

    function resolvePath(pathStr) {
      if (!pathStr) return [...cwdParts];
      let parts = pathStr.startsWith("/") ? [] : [...cwdParts];
      const segments = pathStr.split("/").filter(Boolean);
      for (const seg of segments) {
        if (seg === ".") continue;
        if (seg === "..") {
          if (parts.length > 0) parts.pop();
        } else if (seg === "~") {
          parts = [];
        } else {
          parts.push(seg);
        }
      }
      return parts;
    }

    function getNode(parts) {
      let cur = FS;
      for (const p of parts) {
        if (!cur.entries || !cur.entries[p]) return null;
        cur = cur.entries[p];
      }
      return cur;
    }

    function appendOutput(text, className = "") {
      const el = document.createElement("div");
      if (className) el.className = className;
      el.textContent = text;
      output.appendChild(el);
      body.scrollTop = body.scrollHeight;
    }

    const handlers = {
      help: () => appendOutput("Available commands: help, ls, cd, cat, clear, sudo"),
      ls: (args) => {
        const parts = resolvePath(args[0]);
        const node = getNode(parts);
        if (!node) return appendOutput("ls: no such file or directory");
        if (node.type === "dir") {
          const entries = Object.keys(node.entries || {});
          if (entries.length > 0) appendOutput(entries.join("\t"));
        } else {
          appendOutput(parts[parts.length - 1]);
        }
      },
      cd: (args) => {
        const target = args[0];
        if (!target || target === "~") {
          cwdParts = [];
          promptEl.textContent = getPromptText();
          return;
        }
        const parts = resolvePath(target);
        const node = getNode(parts);
        if (node && node.type === "dir") {
          cwdParts = parts;
          promptEl.textContent = getPromptText();
        } else {
          appendOutput("cd: no such directory");
        }
      },
      cat: async (args) => {
        if (!args[0]) return appendOutput("cat: missing operand");
        const parts = resolvePath(args[0]);
        const node = getNode(parts);
        if (!node) return appendOutput("cat: no such file");
        if (node.type === "dir") return appendOutput(`cat: ${args[0]}: Is a directory`);
        if (node.remote) {
          try {
            const res = await fetch(node.path);
            appendOutput(res.ok ? await res.text() : "cat: failed to fetch file");
          } catch {
            appendOutput("cat: error fetching file");
          }
        } else {
          appendOutput(node.content || "");
        }
      },
      clear: () => { output.innerHTML = ""; },
      sudo: () => appendOutput("sudo: Permission denied."),
    };

    let history = [];
    let histPos = -1;

    input.addEventListener("keydown", async (e) => {
      if (e.key === "Enter") {
        e.preventDefault();
        const raw = input.textContent.trim();
        const currentPrompt = getPromptText();
        appendOutput(`${currentPrompt} ${raw}`, "history-line");
        if (raw) history.push(raw);
        histPos = history.length;
        input.textContent = "";
        
        const parts = raw.split(/\s+/);
        const cmd = parts[0];
        const args = parts.slice(1);
        
        if (handlers[cmd]) {
          await handlers[cmd](args);
        } else if (cmd) {
          appendOutput(cmd + ": command not found");
        }
        
        body.scrollTop = body.scrollHeight;
        focusInput();
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        if (histPos > 0) {
          histPos--;
          input.textContent = history[histPos] || "";
          focusInput();
        }
      } else if (e.key === "ArrowDown") {
        e.preventDefault();
        if (histPos < history.length) {
          histPos++;
          input.textContent = history[histPos] || "";
          focusInput();
        }
      }
    });

    window._retroCLI = { show: showCLI, hide: hideCLI };
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
