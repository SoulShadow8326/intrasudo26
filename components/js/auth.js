(() => {
  const form = document.getElementById("auth-form");
  if (!form) return;

  const buttons = document.querySelectorAll(".auth-mode-btn");
  const otpWrap = document.getElementById("otpform_container");
  const otpInput = document.getElementById("otp");
  const otpDigits = Array.from(document.querySelectorAll(".otp-input"));
  let otpRequested = false;

  const resetFormState = () => {
    otpRequested = false;
    otpWrap.classList.add("hidden");
    form.classList.remove("otp-active");
    otpDigits.forEach((input) => {
      input.value = "";
    });
    otpInput.value = "";
  };

  const syncOtp = () => {
    otpInput.value = otpDigits.map((input) => input.value.trim()).join("");
  };

  otpDigits.forEach((input, index) => {
    input.addEventListener("paste", (e) => {
      e.preventDefault();
      const paste =
        (e.clipboardData || window.clipboardData).getData("text") || "";
      const digits = paste.replace(/\D/g, "").slice(0, 6).split("");
      for (let i = 0; i < 6; i++) {
        otpDigits[i].value = digits[i] || "";
      }
      syncOtp();
      const lastIdx = Math.min(digits.length - 1, 5);
      if (lastIdx >= 0) otpDigits[lastIdx].focus();
      otpRequested = true;
      otpWrap.classList.remove("hidden");
      form.classList.add("otp-active");
    });
    input.addEventListener("input", () => {
      input.value = input.value.replace(/\D/g, "").slice(0, 1);
      syncOtp();
      if (input.value && otpDigits[index + 1]) {
        otpDigits[index + 1].focus();
      }
    });

    input.addEventListener("keydown", (event) => {
      if (event.key === "Backspace" && !input.value && otpDigits[index - 1]) {
        otpDigits[index - 1].focus();
      }
    });
  });

  buttons.forEach((btn) => btn.classList.remove("is-active"));
  resetFormState();

  form.addEventListener("submit", async (event) => {
    event.preventDefault();

    if (!otpRequested) {
      const email = document.getElementById("email").value.trim();
      const name = document.getElementById("name").value.trim();

      if (!name || !email) {
        window.IntraSudo.toast("Fill in your name and email first.", "error");
        return;
      }

      otpRequested = true;
      otpWrap.classList.remove("hidden");
      form.classList.add("otp-active");
      otpDigits[0]?.focus();
      window.IntraSudo.toast("Sending OTP...", "info");

      const otpBody = new URLSearchParams({ email });
      const otpResponse = await fetch("/send_otp", {
        method: "POST",
        body: otpBody,
      });
      const otpPayload = await otpResponse.json();

      if (!otpResponse.ok || otpPayload.error) {
        otpRequested = false;
        otpWrap.classList.add("hidden");
        form.classList.remove("otp-active");
        window.IntraSudo.toast(
          otpPayload.error || "Could not send OTP.",
          "error",
        );
        return;
      }
      window.IntraSudo.toast(
        "OTP sent. Enter the 6-digit code to complete signup.",
        "success",
      );
      return;
    }

    if (otpRequested) {
      syncOtp();
      if (otpInput.value.length !== 6) {
        window.IntraSudo.toast("Enter the full 6-digit OTP.", "error");
        return;
      }
    }
    const body = new URLSearchParams(new FormData(form));
    const response = await fetch("/api/auth", { method: "POST", body });
    const payload = await response.json();
    if (!response.ok || payload.error) {
      window.IntraSudo.toast(
        payload.error || "Authentication failed.",
        "error",
      );
      return;
    }
    window.IntraSudo.toast(
      "Authentication successful. Redirecting...",
      "success",
    );
    setTimeout(() => {
      window.location.href = payload.redirect || "/play";
    }, 700);
  });
})();
