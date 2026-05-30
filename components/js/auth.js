(() => {
  const form = document.getElementById("auth-form");
  if (!form) return;

  const buttons = document.querySelectorAll(".auth-mode-btn");
  const otpWrap = document.getElementById("otpform_container");
  const otpInput = document.getElementById("otp");
  const otpDigits = Array.from(document.querySelectorAll(".otp-input"));
  const getCookie = (name) => {
    const m = document.cookie
      .split("; ")
      .find((row) => row.startsWith(name + "="));
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

      if (!email) {
        window.sudo.toast("Fill in your email first.", "error");
        return;
      }

      const emailRegex = /^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)?@dpsrkp\.net$/;
      if (!emailRegex.test(email)) {
        window.sudo.toast("invalid @dpsrkp.net email", "error");
        return;
      }

      window.sudo.toast("Sending OTP...", "info");

      const otpBody = new URLSearchParams({ email });
      const { res: otpResponse, parsed: otpParsed } =
        await window.sudo.fetchWithCSRF("/send_otp", {
          method: "POST",
          body: otpBody,
        });
      const otpPayload =
        otpParsed.json || (otpParsed.text ? { error: otpParsed.text } : {});

      if (!otpResponse.ok || otpPayload.error) {
        window.sudo.toast(otpPayload.error || "Could not send OTP.", "error");
        return;
      }

      otpRequested = true;
      otpWrap.classList.remove("hidden");
      form.classList.add("otp-active");
      otpDigits[0]?.focus();
      window.sudo.toast(
        "OTP sent. Enter the 6-digit code to complete signup.",
        "success",
      );
      return;
    }

    if (otpRequested) {
      syncOtp();
      if (otpInput.value.length !== 6) {
        window.sudo.toast("Enter the full 6-digit OTP.", "error");
        return;
      }
    }
    const body = new URLSearchParams(new FormData(form));
    const { res: response, parsed } = await window.sudo.fetchWithCSRF(
      "/api/auth",
      {
        method: "POST",
        body,
      },
    );
    const payload = parsed.json || (parsed.text ? { error: parsed.text } : {});
    if (!response.ok || payload.error) {
      window.sudo.toast(payload.error || "Authentication failed.", "error");
      return;
    }
    window.sudo.toast("Authentication successful. Redirecting...", "success");
    setTimeout(() => {
      window.location.href = payload.redirect || "/play";
    }, 700);
  });
})();
