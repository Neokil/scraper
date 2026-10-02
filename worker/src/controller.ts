import { randomUUID } from "node:crypto";
import { readFile } from "node:fs/promises";
import os from "node:os";
import type { BrowserManager } from "./browser.js";
import type { Config } from "./config.js";
import { asError } from "./errors.js";
import { log } from "./log.js";
import type { WorkerRegistration } from "./types.js";

export class ControllerClient {
  readonly incarnationId = randomUUID();
  private registered = false;
  private timer: NodeJS.Timeout | undefined;
  private running = false;
  private pending = false;
  private stopped = true;
  private retryMs = 1_000;

  constructor(
    private readonly config: Config,
    private readonly browsers: BrowserManager,
  ) {}

  start(): void {
    if (!this.stopped) return;
    this.stopped = false;
    this.schedule(0);
  }

  stop(): void {
    this.stopped = true;
    if (this.timer) clearTimeout(this.timer);
  }

  trigger(): void {
    if (this.stopped) return;
    if (this.running) {
      this.pending = true;
      return;
    }
    this.schedule(0);
  }

  private schedule(delay: number): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.tick();
    }, delay);
  }

  private async tick(): Promise<void> {
    if (this.stopped || this.running) return;
    this.running = true;
    let nextDelay = this.config.heartbeatMs;
    try {
      if (!this.registered) {
        await this.register();
        this.registered = true;
      } else {
        this.registered = await this.heartbeat();
        if (!this.registered) nextDelay = 0;
      }
      this.retryMs = 1_000;
    } catch (error) {
      log("warn", "central API communication failed", { error: asError(error).message });
      nextDelay = Math.min(this.retryMs, this.config.heartbeatMs);
      this.retryMs = Math.min(this.retryMs * 2, this.config.heartbeatMs);
    } finally {
      this.running = false;
      if (this.stopped) return;
      if (this.pending) nextDelay = 0;
      this.pending = false;
      this.schedule(nextDelay);
    }
  }

  private async registration(): Promise<WorkerRegistration> {
    return {
      worker_id: this.config.workerId,
      hostname: this.config.hostname,
      version: this.config.version,
      base_url: this.config.baseUrl,
      vnc_url: this.config.vncUrl,
      incarnation_id: this.incarnationId,
      capacity: this.config.capacity,
      cpu_percent: this.cpuPercent(),
      memory_bytes: await this.memoryBytes(),
      sessions: await this.browsers.listSessions(),
      snapshots: this.browsers.snapshots.listPublished(),
    };
  }

  private async register(): Promise<void> {
    const response = await this.send("/internal/workers/register", await this.registration());
    if (!response.ok) {
      throw new Error("registration returned " + response.status + ": " + await response.text());
    }
    log("info", "worker registered", { worker_id: this.config.workerId, incarnation_id: this.incarnationId });
  }

  private async heartbeat(): Promise<boolean> {
    const state = await this.registration();
    const response = await this.send("/internal/workers/" + encodeURIComponent(this.config.workerId) + "/heartbeat", {
      incarnation_id: state.incarnation_id,
      cpu_percent: state.cpu_percent,
      memory_bytes: state.memory_bytes,
      capacity: state.capacity,
      sessions: state.sessions,
      snapshots: state.snapshots,
    });
    if (response.status === 404 || response.status === 409) {
      return false;
    }
    if (!response.ok) {
      throw new Error("heartbeat returned " + response.status + ": " + await response.text());
    }
    return true;
  }

  private async send(pathname: string, body: unknown): Promise<Response> {
    return await fetch(this.config.centralUrl + pathname, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(Math.max(2_000, this.config.heartbeatMs - 250)),
    });
  }

  private cpuPercent(): number {
    const cpuCount = Math.max(1, os.cpus().length);
    return Math.round(((os.loadavg()[0] ?? 0) / cpuCount) * 10_000) / 100;
  }

  private async memoryBytes(): Promise<number> {
    try {
      return Number.parseInt((await readFile("/sys/fs/cgroup/memory.current", "utf8")).trim(), 10);
    } catch {
      return os.totalmem() - os.freemem();
    }
  }
}
