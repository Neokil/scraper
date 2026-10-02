import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import http, { type IncomingMessage, type ServerResponse } from "node:http";
import type { AddressInfo } from "node:net";
import { URL } from "node:url";
import type { BrowserManager } from "./browser.js";
import type { Config } from "./config.js";
import { asError, HttpError } from "./errors.js";
import { log } from "./log.js";
import type { ActionRequest, CreatePageRequest, CreateSessionRequest } from "./types.js";

export class WorkerServer {
  private readonly server: http.Server;

  constructor(
    private readonly config: Config,
    private readonly browsers: BrowserManager,
  ) {
    this.server = http.createServer((request, response) => {
      void this.handle(request, response).catch((error: unknown) => this.failure(response, error));
    });
  }

  async listen(): Promise<void> {
    await new Promise<void>((resolve, reject) => {
      this.server.once("error", reject);
      this.server.listen(this.config.port, this.config.address, () => {
        this.server.off("error", reject);
        resolve();
      });
    });
    const address = this.server.address() as AddressInfo;
    log("info", "worker listening", { address: address.address, port: address.port });
  }

  async close(): Promise<void> {
    await new Promise<void>((resolve, reject) => {
      this.server.close((error) => error ? reject(error) : resolve());
      this.server.closeIdleConnections();
    });
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    const method = request.method ?? "GET";
    const parsed = new URL(request.url ?? "/", "http://worker");
    const pathname = parsed.pathname;

    if (method === "GET" && pathname === "/healthz") {
      this.json(response, 200, { status: "ok" });
      return;
    }
    if (method === "GET" && pathname === "/internal/state") {
      this.json(response, 200, {
        sessions: await this.browsers.listSessions(),
        snapshots: this.browsers.snapshots.listPublished(),
      });
      return;
    }
    if (method === "POST" && pathname === "/internal/sessions") {
      const input = await this.body<CreateSessionRequest>(request);
      this.json(response, 201, await this.browsers.createSession(input));
      return;
    }

    const sessionMatch = /^\/internal\/sessions\/([^/]+)$/.exec(pathname);
    if (sessionMatch?.[1] && method === "DELETE") {
      await this.browsers.deleteSession(decodeURIComponent(sessionMatch[1]));
      response.writeHead(204).end();
      return;
    }
    const activityMatch = /^\/internal\/sessions\/([^/]+)\/activity$/.exec(pathname);
    if (activityMatch?.[1] && method === "POST") {
      const input = await this.body<{ at?: string }>(request);
      this.browsers.activity(decodeURIComponent(activityMatch[1]), input.at);
      response.writeHead(204).end();
      return;
    }
    const sessionPagesMatch = /^\/internal\/sessions\/([^/]+)\/pages$/.exec(pathname);
    if (sessionPagesMatch?.[1] && method === "POST") {
      const input = await this.body<CreatePageRequest>(request);
      this.json(response, 201, await this.browsers.createPage(decodeURIComponent(sessionPagesMatch[1]), input));
      return;
    }
    const sessionEventsMatch = /^\/internal\/sessions\/([^/]+)\/events$/.exec(pathname);
    if (sessionEventsMatch?.[1] && method === "GET") {
      this.json(response, 200, { events: this.browsers.eventsForSession(decodeURIComponent(sessionEventsMatch[1])) });
      return;
    }

    const pageMatch = /^\/internal\/pages\/([^/]+)$/.exec(pathname);
    if (pageMatch?.[1]) {
      const pageId = decodeURIComponent(pageMatch[1]);
      if (method === "GET") {
        this.json(response, 200, await this.browsers.getPageState(pageId));
        return;
      }
      if (method === "DELETE") {
        await this.browsers.deletePage(pageId);
        response.writeHead(204).end();
        return;
      }
    }

    const actionMatch = /^\/internal\/pages\/([^/]+)\/(navigate|click|type|press|select|wait|evaluate)$/.exec(pathname);
    if (actionMatch?.[1] && actionMatch[2] && method === "POST") {
      const input = await this.body<ActionRequest>(request);
      this.json(response, 200, await this.browsers.action(decodeURIComponent(actionMatch[1]), actionMatch[2], input));
      return;
    }

    const pageDataMatch = /^\/internal\/pages\/([^/]+)\/(html|url|title|screenshot|events|snapshots)$/.exec(pathname);
    if (pageDataMatch?.[1] && pageDataMatch[2]) {
      const pageId = decodeURIComponent(pageDataMatch[1]);
      const kind = pageDataMatch[2];
      if (method === "GET" && kind === "html") {
        this.text(response, 200, "text/html; charset=utf-8", await this.browsers.html(pageId));
        return;
      }
      if (method === "GET" && kind === "url") {
        this.json(response, 200, { url: this.browsers.url(pageId) });
        return;
      }
      if (method === "GET" && kind === "title") {
        this.json(response, 200, { title: await this.browsers.title(pageId) });
        return;
      }
      if (method === "GET" && kind === "events") {
        this.json(response, 200, { events: this.browsers.eventsForPage(pageId) });
        return;
      }
      if (method === "GET" && kind === "screenshot") {
        const requestedFormat = parsed.searchParams.get("format");
        if (requestedFormat !== null && requestedFormat !== "png" && requestedFormat !== "jpeg") {
          throw new HttpError(400, "invalid_format", "format must be png or jpeg");
        }
        const format = requestedFormat ?? "jpeg";
        const quality = this.quality(parsed.searchParams.get("quality"));
        const fullPage = this.booleanQuery(parsed.searchParams.get("full_page"), "full_page");
        const activity = parsed.searchParams.get("monitoring") !== "1";
        const screenshotOptions: {
          format: "png" | "jpeg";
          quality?: number;
          fullPage: boolean;
          activity: boolean;
        } = { format, fullPage, activity };
        if (quality !== undefined) screenshotOptions.quality = quality;
        const image = await this.browsers.screenshot(pageId, screenshotOptions);
        response.writeHead(200, {
          "content-type": "image/" + format,
          "content-length": image.byteLength,
          "content-disposition": "inline; filename=\"" + pageId + "." + format + "\"",
          "cache-control": "no-store",
        });
        response.end(image);
        return;
      }
      if (method === "POST" && kind === "snapshots") {
        const input = await this.optionalBody<{
          format?: "png" | "jpeg";
          quality?: number;
          full_page?: boolean;
        }>(request);
        if (input.format !== undefined && input.format !== "png" && input.format !== "jpeg") {
          throw new HttpError(400, "invalid_format", "format must be png or jpeg");
        }
        if (input.quality !== undefined && (!Number.isInteger(input.quality) || input.quality < 0 || input.quality > 100)) {
          throw new HttpError(400, "invalid_quality", "quality must be between 0 and 100");
        }
        if (input.full_page !== undefined && typeof input.full_page !== "boolean") {
          throw new HttpError(400, "invalid_full_page", "full_page must be a boolean");
        }
        const snapshotOptions: {
          format?: "png" | "jpeg";
          quality?: number;
          fullPage?: boolean;
        } = { format: input.format ?? "jpeg" };
        if (input.quality !== undefined) snapshotOptions.quality = input.quality;
        if (input.full_page !== undefined) snapshotOptions.fullPage = input.full_page;
        this.json(response, 201, await this.browsers.createManualSnapshot(pageId, snapshotOptions));
        return;
      }
    }

    const snapshotMatch = /^\/internal\/snapshots\/([^/]+)$/.exec(pathname);
    if (snapshotMatch?.[1]) {
      const snapshotId = decodeURIComponent(snapshotMatch[1]);
      if (method === "GET") {
        const snapshot = this.browsers.snapshots.get(snapshotId);
        if (!snapshot) throw new HttpError(404, "snapshot_not_found", "snapshot not found");
        const details = await stat(snapshot.imagePath);
        response.writeHead(200, {
          "content-type": "image/" + snapshot.metadata.format,
          "content-length": details.size,
          "content-disposition": "inline; filename=\"" + snapshot.metadata.id + "." + snapshot.metadata.format + "\"",
          "cache-control": "no-store",
        });
        createReadStream(snapshot.imagePath).pipe(response);
        return;
      }
      if (method === "DELETE") {
        if (!await this.browsers.snapshots.delete(snapshotId)) {
          throw new HttpError(404, "snapshot_not_found", "snapshot not found");
        }
        response.writeHead(204).end();
        return;
      }
    }

    throw new HttpError(404, "not_found", "route not found");
  }

  private async optionalBody<T extends object>(request: IncomingMessage): Promise<T> {
    if (request.headers["content-length"] === "0" || request.headers["content-length"] === undefined) {
      return {} as T;
    }
    return await this.body<T>(request);
  }

  private async body<T>(request: IncomingMessage): Promise<T> {
    const chunks: Buffer[] = [];
    let size = 0;
    for await (const chunk of request) {
      const buffer = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk);
      size += buffer.byteLength;
      if (size > 1_048_576) throw new HttpError(413, "body_too_large", "request body exceeds 1 MiB");
      chunks.push(buffer);
    }
    if (size === 0) throw new HttpError(400, "invalid_json", "request body is required");
    try {
      const parsed: unknown = JSON.parse(Buffer.concat(chunks).toString("utf8"));
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
        throw new Error("expected an object");
      }
      return parsed as T;
    } catch (error) {
      throw new HttpError(400, "invalid_json", asError(error).message);
    }
  }

  private quality(raw: string | null): number | undefined {
    if (raw === null) return undefined;
    const quality = Number(raw);
    if (!Number.isInteger(quality) || quality < 0 || quality > 100) {
      throw new HttpError(400, "invalid_quality", "quality must be between 0 and 100");
    }
    return quality;
  }

  private booleanQuery(raw: string | null, name: string): boolean {
    if (raw === null || raw === "false") return false;
    if (raw === "true") return true;
    throw new HttpError(400, "invalid_" + name, name + " must be true or false");
  }

  private json(response: ServerResponse, status: number, value: unknown): void {
    const body = Buffer.from(JSON.stringify(value) + "\n");
    response.writeHead(status, {
      "content-type": "application/json",
      "content-length": body.byteLength,
    });
    response.end(body);
  }

  private text(response: ServerResponse, status: number, contentType: string, value: string): void {
    const body = Buffer.from(value);
    response.writeHead(status, {
      "content-type": contentType,
      "content-length": body.byteLength,
    });
    response.end(body);
  }

  private failure(response: ServerResponse, value: unknown): void {
    if (response.headersSent) {
      response.destroy(asError(value));
      return;
    }
    const error = asError(value);
    let status = 500;
    let code = "worker_internal_error";
    if (value instanceof HttpError) {
      status = value.status;
      code = value.code;
    } else if (error.name === "TimeoutError") {
      status = 504;
      code = "playwright_timeout";
    }
    log(status >= 500 ? "error" : "warn", "worker request failed", { status, code, error: error.message });
    this.json(response, status, {
      type: "https://scraper.local/problems/" + code,
      title: code.replaceAll("_", " "),
      status,
      code,
      detail: error.message,
    });
  }
}
