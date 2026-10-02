import assert from "node:assert/strict";
import { mkdtemp, readdir, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import type { Page } from "playwright";
import type { Config } from "../config.js";
import { HttpError } from "../errors.js";
import { SnapshotStore } from "../snapshots.js";
import type { Snapshot } from "../types.js";

function config(root: string): Config {
  return {
    address: "127.0.0.1",
    port: 8081,
    workerId: "test",
    hostname: "test",
    version: "test",
    baseUrl: "http://test:8081",
    vncUrl: "http://test:6080/websockify",
    centralUrl: "http://test:8080",
    capacity: 3,
    heartbeatMs: 10_000,
    screenshotIntervalMs: 10_000,
    periodicRetentionMs: 600_000,
    errorRetentionMs: 86_400_000,
    screenshotMaxBytes: 1_000_000,
    screenshotRoot: root,
    sessionRoot: path.join(root, "sessions"),
    actionTimeoutMs: 30_000,
    maxEvents: 100,
  };
}

async function writeSnapshot(root: string, snapshot: Snapshot, image = "png"): Promise<void> {
  await writeFile(path.join(root, snapshot.id + "." + snapshot.format), Buffer.from(image));
  await writeFile(path.join(root, snapshot.id + ".json"), JSON.stringify(snapshot));
}

test("manual snapshots survive store initialization and can be deleted", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "scraper-snapshots-"));
  const snapshot: Snapshot = {
    id: "snap_test",
    session: "sess_old",
    page: "page_old",
    class: "manual",
    format: "png",
    created_at: new Date().toISOString(),
    size_bytes: 3,
  };
  await writeSnapshot(root, snapshot);

  const store = new SnapshotStore(config(root));
  await store.initialize();
  assert.equal(store.listManual().length, 1);
  assert.equal(store.get("snap_test")?.metadata.id, "snap_test");
  assert.equal(await store.delete("snap_test"), true);
  assert.equal(store.get("snap_test"), undefined);
});

test("a manual snapshot is rejected instead of exceeding the storage ceiling", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "scraper-snapshots-limit-"));
  const limited = config(root);
  limited.screenshotMaxBytes = 2;
  const store = new SnapshotStore(limited);
  await store.initialize();
  const page = {
    screenshot: async () => Buffer.from("png"),
  } as unknown as Page;

  await assert.rejects(
    store.capture(page, {
      sessionId: "sess_test",
      pageId: "page_test",
      className: "manual",
      format: "png",
    }),
    (error: unknown) => error instanceof HttpError && error.code === "snapshot_storage_full",
  );
  assert.equal(store.listManual().length, 0);
});

test("initialization removes incomplete snapshot files", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "scraper-snapshots-incomplete-"));
  await writeFile(path.join(root, "snap_broken.json"), "not json");
  await writeFile(path.join(root, "snap_orphan.png"), Buffer.from("png"));

  const store = new SnapshotStore(config(root));
  await store.initialize();

  assert.deepEqual(await readdir(root), []);
});

test("cleanup applies retention without expiring manual snapshots", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "scraper-snapshots-retention-"));
  const expired: Snapshot = {
    id: "snap_expired",
    class: "periodic",
    format: "png",
    created_at: new Date(Date.now() - 60_000).toISOString(),
    size_bytes: 3,
  };
  const manual: Snapshot = {
    id: "snap_manual",
    class: "manual",
    format: "png",
    created_at: new Date(Date.now() - 60_000).toISOString(),
    size_bytes: 3,
  };
  await writeSnapshot(root, expired);
  await writeSnapshot(root, manual);
  const retention = config(root);
  retention.periodicRetentionMs = 1_000;

  const store = new SnapshotStore(retention);
  await store.initialize();

  assert.equal(store.get(expired.id), undefined);
  assert.equal(store.get(manual.id)?.metadata.id, manual.id);
});
