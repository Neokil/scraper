import assert from "node:assert/strict";
import test from "node:test";
import { loadConfig } from "../config.js";

test("configuration rejects partial integers", () => {
  const previous = process.env.WORKER_PORT;
  process.env.WORKER_PORT = "8081px";
  try {
    assert.throws(() => loadConfig(), /WORKER_PORT must be a positive integer/);
  } finally {
    if (previous === undefined) delete process.env.WORKER_PORT;
    else process.env.WORKER_PORT = previous;
  }
});

test("configuration rejects invalid durations", () => {
  const previous = process.env.ACTION_TIMEOUT;
  process.env.ACTION_TIMEOUT = "soon";
  try {
    assert.throws(() => loadConfig(), /ACTION_TIMEOUT must be a positive duration/);
  } finally {
    if (previous === undefined) delete process.env.ACTION_TIMEOUT;
    else process.env.ACTION_TIMEOUT = previous;
  }
});
