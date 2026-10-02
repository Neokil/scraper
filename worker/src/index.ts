import { BrowserManager } from "./browser.js";
import { loadConfig } from "./config.js";
import { ControllerClient } from "./controller.js";
import { asError } from "./errors.js";
import { log } from "./log.js";
import { WorkerServer } from "./server.js";

async function main(): Promise<void> {
  const config = loadConfig();
  let controller: ControllerClient | undefined;
  const browsers = new BrowserManager(config, () => controller?.trigger());
  await browsers.initialize();
  const server = new WorkerServer(config, browsers);
  await server.listen();
  controller = new ControllerClient(config, browsers);
  controller.start();

  let stopping = false;
  const shutdown = async (signal: string): Promise<void> => {
    if (stopping) return;
    stopping = true;
    log("info", "worker shutting down", { signal });
    controller?.stop();
    await server.close().catch(() => {});
    await browsers.shutdown();
  };
  process.on("SIGTERM", () => void shutdown("SIGTERM").then(() => process.exit(0)));
  process.on("SIGINT", () => void shutdown("SIGINT").then(() => process.exit(0)));
}

main().catch((error: unknown) => {
  log("error", "worker failed", { error: asError(error).message });
  process.exit(1);
});
