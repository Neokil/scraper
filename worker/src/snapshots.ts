import { randomUUID } from "node:crypto";
import { mkdir, readFile, readdir, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import type { Page } from "playwright";
import type { Config } from "./config.js";
import { HttpError } from "./errors.js";
import type { Snapshot } from "./types.js";

interface StoredSnapshot {
  metadata: Snapshot;
  imagePath: string;
  sidecarPath: string;
}

export class SnapshotStore {
  private readonly snapshots = new Map<string, StoredSnapshot>();

  constructor(private readonly config: Config) {}

  async initialize(): Promise<void> {
    await mkdir(this.config.screenshotRoot, { recursive: true });
    this.snapshots.clear();
    const entries = await readdir(this.config.screenshotRoot, { withFileTypes: true });
    const referencedImages = new Set<string>();
    for (const entry of entries) {
      if (!entry.isFile() || !entry.name.endsWith(".json")) continue;
      const sidecarPath = path.join(this.config.screenshotRoot, entry.name);
      try {
        const parsed: unknown = JSON.parse(await readFile(sidecarPath, "utf8"));
        if (!isSnapshot(parsed)) throw new Error("invalid snapshot metadata");
        const metadata = parsed;
        const imagePath = path.join(this.config.screenshotRoot, metadata.id + "." + metadata.format);
        const details = await stat(imagePath);
        metadata.size_bytes = details.size;
        this.snapshots.set(metadata.id, { metadata, imagePath, sidecarPath });
        referencedImages.add(imagePath);
      } catch {
        await rm(sidecarPath, { force: true });
      }
    }
    await Promise.all(entries
      .filter((entry) => entry.isFile() && /\.(png|jpeg)$/.test(entry.name))
      .map((entry) => path.join(this.config.screenshotRoot, entry.name))
      .filter((imagePath) => !referencedImages.has(imagePath))
      .map((imagePath) => rm(imagePath, { force: true })));
    await this.cleanup();
  }

  async capture(
    page: Page,
    details: {
      sessionId: string;
      pageId: string;
      className: Snapshot["class"];
      format?: Snapshot["format"];
      fullPage?: boolean;
      quality?: number;
    },
  ): Promise<Snapshot> {
    const format = details.format ?? "jpeg";
    const options = format === "jpeg"
      ? { type: "jpeg" as const, quality: details.quality ?? 70, fullPage: details.fullPage ?? false }
      : { type: "png" as const, fullPage: details.fullPage ?? false };
    const image = await page.screenshot(options);
    const id = "snap_" + randomUUID().replaceAll("-", "");
    const metadata: Snapshot = {
      id,
      session: details.sessionId,
      page: details.pageId,
      class: details.className,
      format,
      created_at: new Date().toISOString(),
      size_bytes: image.byteLength,
    };
    const imagePath = path.join(this.config.screenshotRoot, id + "." + format);
    const sidecarPath = path.join(this.config.screenshotRoot, id + ".json");
    try {
      await writeFile(imagePath, image, { flag: "wx", mode: 0o640 });
      await writeFile(sidecarPath, JSON.stringify(metadata) + "\n", { flag: "wx", mode: 0o640 });
    } catch (error) {
      await Promise.all([rm(imagePath, { force: true }), rm(sidecarPath, { force: true })]);
      throw error;
    }
    this.snapshots.set(id, { metadata, imagePath, sidecarPath });
    await this.cleanup();
    if (!this.snapshots.has(id)) {
      throw new HttpError(507, "snapshot_storage_full", "snapshot was evicted by the worker storage ceiling");
    }
    if (this.totalBytes() > this.config.screenshotMaxBytes) {
      await this.delete(id);
      throw new HttpError(507, "snapshot_storage_full", "manual snapshots have exhausted the worker storage ceiling");
    }
    return metadata;
  }

  get(id: string): StoredSnapshot | undefined {
    return this.snapshots.get(id);
  }

  listManual(): Snapshot[] {
    return [...this.snapshots.values()]
      .filter((entry) => entry.metadata.class === "manual")
      .map((entry) => entry.metadata)
      .sort((a, b) => a.created_at.localeCompare(b.created_at));
  }

  listPublished(): Snapshot[] {
    return [...this.snapshots.values()]
      .filter((entry) => entry.metadata.class !== "periodic")
      .map((entry) => entry.metadata)
      .sort((a, b) => a.created_at.localeCompare(b.created_at));
  }

  async delete(id: string): Promise<boolean> {
    const entry = this.snapshots.get(id);
    if (!entry) return false;
    this.snapshots.delete(id);
    await Promise.all([
      rm(entry.imagePath, { force: true }),
      rm(entry.sidecarPath, { force: true }),
    ]);
    return true;
  }

  async cleanup(now = Date.now()): Promise<void> {
    const entries = [...this.snapshots.values()].sort(
      (a, b) => a.metadata.created_at.localeCompare(b.metadata.created_at),
    );
    for (const entry of entries) {
      const age = now - Date.parse(entry.metadata.created_at);
      const expired = entry.metadata.class === "periodic"
        ? age > this.config.periodicRetentionMs
        : entry.metadata.class === "error"
          ? age > this.config.errorRetentionMs
          : false;
      if (expired) await this.delete(entry.metadata.id);
    }

    let total = this.totalBytes();
    if (total <= this.config.screenshotMaxBytes) return;
    const evictable = [...this.snapshots.values()]
      .filter((entry) => entry.metadata.class !== "manual")
      .sort((a, b) => a.metadata.created_at.localeCompare(b.metadata.created_at));
    for (const entry of evictable) {
      if (total <= this.config.screenshotMaxBytes) break;
      total -= entry.metadata.size_bytes;
      await this.delete(entry.metadata.id);
    }
  }

  private totalBytes(): number {
    return [...this.snapshots.values()].reduce((sum, entry) => sum + entry.metadata.size_bytes, 0);
  }
}

function isSnapshot(value: unknown): value is Snapshot {
  if (typeof value !== "object" || value === null) return false;
  const snapshot = value as Partial<Snapshot>;
  return typeof snapshot.id === "string"
    && /^snap_[A-Za-z0-9_-]+$/.test(snapshot.id)
    && (snapshot.class === "periodic" || snapshot.class === "error" || snapshot.class === "manual")
    && (snapshot.format === "png" || snapshot.format === "jpeg")
    && typeof snapshot.created_at === "string"
    && Number.isFinite(Date.parse(snapshot.created_at));
}
