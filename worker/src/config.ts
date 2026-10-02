import os from "node:os";
import path from "node:path";

export interface Config {
  address: string;
  port: number;
  workerId: string;
  hostname: string;
  version: string;
  baseUrl: string;
  vncUrl: string;
  centralUrl: string;
  capacity: number;
  heartbeatMs: number;
  screenshotIntervalMs: number;
  periodicRetentionMs: number;
  errorRetentionMs: number;
  screenshotMaxBytes: number;
  screenshotRoot: string;
  sessionRoot: string;
  actionTimeoutMs: number;
  maxEvents: number;
}

function integer(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const value = Number(raw);
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error(name + " must be a positive integer");
  }
  return value;
}

function duration(name: string, fallback: number): number {
  const raw = process.env[name];
  if (!raw) return fallback;
  const match = /^([1-9][0-9]*)(ms|s|m|h)?$/.exec(raw);
  if (!match) throw new Error(name + " must be a positive duration");
  const amount = Number(match[1]);
  const unit = match[2] ?? "ms";
  const multiplier = unit === "h" ? 3_600_000 : unit === "m" ? 60_000 : unit === "s" ? 1_000 : 1;
  const milliseconds = amount * multiplier;
  if (!Number.isSafeInteger(milliseconds)) throw new Error(name + " is too large");
  return milliseconds;
}

export function loadConfig(): Config {
  const workerId = process.env.WORKER_ID ?? os.hostname();
  const port = integer("WORKER_PORT", 8081);
  return {
    address: process.env.WORKER_ADDRESS ?? "0.0.0.0",
    port,
    workerId,
    hostname: process.env.WORKER_HOSTNAME ?? os.hostname(),
    version: process.env.WORKER_VERSION ?? "dev",
    baseUrl: process.env.WORKER_BASE_URL ?? "http://" + workerId + ":" + port,
    vncUrl: process.env.WORKER_VNC_URL ?? "http://" + workerId + ":6080/websockify",
    centralUrl: (process.env.SCRAPE_API_URL ?? "http://scrape-api:8080").replace(/\/$/, ""),
    capacity: integer("WORKER_CAPACITY", 3),
    heartbeatMs: duration("WORKER_HEARTBEAT_INTERVAL", 10_000),
    screenshotIntervalMs: duration("SCREENSHOT_INTERVAL", 10_000),
    periodicRetentionMs: duration("SCREENSHOT_RETENTION", 600_000),
    errorRetentionMs: duration("ERROR_SCREENSHOT_RETENTION", 86_400_000),
    screenshotMaxBytes: integer("SCREENSHOT_MAX_BYTES", 1_073_741_824),
    screenshotRoot: process.env.SCREENSHOT_ROOT ?? path.resolve("screenshots"),
    sessionRoot: process.env.SESSION_ROOT ?? path.resolve("sessions"),
    actionTimeoutMs: duration("ACTION_TIMEOUT", 30_000),
    maxEvents: integer("MAX_EVENTS", 500),
  };
}
