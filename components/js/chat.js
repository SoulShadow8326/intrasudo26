(() => {
  const form = document.getElementById("chat-form");
  const thread = document.getElementById("chat-thread");
  if (!form || !thread) return;

  const renderMessages = (messages) => {
    if (!messages.length) {
      thread.innerHTML = '<p class="empty-state">No chat history yet.</p>';
      return;
    }
    thread.innerHTML = messages
      .map(
        (msg) => `
      <div class="${msg.kind === "hint" ? "chat-hint" : "chat-message"}">
        <p class="chat-message-body">${msg.content}</p>
        <p class="chat-message-meta">${msg.author} • ${new Date(msg.time * 1000).toLocaleString()}</p>
      </div>
    `,
      )
      .join("");
    thread.scrollTop = thread.scrollHeight;
  };

  const initialMessages = [
    ...(window.__INTRASUDO_HINTS__ || []),
    ...(window.__INTRASUDO_MESSAGES__ || []),
  ].sort((a, b) => a.time - b.time);
  renderMessages(initialMessages);

  async function refreshChat() {
    const response = await fetch("/api/chats");
    if (!response.ok) return;
    const payload = await response.json();
    const all = [...(payload.hints || []), ...(payload.chats || [])].sort(
      (a, b) => a.time - b.time,
    );
    renderMessages(all);
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const body = new URLSearchParams(new FormData(form));
    const response = await fetch("/api/messages", { method: "POST", body });
    const payload = await response.json();
    if (!response.ok || payload.error) {
      window.IntraSudo.flashMessage(
        "play-message",
        payload.error || "Could not send message.",
        "error",
      );
      window.IntraSudo.toast(
        payload.error || "Could not send message.",
        "error",
      );
      return;
    }
    form.reset();
    window.IntraSudo.toast("Message sent.", "success");
    refreshChat();
  });

  setInterval(refreshChat, 5000);
})();
