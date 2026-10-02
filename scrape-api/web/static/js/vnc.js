import RFB from "/static/vendor/novnc/lib/rfb.js";

const sessionId = window.location.pathname.split("/").filter(Boolean).at(-2);
const scheme = window.location.protocol === "https:" ? "wss" : "ws";
const socketURL = scheme + "://" + window.location.host + "/v1/sessions/" + encodeURIComponent(sessionId) + "/vnc/websocket";
const status = document.querySelector("#status");
const screen = document.querySelector("#screen");
const rfb = new RFB(screen, socketURL);
rfb.scaleViewport = true;
rfb.resizeSession = false;
rfb.addEventListener("connect", () => { status.textContent = "Connected"; });
rfb.addEventListener("disconnect", (event) => {
  status.textContent = event.detail.clean ? "Disconnected" : "Connection lost";
});

let lastActivity = 0;
const reportActivity = () => {
  const now = Date.now();
  if (now - lastActivity < 1000) return;
  lastActivity = now;
  fetch("/v1/sessions/" + encodeURIComponent(sessionId) + "/activity", { method: "POST", keepalive: true }).catch(() => {});
};
screen.addEventListener("pointerdown", reportActivity, { passive: true });
screen.addEventListener("pointermove", (event) => {
  if (event.buttons) reportActivity();
}, { passive: true });
window.addEventListener("keydown", reportActivity, { passive: true });
