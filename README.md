# Browser Scraping Infrastructure

A self-hosted control plane for interactive, headed Chromium sessions. Clients use one Go HTTP service, `scrape-api`; it schedules sessions onto TypeScript/Playwright workers and proxies browser automation, screenshots, diagnostics, and noVNC access without exposing worker APIs to clients.

> [!WARNING]
> This version intentionally has no authentication. It is for a trusted LAN only. The JavaScript evaluation endpoint executes arbitrary code in a page, profile files can contain proxy credentials, and a worker's noVNC display is shared by every session on that worker. Do not expose this service or worker ports to the internet.

The detailed design decisions and acceptance criteria are recorded in [plan.md](plan.md).

## What is implemented

- A central Go/chi API with in-memory worker, session, page, and snapshot routing state.
- A strict-TypeScript worker controlling one independent, always-headed Chromium process and clean temporary user-data directory per session.
- Multiple pages per session and Playwright-backed navigate, click, type, press, select, wait, and evaluate operations.
- HTML, URL, title, live screenshot, retained manual snapshot, and bounded diagnostic APIs.
- Worker self-registration, heartbeats, drain/resume, offline detection, least-loaded placement, and incarnation-aware reconciliation.
- Configurable per-session idle expiry. API/browser activity and actual noVNC input refresh it; monitoring traffic does not.
- Periodic screenshots, longer-lived error screenshots, manual snapshots, retention cleanup, and a worker storage ceiling.
- JSON profile templates stored by `scrape-api`, with clean runtime profiles for every new session.
- A server-rendered dashboard with separate Go templates, CSS, and JavaScript, plus an embedded pinned noVNC client.
- Self-contained ReDoc API reference pages generated from public and internal OpenAPI 3.1 contracts.
- Native local Docker Compose images (including arm64) and a deterministic headed-browser end-to-end suite.
- Proxmox VE 8.3.5 host-local automation for latest-Debian, unprivileged LXC templates and create/list/delete/reconcile workflows.
- GitHub Actions for Go, TypeScript, OpenAPI, shell, LXC-helper, multi-architecture image, and Compose integration checks.

There is deliberately no database, durable session history, authentication, queue, scheduler, proxy UI, CAPTCHA solver, fingerprint spoofing, Kubernetes integration, or autoscaling.

## Architecture

```text
trusted-LAN client
        |
        | HTTP / WebSocket
        v
  scrape-api :8080
  - public REST API
  - in-memory registry
  - profile files
  - dashboard + noVNC client
        |
        | internal HTTP only
        +------------------------+------------------------+
        v                        v                        v
 scrape-01 worker        scrape-02 worker        scrape-03 worker
 Xvfb + Openbox          Xvfb + Openbox          Xvfb + Openbox
 Playwright Chromium     Playwright Chromium     Playwright Chromium
 x11vnc + websockify     x11vnc + websockify     x11vnc + websockify
```

The production sizing target is three worker LXCs, three concurrent sessions per worker, and three pages per session. Those numbers are operational sizing signals, not application-enforced limits. Worker selection is least-loaded with a stable worker-ID tie break; draining workers keep existing sessions but receive no new ones.

## Local quick start

Prerequisites are Docker with Compose v2. The verification scripts additionally use `curl`, `jq`, Node.js, and `rg`.

```bash
cp .env.example .env       # optional overrides
docker compose up --build --wait
```

Open:

- Dashboard: <http://localhost:8080/>
- Public API reference: <http://localhost:8080/static/docs/public.html>
- Internal protocol reference: <http://localhost:8080/static/docs/worker.html>
- Health: <http://localhost:8080/healthz>
- Readiness and online-worker count: <http://localhost:8080/readyz>

Stop the stack with:

```bash
docker compose down --volumes --remove-orphans
```

The Compose build is native to the host architecture and has been designed for the requested arm64 development environment. CI also builds both service images for linux/arm64 and linux/amd64.

## First API workflow

Create a session with the bundled generic profile. Omitting `idle_timeout_seconds` uses the configured 10-minute default.

```bash
session_id="$({
  curl -fsS http://localhost:8080/v1/sessions \
    -H 'content-type: application/json' \
    -d '{"browser":"chromium","profile":"generic","idle_timeout_seconds":600}'
} | jq -r .id)"
```

Create a page and interact with it:

```bash
page_id="$({
  curl -fsS "http://localhost:8080/v1/sessions/$session_id/pages" \
    -H 'content-type: application/json' \
    -d '{"url":"https://example.com"}'
} | jq -r .id)"

curl -fsS "http://localhost:8080/v1/pages/$page_id/title"
curl -fsS "http://localhost:8080/v1/pages/$page_id/evaluate" \
  -H 'content-type: application/json' \
  -d '{"expression":"document.querySelector(\"h1\")?.textContent"}'
curl -fsS "http://localhost:8080/v1/pages/$page_id/screenshot?format=jpeg&quality=60" \
  --output page.jpg
```

Open `http://localhost:8080/v1/sessions/$session_id/vnc` for live browser control. Terminate the session when finished:

```bash
curl -fsS -X DELETE "http://localhost:8080/v1/sessions/$session_id"
```

The full public contract, request schemas, filters, response bodies, and problem responses are in [scrape-api/api/public.openapi.yaml](scrape-api/api/public.openapi.yaml).

## Session and recovery semantics

A session owns one persistent Playwright browser context and Chromium process while it is running. Its tabs are pages inside that context. The worker uses a fresh temporary user-data directory for every session and removes it at termination, so cookies, local storage, cache, downloads, and login state do not survive into a later session. Reauthentication after worker or session restart is expected.

Runtime ownership is intentional:

- `scrape-api` keeps current routing metadata in memory.
- Each worker is authoritative for its active sessions and pages.
- A worker sends its full live state on registration and every heartbeat.
- After an API restart, worker registration reconstructs the central registry.
- A worker restart changes its incarnation and loses all of that worker's sessions.
- Session deletion removes both browser state and central routing metadata; there is no historical session store.
- JSON profile definitions persist on the central filesystem. Worker snapshots persist on the worker filesystem according to their retention class.

The default idle timeout is 600 seconds and can be overridden per `POST /v1/sessions`. Explicit session/page API operations, Playwright navigation/network activity, and keyboard/pointer input from the embedded noVNC client count as activity. Heartbeats, periodic screenshots, dashboard polling, and passive VNC framebuffer updates do not.

## Profiles

Profile definitions live as individual `<name>.json` files in `PROFILES_DIR`. `GET /v1/profiles`, `GET /v1/profiles/{name}`, and `POST /v1/profiles` are supported; updating and deleting profiles are intentionally not part of v1. Names must match `^[a-z0-9][a-z0-9-]{0,62}$`, files are created atomically, and duplicates return HTTP 409.

Example:

```json
{
  "name": "desktop-de",
  "description": "German desktop context",
  "browser": "chromium",
  "userAgent": "Mozilla/5.0 ...",
  "locale": "de-DE",
  "timezoneId": "Europe/Berlin",
  "viewport": { "width": 1920, "height": 1080 },
  "deviceScaleFactor": 1,
  "colorScheme": "light",
  "javaScriptEnabled": true,
  "ignoreHTTPSErrors": false,
  "acceptDownloads": true,
  "geolocation": { "latitude": 52.52, "longitude": 13.405, "accuracy": 50 },
  "permissions": ["geolocation"],
  "extraHTTPHeaders": { "x-client": "scraper" },
  "proxy": { "server": "http://proxy.lan:3128", "bypass": "localhost" }
}
```

Only explicitly typed Playwright browser-context settings are accepted. Profiles are immutable templates, not browser-data directories. The generic profile is in [scrape-api/profiles/generic.json](scrape-api/profiles/generic.json); the complete field reference is in [docs/profiles.md](docs/profiles.md).

## Monitoring and diagnostics

The worker records bounded events for console errors, uncaught page errors, failed requests, navigation errors, Playwright/action errors, and unexpected browser closure. Error events from page/action failures can reference retained screenshots. Events include the active session/page IDs, URL, timestamp, message, and safe context; public responses do not expose stack traces or worker connection URLs.

Screenshot classes are:

| Class | Default retention | Access |
| --- | --- | --- |
| `periodic` | 10 minutes | Used for monitoring and automatically cleaned |
| `error` | 24 hours | Routed through `scrape-api` while retained |
| `manual` | Until explicit deletion | Created and fetched by opaque snapshot ID |

`SCREENSHOT_MAX_BYTES` bounds total worker storage. Oldest periodic/error images are evicted first; an attempted manual snapshot is rejected if retained manual images leave no room. Existing manual images have no time expiry and are never silently evicted. A manual or error snapshot can outlive its session until deletion/expiry, but it does not preserve a discoverable historical session record. Restarting a worker preserves snapshot files on its filesystem/volume and republishes routable manual/error metadata; deleting the worker LXC loses them.

## Dashboard and noVNC

The dashboard is rendered by Go and uses maintainable source files under `scrape-api/web/templates`, `scrape-api/web/static/css`, and `scrape-api/web/static/js`. It shows worker state/resources, active sessions, pages, current screenshots, recent diagnostics, profile management, terminate controls, and an Open Browser button.

The noVNC browser and WebSocket are both served through `scrape-api`; clients never connect to x11vnc/websockify directly. Xvfb is shared per worker to reduce overhead, so the live view can show windows belonging to other sessions on the same worker. Browser processes and their temporary profiles remain separate.

## Configuration

All runtime settings are environment variables. Durations accept Go duration syntax in `scrape-api` and positive `ms`, `s`, `m`, or `h` values in the worker.

### scrape-api

| Variable | Default | Purpose |
| --- | --- | --- |
| `SERVER_ADDRESS` | `:8080` | HTTP listen address |
| `PROFILES_DIR` | `./profiles` | Authoritative JSON profile directory |
| `WORKER_REQUEST_TIMEOUT` | `30s` | Central-to-worker HTTP timeout |
| `WORKER_TIMEOUT` | `30s` | Heartbeat age before a worker is offline |
| `DEFAULT_IDLE_TIMEOUT` | `10m` | Session default when omitted |
| `DASHBOARD_POLL_INTERVAL` | `3s` | Dashboard refresh period |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |

### worker

| Variable | Default | Purpose |
| --- | --- | --- |
| `WORKER_ID` | hostname | Stable worker identifier |
| `WORKER_HOSTNAME` | hostname | Display hostname |
| `WORKER_ADDRESS` / `WORKER_PORT` | `0.0.0.0` / `8081` | Internal API listener |
| `WORKER_BASE_URL` | derived | URL used by `scrape-api` |
| `WORKER_VNC_URL` | derived | Internal websockify URL |
| `SCRAPE_API_URL` | `http://scrape-api:8080` | Registration/heartbeat target |
| `WORKER_CAPACITY` | `3` | Dashboard/sizing target, not a hard cap |
| `WORKER_HEARTBEAT_INTERVAL` | `10s` | Registration/heartbeat interval |
| `ACTION_TIMEOUT` | `30s` | Default and maximum browser action timeout |
| `MAX_EVENTS` | `500` | Per-session diagnostic ring size |
| `SCREENSHOT_INTERVAL` | `10s` | Periodic capture interval |
| `SCREENSHOT_RETENTION` | `10m` | Periodic image retention |
| `ERROR_SCREENSHOT_RETENTION` | `24h` | Error image retention |
| `SCREENSHOT_MAX_BYTES` | `1073741824` | Per-worker image storage ceiling |
| `SCREENSHOT_ROOT` | `./screenshots` | Worker image/sidecar directory |
| `SESSION_ROOT` | `./sessions` | Temporary user-data parent |
| `DISPLAY` | `:99` | Shared X display |
| `DISPLAY_RESOLUTION` / `DISPLAY_DEPTH` | `1920x1080` / `24` | Xvfb geometry |

Compose overrides are documented in [.env.example](.env.example). Proxmox deployment values are separate in [deploy/proxmox/config.example.env](deploy/proxmox/config.example.env).

## Development and validation

Go 1.24 and Node.js 20 or newer are required outside Docker.

```bash
cd scrape-api
npm ci --ignore-scripts
npm run generate             # pinned noVNC + self-contained API docs
npm run lint:openapi
go test -race ./...
go vet ./...
cd ..

cd worker
npm ci --ignore-scripts
npm run lint
npm test
```

The Compose acceptance suite uses real headed Chromium:

```bash
docker compose up --build --wait
./test/e2e/smoke.sh
./test/e2e/recovery.sh
```

The smoke suite covers profiles, three pages, all browser actions, inspection, screenshots, diagnostics, dashboard routes, the noVNC WebSocket, deletion, and idle expiry. The recovery suite restarts each service to prove API reconciliation and worker-session loss semantics. See [docs/operations.md](docs/operations.md) for operating and troubleshooting details.

## Proxmox deployment

Production automation is intended to run directly as root on the Proxmox VE 8.3.5 host containing this repository. It selects the latest available Debian standard template, creates unprivileged amd64 LXCs on `Synology-Backup`, attaches them to DHCP on `vmbr0`, discovers guest IPs, injects service configuration, and uses systemd for supervision.

The normal flow is:

```bash
cd deploy/proxmox
cp config.example.env config.env
./build-api-template.sh
./build-worker-template.sh
./create-fleet.sh
./list.sh
```

Deletion is restricted to scraper-tagged CTIDs and requires typing the CTID unless `--force` is supplied. Full prerequisites, command behavior, validation, recovery, and rollback steps are in [docs/proxmox.md](docs/proxmox.md). The scripts are statically/test-double validated in CI; actual host execution remains an operator-run step.

## Repository map

| Path | Contents |
| --- | --- |
| `scrape-api/` | Central Go module, OpenAPI contracts, profiles, dashboard assets, and API Dockerfile |
| `worker/` | TypeScript Playwright module and worker Dockerfile |
| `deploy/docker/` | Worker graphical-process entrypoint |
| `deploy/systemd/` | Hardened guest service units |
| `deploy/proxmox/` | LXC template and lifecycle automation |
| `test/e2e/` | Real-browser Compose tests |
| `.github/workflows/ci.yml` | CI validation and architecture builds |

## Deferred extensions

The public REST API remains the source of truth so clients can be generated later. Future-compatible but unimplemented areas include job queues/scheduling, persistent cookies and session history, durable metadata storage, authentication/multi-user authorization, proxy management, CAPTCHA handling, browser fingerprint spoofing, Firefox/WebKit, public event streaming/session recording, Kubernetes, and automatic scaling.
