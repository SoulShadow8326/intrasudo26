window.sudoConfetti = (() => {
  const canvas = document.getElementById("confetti-canvas");
  if (!canvas) return null;

  const ctx = canvas.getContext("2d");

  const colors = ["#00E5FF", "#FFD500", "#00C48C", "#FF6B92", "#A96CFF"];
  let pieces = [];

  const resize = () => {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
  };

  const seed = () => {
    pieces = Array.from({ length: 90 }, () => ({
      x: Math.floor(Math.random() * canvas.width),
      y: Math.floor(Math.random() * canvas.height) - canvas.height,
      size: 6 + Math.floor(Math.random() * 10),
      speed: 2 + Math.floor(Math.random() * 3),
      drift: -2 + Math.floor(Math.random() * 5),
      color: colors[Math.floor(Math.random() * colors.length)],
    }));
  };

  const burst = () => {
    resize();
    seed();
    canvas.classList.remove("hidden");
    if (typeof ctx.imageSmoothingEnabled !== "undefined") ctx.imageSmoothingEnabled = false;
    ctx.save();
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillStyle = "#ffffff";
    const baseSize = Math.max(32, Math.min(72, Math.floor(canvas.width / 18)));
    ctx.font = `${baseSize}px 'PressStart2P', 'Pixelify Sans', sans-serif`;

    const parseLevel = (id) => {
      if (!id) return 0;
      const s = String(id);
      const match = s.match(/(\d+)$/);
      return match ? parseInt(match[1], 10) : 0;
    };

    const currentLevel = window.__LEVEL__ ? parseLevel(window.__LEVEL__.ID) : 0;
    const nextLevel = currentLevel + 1;

    if (window.sudoAudio) window.sudoAudio.playConfetti();
    let frames = 0;

    const draw = () => {
      frames += 1;
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      pieces.forEach((piece) => {
        piece.y += piece.speed;
        piece.x += piece.drift;
        const px = Math.round(piece.x);
        const py = Math.round(piece.y);
        const s = Math.round(piece.size);
        ctx.fillStyle = piece.color;
        if (s <= 8) {
          ctx.fillRect(px, py, s, s);
          ctx.fillRect(px + s, py, s, s);
          ctx.fillRect(px, py + s, s, s);
          ctx.fillRect(px + s, py + s, s, s);
        } else {
          ctx.fillRect(px, py, s, s);
        }

        ctx.fillStyle = "rgba(0,0,0,0.15)";
        ctx.fillRect(px + Math.max(1, Math.floor(s / 2)), py + Math.max(1, Math.floor(s / 2)), Math.max(1, Math.floor(s / 6)), Math.max(1, Math.floor(s / 6)));
      });

      const x = canvas.width / 2;
      const y = canvas.height / 2;

      if (frames < 50) {
        const alpha = frames < 10 ? frames / 10 : frames > 40 ? (50 - frames) / 10 : 1;
        ctx.globalAlpha = Math.max(0, Math.min(1, alpha));
        const phase = (frames % 40) / 40;
        const shadowOffset = phase < 0.5 ? 3 : 2;
        const shadowAlpha = phase < 0.5 ? 0.6 : 0.55;
        ctx.fillStyle = `rgba(0,0,0,${shadowAlpha})`;
        ctx.fillText("LEVEL UP!", x + shadowOffset, y + shadowOffset);
        ctx.fillStyle = "#fff";
        ctx.fillText("LEVEL UP!", x, y);
      } else if (frames < 140) {
        const startFrame = 50;
        const duration = 40;
        const t = Math.max(0, Math.min(1, (frames - startFrame) / duration));
        const easedT = t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2;
        
        const alpha = frames < 60 ? (frames - 50) / 10 : frames > 130 ? (140 - frames) / 10 : 1;
        ctx.globalAlpha = Math.max(0, Math.min(1, alpha));

        const distance = 120;
        const oldY = y + (easedT * distance);
        const newY = (y - distance) + (easedT * distance);

        if (t < 1) {
          const oldAlpha = ctx.globalAlpha * (1 - easedT);
          ctx.save();
          ctx.globalAlpha = oldAlpha;
          ctx.fillStyle = "rgba(0,0,0,0.4)";
          ctx.fillText(`LEVEL ${currentLevel}`, x + 2, oldY + 2);
          ctx.fillStyle = "rgba(255,255,255,0.7)";
          ctx.fillText(`LEVEL ${currentLevel}`, x, oldY);
          ctx.restore();
        }

        ctx.fillStyle = "rgba(0,0,0,0.6)";
        ctx.fillText(`LEVEL ${nextLevel}`, x + 3, newY + 3);
        ctx.fillStyle = "#fff";
        ctx.fillText(`LEVEL ${nextLevel}`, x, newY);
      }

      ctx.globalAlpha = 1;
      if (frames < 150) {
        requestAnimationFrame(draw);
      } else {
        canvas.classList.add("hidden");
        ctx.restore();
      }
    };

    draw();
  };

  resize();
  window.addEventListener("resize", resize);
  return burst;
})();
