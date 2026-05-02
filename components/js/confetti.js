window.IntraSudoConfetti = (() => {
  const canvas = document.getElementById("confetti-canvas");
  if (!canvas) return null;

  const ctx = canvas.getContext("2d");
  const colors = ["#22d3ee", "#f59e0b", "#34d399", "#fb7185", "#c084fc"];
  let pieces = [];

  const resize = () => {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
  };

  const seed = () => {
    pieces = Array.from({ length: 120 }, () => ({
      x: Math.random() * canvas.width,
      y: Math.random() * canvas.height - canvas.height,
      size: 6 + Math.random() * 8,
      speed: 2 + Math.random() * 4,
      drift: -2 + Math.random() * 4,
      color: colors[Math.floor(Math.random() * colors.length)],
    }));
  };

  const burst = () => {
    resize();
    seed();
    canvas.classList.remove("hidden");
    if (window.IntraSudoAudio) window.IntraSudoAudio.playConfetti();
    let frames = 0;

    const draw = () => {
      frames += 1;
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      pieces.forEach((piece) => {
        piece.y += piece.speed;
        piece.x += piece.drift;
        ctx.fillStyle = piece.color;
        ctx.fillRect(piece.x, piece.y, piece.size, piece.size * 0.6);
      });
      if (frames < 160) {
        requestAnimationFrame(draw);
      } else {
        canvas.classList.add("hidden");
      }
    };

    draw();
  };

  resize();
  window.addEventListener("resize", resize);
  return burst;
})();
