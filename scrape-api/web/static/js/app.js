const refreshMs = Number(document.body.dataset.autoRefresh || 0);
if (refreshMs > 0) {
  window.setTimeout(() => window.location.reload(), refreshMs);
}

const events = document.querySelector("[data-events-url]");
if (events) {
  fetch(events.dataset.eventsUrl)
    .then(async (response) => {
      if (!response.ok) throw new Error(await response.text());
      return response.json();
    })
    .then((payload) => {
      events.textContent = JSON.stringify(payload.events ?? payload, null, 2);
    })
    .catch((error) => {
      events.textContent = "Unable to load diagnostics: " + error.message;
    });
}

const terminate = document.querySelector("[data-delete-session]");
if (terminate) {
  terminate.addEventListener("click", async () => {
    if (!window.confirm("Terminate " + terminate.dataset.deleteSession + "?")) return;
    terminate.disabled = true;
    const response = await fetch("/v1/sessions/" + encodeURIComponent(terminate.dataset.deleteSession), { method: "DELETE" });
    if (response.ok) window.location.assign("/");
    else {
      window.alert(await response.text());
      terminate.disabled = false;
    }
  });
}

const profileForm = document.querySelector("#profile-form");
if (profileForm) {
  profileForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = new FormData(profileForm);
    const payload = {
      name: form.get("name"),
      description: form.get("description") || undefined,
      browser: "chromium",
      userAgent: form.get("userAgent") || undefined,
      locale: form.get("locale") || undefined,
      timezoneId: form.get("timezoneId") || undefined,
      viewport: {
        width: Number(form.get("width")),
        height: Number(form.get("height")),
      },
    };
    const output = document.querySelector("#profile-result");
    const response = await fetch("/v1/profiles", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(payload),
    });
    output.textContent = response.ok ? "Profile created." : await response.text();
    if (response.ok) window.location.reload();
  });
}
