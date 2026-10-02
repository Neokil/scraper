const target = process.env.SCRAPER_WS_URL;
if (!target) throw new Error("SCRAPER_WS_URL is required");

await new Promise((resolve, reject) => {
  const timeout = setTimeout(() => reject(new Error("VNC WebSocket did not open")), 5000);
  const socket = new WebSocket(target, ["binary"]);
  socket.addEventListener("open", () => {
    clearTimeout(timeout);
    socket.close();
    resolve();
  }, { once: true });
  socket.addEventListener("error", () => {
    clearTimeout(timeout);
    reject(new Error("VNC WebSocket failed"));
  }, { once: true });
});
