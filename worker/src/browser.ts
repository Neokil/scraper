import { randomUUID } from "node:crypto";
import { mkdir, rm } from "node:fs/promises";
import path from "node:path";
import {
  chromium,
  type BrowserContext,
  type Page,
} from "playwright";
import { runPageAction } from "./actions.js";
import type { Config } from "./config.js";
import { asError, HttpError } from "./errors.js";
import { log } from "./log.js";
import { contextOptions } from "./profile.js";
import { SnapshotStore } from "./snapshots.js";
import type {
  ActionRequest,
  CreatePageRequest,
  CreateSessionRequest,
  EventRecord,
  PageState,
  SessionState,
  Snapshot,
} from "./types.js";

interface PageRuntime {
  id: string;
  page: Page;
  createdAt: Date;
  lastActivity: Date;
  lastCaptureAt: number;
  capturing: boolean;
}

interface SessionRuntime {
  id: string;
  context: BrowserContext;
  directory: string;
  browser: "chromium";
  profile: string;
  createdAt: Date;
  lastActivity: Date;
  lastActivityAt: number;
  lastReportedActivity?: Date;
  idleTimeoutSeconds: number;
  sparePage: Page | undefined;
  pages: Map<string, PageRuntime>;
  pageIds: WeakMap<Page, string>;
  events: EventRecord[];
  closing: boolean;
}

export class BrowserManager {
  private readonly sessions = new Map<string, SessionRuntime>();
  private readonly pageSessions = new Map<string, string>();
  private maintenanceTimer?: NodeJS.Timeout;
  private maintaining = false;
  readonly snapshots: SnapshotStore;

  constructor(
    private readonly config: Config,
    private readonly onStateChanged: () => void,
  ) {
    this.snapshots = new SnapshotStore(config);
  }

  async initialize(): Promise<void> {
    await rm(this.config.sessionRoot, { recursive: true, force: true });
    await mkdir(this.config.sessionRoot, { recursive: true });
    await this.snapshots.initialize();
    this.maintenanceTimer = setInterval(() => {
      void this.maintain();
    }, Math.min(1_000, this.config.screenshotIntervalMs));
  }

  async shutdown(): Promise<void> {
    if (this.maintenanceTimer) clearInterval(this.maintenanceTimer);
    await Promise.allSettled([...this.sessions.keys()].map((id) => this.deleteSession(id)));
  }

  async createSession(input: CreateSessionRequest): Promise<SessionState> {
    if (!/^sess_[a-f0-9]{32}$/.test(input.id)) {
      throw new HttpError(400, "invalid_session_id", "session id is invalid");
    }
    if (this.sessions.has(input.id)) {
      throw new HttpError(409, "session_exists", "session already exists");
    }
    if (input.browser !== "chromium" || input.profile_settings.browser !== "chromium") {
      throw new HttpError(400, "unsupported_browser", "only chromium is supported");
    }
    if (!Number.isInteger(input.idle_timeout_seconds) || input.idle_timeout_seconds < 1) {
      throw new HttpError(400, "invalid_idle_timeout", "idle_timeout_seconds must be positive");
    }

    const directory = path.join(this.config.sessionRoot, input.id);
    await mkdir(directory, { recursive: false, mode: 0o700 });
    let context: BrowserContext | undefined;
    try {
      context = await chromium.launchPersistentContext(directory, contextOptions(input.profile_settings));
      const initialPages = context.pages();
      const sparePage = initialPages[0];
      await Promise.allSettled(initialPages.slice(1).map((page) => page.close()));
      const now = new Date();
      const session: SessionRuntime = {
        id: input.id,
        context,
        directory,
        browser: "chromium",
        profile: input.profile,
        createdAt: now,
        lastActivity: now,
        lastActivityAt: performance.now(),
        idleTimeoutSeconds: input.idle_timeout_seconds,
        sparePage,
        pages: new Map(),
        pageIds: new WeakMap(),
        events: [],
        closing: false,
      };
      this.sessions.set(session.id, session);
      context.on("page", (page) => this.attachPage(session, page, "page_" + randomUUID().replaceAll("-", "")));
      context.on("close", () => {
        if (!session.closing) {
          this.recordEvent(session, undefined, "browser_closed", "Chromium context closed unexpectedly");
          void this.deleteSession(session.id);
        }
      });
      this.onStateChanged();
      log("info", "session created", { session_id: input.id, profile: input.profile });
      return await this.sessionState(session);
    } catch (error) {
      if (context) await context.close().catch(() => {});
      await rm(directory, { recursive: true, force: true });
      throw error;
    }
  }

  async deleteSession(id: string): Promise<void> {
    const session = this.sessions.get(id);
    if (!session) throw new HttpError(404, "session_not_found", "session not found");
    if (session.closing) return;
    session.closing = true;
    this.sessions.delete(id);
    for (const pageId of session.pages.keys()) this.pageSessions.delete(pageId);
    await session.context.close().catch((error: unknown) => {
      log("warn", "browser context close failed", { session_id: id, error: asError(error).message });
    });
    await rm(session.directory, { recursive: true, force: true });
    this.onStateChanged();
    log("info", "session deleted", { session_id: id });
  }

  async listSessions(): Promise<SessionState[]> {
    return await Promise.all([...this.sessions.values()].map((session) => this.sessionState(session)));
  }

  async createPage(sessionId: string, input: CreatePageRequest): Promise<PageState> {
    const session = this.requireSession(sessionId);
    if (!input.id || this.pageSessions.has(input.id)) {
      throw new HttpError(409, "page_exists", "page id is missing or already exists");
    }
    const page = session.sparePage && !session.sparePage.isClosed()
      ? session.sparePage
      : await session.context.newPage();
    session.sparePage = undefined;
    const runtime = this.attachPage(session, page, input.id);
    if (input.url) {
      try {
        await page.goto(input.url, { waitUntil: "domcontentloaded", timeout: this.config.actionTimeoutMs });
      } catch (error) {
        const normalized = asError(error);
        const event = this.recordEvent(session, runtime, "navigation_error", normalized.message);
        void this.captureError(session, runtime, event);
        throw playwrightError(normalized, "navigation_failed");
      }
    }
    this.touch(session);
    return await this.pageState(session, runtime);
  }

  async deletePage(pageId: string): Promise<void> {
    const { session, runtime } = this.requirePage(pageId);
    session.pages.delete(pageId);
    this.pageSessions.delete(pageId);
    await runtime.page.close();
    this.touch(session);
  }

  async getPageState(pageId: string): Promise<PageState> {
    const { session, runtime } = this.requirePage(pageId);
    return await this.pageState(session, runtime);
  }

  async action(pageId: string, action: string, input: ActionRequest): Promise<unknown> {
    const { session, runtime } = this.requirePage(pageId);
    this.touch(session, runtime);
    try {
      return await runPageAction(runtime.page, action, input, this.config.actionTimeoutMs);
    } catch (error) {
      const normalized = asError(error);
      const event = this.recordEvent(
        session,
        runtime,
        action === "navigate" ? "navigation_error" : "playwright_error",
        normalized.message,
      );
      void this.captureError(session, runtime, event);
      if (error instanceof HttpError) throw error;
      throw playwrightError(normalized, action === "navigate" ? "navigation_failed" : "playwright_error");
    }
  }

  async html(pageId: string): Promise<string> {
    const { session, runtime } = this.requirePage(pageId);
    this.touch(session, runtime);
    return await runtime.page.content();
  }

  async title(pageId: string): Promise<string> {
    const { session, runtime } = this.requirePage(pageId);
    this.touch(session, runtime);
    return await runtime.page.title();
  }

  url(pageId: string): string {
    const { session, runtime } = this.requirePage(pageId);
    this.touch(session, runtime);
    return runtime.page.url();
  }

  async screenshot(
    pageId: string,
    options: { format: "png" | "jpeg"; quality?: number; fullPage: boolean; activity: boolean },
  ): Promise<Buffer> {
    const { session, runtime } = this.requirePage(pageId);
    if (options.activity) this.touch(session, runtime);
    if (options.format === "jpeg") {
      return await runtime.page.screenshot({
        type: "jpeg",
        quality: options.quality ?? 70,
        fullPage: options.fullPage,
      });
    }
    return await runtime.page.screenshot({ type: "png", fullPage: options.fullPage });
  }

  async createManualSnapshot(
    pageId: string,
    options: { format?: "png" | "jpeg"; quality?: number; fullPage?: boolean },
  ): Promise<Snapshot> {
    const { session, runtime } = this.requirePage(pageId);
    this.touch(session, runtime);
    return await this.snapshots.capture(runtime.page, {
      sessionId: session.id,
      pageId,
      className: "manual",
      ...options,
    });
  }

  eventsForSession(sessionId: string): EventRecord[] {
    return [...this.requireSession(sessionId).events];
  }

  eventsForPage(pageId: string): EventRecord[] {
    const { session } = this.requirePage(pageId);
    return session.events.filter((event) => event.page === pageId);
  }

  activity(sessionId: string, at?: string): void {
    const session = this.requireSession(sessionId);
    const receivedAt = new Date();
    const value = at ? new Date(at) : receivedAt;
    if (Number.isNaN(value.valueOf())) throw new HttpError(400, "invalid_activity", "invalid activity timestamp");
    if (!session.lastReportedActivity || value > session.lastReportedActivity) {
      session.lastReportedActivity = value;
      session.lastActivity = receivedAt;
      session.lastActivityAt = performance.now();
    }
  }

  private attachPage(session: SessionRuntime, page: Page, requestedId: string): PageRuntime {
    const existingId = session.pageIds.get(page);
    if (existingId && existingId !== requestedId) {
      session.pages.delete(existingId);
      this.pageSessions.delete(existingId);
    } else if (existingId) {
      const existing = session.pages.get(existingId);
      if (existing) return existing;
    }
    const now = new Date();
    const runtime: PageRuntime = {
      id: requestedId,
      page,
      createdAt: now,
      lastActivity: now,
      lastCaptureAt: performance.now(),
      capturing: false,
    };
    session.pages.set(requestedId, runtime);
    session.pageIds.set(page, requestedId);
    this.pageSessions.set(requestedId, session.id);

    page.on("request", () => this.touch(session, runtime));
    page.on("framenavigated", () => this.touch(session, runtime));
    page.on("console", (message) => {
      if (message.type() === "error") {
        this.recordEvent(session, runtime, "console_error", message.text());
      }
    });
    page.on("pageerror", (error) => {
      const event = this.recordEvent(session, runtime, "page_error", error.message);
      void this.captureError(session, runtime, event);
    });
    page.on("requestfailed", (request) => {
      this.recordEvent(session, runtime, "request_failed", request.failure()?.errorText ?? "request failed", {
        method: request.method(),
        resource_type: request.resourceType(),
      });
    });
    page.on("close", () => {
      session.pages.delete(runtime.id);
      this.pageSessions.delete(runtime.id);
      this.onStateChanged();
    });
    this.onStateChanged();
    return runtime;
  }

  private async sessionState(session: SessionRuntime): Promise<SessionState> {
    const pages = await Promise.all([...session.pages.values()].map((page) => this.pageState(session, page)));
    return {
      id: session.id,
      worker: this.config.workerId,
      status: "running",
      browser: session.browser,
      profile: session.profile,
      created_at: session.createdAt.toISOString(),
      last_activity: session.lastActivity.toISOString(),
      idle_timeout_seconds: session.idleTimeoutSeconds,
      pages,
    };
  }

  private async pageState(session: SessionRuntime, runtime: PageRuntime): Promise<PageState> {
    let title = "";
    try {
      title = await runtime.page.title();
    } catch {
      // A page can disappear while a heartbeat is being serialized.
    }
    return {
      id: runtime.id,
      session_id: session.id,
      url: runtime.page.url(),
      title,
      status: runtime.page.isClosed() ? "closed" : "open",
      created_at: runtime.createdAt.toISOString(),
      last_activity: runtime.lastActivity.toISOString(),
    };
  }

  private requireSession(id: string): SessionRuntime {
    const session = this.sessions.get(id);
    if (!session) throw new HttpError(404, "session_not_found", "session not found");
    return session;
  }

  private requirePage(id: string): { session: SessionRuntime; runtime: PageRuntime } {
    const sessionId = this.pageSessions.get(id);
    if (!sessionId) throw new HttpError(404, "page_not_found", "page not found");
    const session = this.requireSession(sessionId);
    const runtime = session.pages.get(id);
    if (!runtime) throw new HttpError(404, "page_not_found", "page not found");
    return { session, runtime };
  }

  private touch(session: SessionRuntime, page?: PageRuntime): void {
    const now = new Date();
    session.lastActivity = now;
    session.lastActivityAt = performance.now();
    if (page) page.lastActivity = now;
  }

  private recordEvent(
    session: SessionRuntime,
    page: PageRuntime | undefined,
    type: string,
    message: string,
    context?: Record<string, unknown>,
  ): EventRecord {
    const event: EventRecord = {
      id: "evt_" + randomUUID().replaceAll("-", ""),
      timestamp: new Date().toISOString(),
      session: session.id,
      type,
      message,
    };
    if (page) {
      event.page = page.id;
      event.url = page.page.url();
    }
    if (context) event.context = context;
    session.events.push(event);
    if (session.events.length > this.config.maxEvents) {
      session.events.splice(0, session.events.length - this.config.maxEvents);
    }
    return event;
  }

  private async captureError(session: SessionRuntime, page: PageRuntime, event: EventRecord): Promise<void> {
    try {
      const snapshot = await this.snapshots.capture(page.page, {
        sessionId: session.id,
        pageId: page.id,
        className: "error",
      });
      event.snapshot_id = snapshot.id;
    } catch (error) {
      if (session.closing || page.page.isClosed()) return;
      log("warn", "error screenshot failed", {
        session_id: session.id,
        page_id: page.id,
        error: asError(error).message,
      });
    }
  }

  private async maintain(): Promise<void> {
    if (this.maintaining) return;
    this.maintaining = true;
    try {
      await this.runMaintenance();
    } finally {
      this.maintaining = false;
    }
  }

  private async runMaintenance(): Promise<void> {
    const now = performance.now();
    const expired: string[] = [];
    for (const session of this.sessions.values()) {
      if (now - session.lastActivityAt >= session.idleTimeoutSeconds * 1_000) {
        expired.push(session.id);
        continue;
      }
      for (const page of session.pages.values()) {
        if (page.capturing || page.page.isClosed()) continue;
        if (now - page.lastCaptureAt < this.config.screenshotIntervalMs) continue;
        page.capturing = true;
        page.lastCaptureAt = now;
        void this.snapshots.capture(page.page, {
          sessionId: session.id,
          pageId: page.id,
          className: "periodic",
        }).catch((error: unknown) => {
          log("warn", "periodic screenshot failed", {
            session_id: session.id,
            page_id: page.id,
            error: asError(error).message,
          });
        }).finally(() => {
          page.capturing = false;
        });
      }
    }
    for (const id of expired) {
      log("info", "session expired", { session_id: id });
      await this.deleteSession(id).catch(() => {});
    }
    await this.snapshots.cleanup();
  }

}

function playwrightError(error: Error, code: string): HttpError {
  if (error.name === "TimeoutError") {
    return new HttpError(504, "playwright_timeout", error.message);
  }
  return new HttpError(502, code, error.message);
}
