(() => {
  const config = window.__REDIRECT__;
  if (!config) return;

  window.sudo.toast(config.reason, config.tone || "success");
  setTimeout(
    () => {
      window.location.href = config.url || "/";
    },
    Number(config.delay || 1400),
  );
})();
