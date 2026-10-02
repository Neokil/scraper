import type { Page } from "playwright";
import { HttpError } from "./errors.js";
import type { ActionRequest } from "./types.js";

export async function runPageAction(
  page: Page,
  action: string,
  input: ActionRequest,
  maximumTimeout: number,
): Promise<unknown> {
  const timeout = actionTimeout(input.timeout, maximumTimeout);
  switch (action) {
    case "navigate": {
      const response = await page.goto(requiredString(input, "url"), {
        waitUntil: waitUntil(input.wait_until),
        timeout,
      });
      return { url: page.url(), status: response?.status() ?? null };
    }
    case "click":
      await page.locator(requiredString(input, "selector")).click({ timeout });
      return { ok: true };
    case "type": {
      const delay = optionalNumber(input.delay);
      await page.locator(requiredString(input, "selector")).pressSequentially(
        requiredString(input, "text"),
        delay === undefined ? { timeout } : { timeout, delay },
      );
      return { ok: true };
    }
    case "press":
      await page.locator(requiredString(input, "selector")).press(requiredString(input, "key"), { timeout });
      return { ok: true };
    case "select": {
      const raw = input.values ?? input.value;
      const values = Array.isArray(raw) ? raw.map(String) : [String(raw ?? "")];
      if (values.length === 0 || values[0] === "") {
        throw new HttpError(400, "invalid_request", "value or values is required");
      }
      return {
        values: await page.locator(requiredString(input, "selector")).selectOption(values, { timeout }),
      };
    }
    case "wait":
      await page.locator(requiredString(input, "selector")).waitFor({
        state: waitState(input.state),
        timeout,
      });
      return { ok: true };
    case "evaluate": {
      const result = await withTimeout(page.evaluate(requiredString(input, "expression")), timeout);
      let serialized: string | undefined;
      try {
        serialized = JSON.stringify(result);
      } catch {
        throw new HttpError(422, "result_not_serializable", "evaluation result is not JSON serializable");
      }
      if (serialized === undefined) {
        throw new HttpError(422, "result_not_serializable", "evaluation result is not JSON serializable");
      }
      return { result };
    }
    default:
      throw new HttpError(404, "action_not_found", "unknown page action");
  }
}

function requiredString(input: ActionRequest, key: string): string {
  const value = input[key];
  if (typeof value !== "string" || value === "") {
    throw new HttpError(400, "invalid_request", key + " must be a non-empty string");
  }
  return value;
}

function optionalNumber(value: unknown): number | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    throw new HttpError(400, "invalid_request", "expected a non-negative number");
  }
  return value;
}

function actionTimeout(value: unknown, maximum: number): number {
  const requested = optionalNumber(value);
  if (requested === undefined) return maximum;
  if (requested < 1 || requested > maximum) {
    throw new HttpError(400, "invalid_timeout", "timeout must be between 1 and the configured ACTION_TIMEOUT maximum");
  }
  return requested;
}

async function withTimeout<T>(operation: Promise<T>, timeout: number): Promise<T> {
  let timer: NodeJS.Timeout | undefined;
  try {
    return await Promise.race([
      operation,
      new Promise<T>((_resolve, reject) => {
        timer = setTimeout(
          () => reject(new HttpError(504, "playwright_timeout", "operation exceeded the configured timeout")),
          timeout,
        );
      }),
    ]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

function waitUntil(value: unknown): "load" | "domcontentloaded" | "networkidle" | "commit" {
  if (value === undefined) return "domcontentloaded";
  if (value === "load" || value === "domcontentloaded" || value === "networkidle" || value === "commit") return value;
  throw new HttpError(400, "invalid_request", "invalid wait_until");
}

function waitState(value: unknown): "attached" | "detached" | "visible" | "hidden" {
  if (value === undefined) return "visible";
  if (value === "attached" || value === "detached" || value === "visible" || value === "hidden") return value;
  throw new HttpError(400, "invalid_request", "invalid wait state");
}
