(() => {
  const config = window.__INTRASUDO_REDIRECT__;
  if (!config) return;

  window.IntraSudo.toast(config.reason, config.tone || "success");
  setTimeout(() => {
    window.location.href = config.url || "/";
  }, Number(config.delay || 1400));
})();
