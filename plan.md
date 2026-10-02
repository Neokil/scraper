# Browser Scraping Infrastructure — Implementation Plan

## 1. Purpose and authority

This plan covers the complete first-version scope described in README.md: worker images, browser control, the central API, monitoring, dashboard, noVNC access, diagnostics, profile templates, automated Proxmox deployment, local development, and verification.

The decisions recorded below were made during the planning interview. Where they conflict with README.md, they are the implementation requirements. README.md must be revised during implementation so that it no longer contradicts this plan.

## 2. Confirmed product decisions

- The full README v1 Definition of Done is in scope. The future job/queue system, scheduled scraping, CAPTCHA solving, fingerprint spoofing, proxy-management UI, Kubernetes, and autoscaling remain out of scope.
- The deployment is a trusted, single-tenant LAN service. V1 has no API or dashboard authentication. This explicitly supersedes README section 19.
- scrape-api is the only client-facing entry point. Worker HTTP, VNC, Playwright, Chromium, and X11 ports must not be exposed as public application APIs.
- The central service is Go using chi. The worker is strict TypeScript/Node.js using Playwright.
- Public and internal APIs are OpenAPI-first. Specifications, examples, error models, and generated artifacts are checked into the repository and verified for drift.
- There is no PostgreSQL or other external database in v1. Live metadata is held in memory. This explicitly supersedes README sections 18 and 25 Phase 5.
- A restarted central API reconstructs workers, sessions, and pages from workers. A restarted worker loses all of its sessions.
- Closed, expired, and lost sessions disappear immediately from the API and dashboard; v1 has no historical session list.
- Every session gets its own Chromium process and temporary user-data directory. All sessions on one worker share that worker's Xvfb, Openbox, x11vnc, and noVNC display.
- Browsers always run headed. The public API will not offer a functional headless mode in v1.
- Profiles are clean JSON configuration templates, not persistent Chromium user-data directories. Every session starts clean; cookies, login state, and local storage are not retained for the next session.
- scrape-api owns the authoritative profile directory. V1 supports listing and creating profiles, not updating or deleting them. Workers receive a validated profile snapshot when a session is created.
- A built-in generic profile is always available. Profile settings use a documented typed allowlist rather than arbitrary Playwright option passthrough.
- The default idle timeout is 10 minutes and can be set per session. API use, browser navigation/network activity, and actual keyboard or pointer input through noVNC reset it.
- Periodic screenshots default to 10-second capture and 10-minute retention. Error screenshots default to 24-hour retention. Manual snapshots remain until explicitly deleted. All values are configurable.
- Worker-local files hold screenshots and diagnostics. Deleting a worker LXC deletes those artifacts.
- Production consists of one scrape-api LXC and initially three worker LXCs.
- Three sessions per worker and three pages per session are sizing targets, not enforced limits. Least-loaded placement continues even when every online worker is beyond its advertised target capacity.
- Proxmox is version 8.3.5. Containers are unprivileged, use DHCP on vmbr0, and use Synology-Backup storage. Other deployment values are configurable.
- Proxmox lifecycle scripts run directly on the Proxmox host, discover container IP addresses, and use the Proxmox CLI.
- Local development and end-to-end validation use Docker Compose on arm64. Production artifacts target amd64.
- CI uses GitHub Actions.

## 3. Target architecture

~~~text
LAN clients
    |
    | HTTP / WebSocket
    v
scrape-api LXC
    |-- public REST API
    |-- server-rendered dashboard
    |-- static CSS and JavaScript
    |-- embedded noVNC client
    |-- VNC WebSocket proxy
    |-- in-memory worker/session/page registry
    |-- filesystem profile definitions
    |
    | internal HTTP/JSON generated from OpenAPI
    |
    +--> worker-01 LXC
    +--> worker-02 LXC
    +--> worker-03 LXC

Each worker:
    systemd
      |-- Xvfb :99
      |-- Openbox
      |-- x11vnc / websockify
      +-- TypeScript worker service
             |-- Chromium process + temp profile for session A
             |-- Chromium process + temp profile for session B
             +-- Chromium process + temp profile for session C
~~~

The worker implementation is hidden behind an internal client interface in scrape-api. That interface must be narrow enough to replace LXC workers with another backend later without changing the public REST API.

## 4. Repository layout

Use a monorepo with explicit service, contract, deployment, and documentation boundaries:

~~~text
/
├── scrape-api/                  Self-contained central Go service
│   ├── cmd/scrape-api/          Go entry point
│   ├── internal/
│   │   ├── api/                 Public HTTP handlers
│   │   ├── dashboard/           Dashboard handlers and view models
│   │   ├── profiles/            JSON profile repository and validation
│   │   ├── registry/            Registry, scheduling, and reconciliation
│   │   └── workerclient/        Internal worker protocol adapter
│   ├── api/                     Public and worker OpenAPI contracts
│   ├── profiles/                Bundled JSON profile definitions
│   ├── web/
│   │   ├── templates/           Separate Go HTML templates
│   │   ├── static/css/          Separate CSS files
│   │   ├── static/js/           Separate maintainable JavaScript files
│   │   └── static/vendor/novnc/ Pinned noVNC client assets
│   ├── scripts/                 Asset generation and contract validation
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   ├── package.json
│   └── package-lock.json
├── worker/
│   ├── src/                     TypeScript API, browser, diagnostics, and storage
│   ├── Dockerfile
│   ├── package.json
│   └── tsconfig.json
├── deploy/
│   ├── docker/
│   ├── proxmox/
│   └── systemd/
├── test/
│   ├── fixture-site/
│   ├── contract/
│   └── e2e/
├── .github/workflows/
├── README.md
└── plan.md
~~~

Generated files must contain a header identifying their source and regeneration command. Generation must be deterministic.

## 5. API contracts

### 5.1 Contract rules

- The checked-in OpenAPI documents are the source of truth.
- Use stable JSON field names, UTC RFC 3339 timestamps, opaque IDs with resource prefixes, and documented enums.
- Use application/problem+json errors with a stable machine-readable code, human-readable detail, request ID, and relevant resource IDs.
- Validate requests at the central boundary and again at the worker boundary where worker safety requires it.
- Put bounded timeouts on central-to-worker requests and Playwright operations.
- Do not expose raw Playwright objects, worker URLs, display numbers, ports, filesystem paths, or stack traces.
- The public API may report the assigned worker for monitoring, but clients must never need it to address a session or page.

### 5.2 Public API

Document and implement these endpoint groups under /v1:

- Health:
  - GET /healthz
  - GET /readyz
- Workers:
  - GET /v1/workers
  - GET /v1/workers/{workerId}
  - POST /v1/workers/{workerId}/drain
  - POST /v1/workers/{workerId}/resume
- Sessions:
  - POST /v1/sessions
  - GET /v1/sessions
  - GET /v1/sessions/{sessionId}
  - DELETE /v1/sessions/{sessionId}
- Pages:
  - POST /v1/sessions/{sessionId}/pages
  - GET /v1/sessions/{sessionId}/pages
  - GET /v1/pages/{pageId}
  - DELETE /v1/pages/{pageId}
- Page actions:
  - POST /v1/pages/{pageId}/navigate
  - POST /v1/pages/{pageId}/click
  - POST /v1/pages/{pageId}/type
  - POST /v1/pages/{pageId}/press
  - POST /v1/pages/{pageId}/select
  - POST /v1/pages/{pageId}/wait
  - POST /v1/pages/{pageId}/evaluate
- Page inspection:
  - GET /v1/pages/{pageId}/html
  - GET /v1/pages/{pageId}/screenshot
  - GET /v1/pages/{pageId}/url
  - GET /v1/pages/{pageId}/title
- Profiles:
  - GET /v1/profiles
  - GET /v1/profiles/{profileName}
  - POST /v1/profiles
- Diagnostics:
  - GET /v1/sessions/{sessionId}/events
  - GET /v1/pages/{pageId}/events
  - POST /v1/pages/{pageId}/snapshots
  - GET /v1/snapshots/{snapshotId}
  - DELETE /v1/snapshots/{snapshotId}
- Live access:
  - GET /v1/sessions/{sessionId}/vnc
  - GET /v1/sessions/{sessionId}/vnc/websocket
  - POST /v1/sessions/{sessionId}/activity for explicit noVNC input notifications

Session creation accepts browser, optional profile, and idle_timeout_seconds. Browser is documented as chromium-only, headless is omitted, and an omitted idle timeout uses 600 seconds.

List-session filters support status and worker. Action schemas expose Playwright's relevant selector, value, timeout, and wait-until semantics without creating a second selector language.

Screenshot responses support full_page, png or jpeg, and configurable JPEG quality. Binary endpoints set correct content type, cache policy, and filename headers.

JavaScript evaluation accepts an expression and returns only JSON-serializable results. Serialization failures, thrown page errors, and timeouts use documented problem codes. The OpenAPI description must prominently warn that this is arbitrary code execution in the visited page.

### 5.3 Internal worker API

Define a separate, non-public OpenAPI contract covering:

- Worker registration and periodic heartbeats.
- A worker incarnation ID that changes on every worker-service start.
- Complete state reconciliation after scrape-api restarts.
- Worker status and resource usage.
- Session create, get, list, and delete.
- Page create, get, list, delete, actions, and inspection.
- Screenshot and event retrieval.
- Shared VNC connection metadata used only by the central proxy.

The worker registers on startup and retries with bounded exponential backoff. It also re-registers if a heartbeat receives an unknown-worker response. Registration and reconciliation are idempotent.

Draining is central scheduling state: a draining worker keeps and serves its existing sessions but receives no newly scheduled sessions. Resume returns it to online scheduling once it is heartbeating normally.

## 6. Runtime state and recovery

### 6.1 Central in-memory model

Maintain concurrency-safe registries for:

- Workers: identity, incarnation, address, version, status, capacity target, resources, last heartbeat.
- Sessions: identity, worker, status, selected profile, creation time, last activity, idle timeout.
- Pages: identity, owning session, URL, title, status, timestamps.
- Active diagnostics and snapshot references.

Keep HTTP handlers thin. State transitions belong in services with deterministic unit tests.

### 6.2 Worker model

The worker is authoritative for browser processes currently running on it. It keeps enough live metadata to return a complete reconciliation snapshot containing sessions, pages, timestamps, and activity state.

When scrape-api restarts:

1. Its registries start empty.
2. Workers retry registration or are told to register by their next heartbeat.
3. scrape-api fetches and validates each worker's complete state.
4. It rebuilds session/page routing plus surviving manual-snapshot routing and resumes normal operation.
5. Duplicate or inconsistent IDs are rejected and clearly logged.

When a worker restarts or its incarnation changes:

1. All sessions previously assigned to that worker are removed immediately.
2. Their temporary browser data and active diagnostics are considered lost.
3. The worker registers as a fresh instance with no sessions.

If heartbeats stop for the configurable timeout, mark the worker offline and remove its sessions. Default heartbeat interval is 10 seconds and timeout is 30 seconds. A returning worker reconciles as above.

No historical tombstones are retained after close, expiry, or loss.

## 7. Scheduling and lifecycle

- Select a non-draining online worker using the lowest active-session count, with stable tie-breaking.
- Treat advertised capacity as monitoring/sizing information, not an eligibility ceiling.
- Return 503 with a stable no_available_worker error only when no worker is online.
- Generate session and page IDs centrally so public routing never depends on worker-local IDs.
- Make create/delete transitions idempotent where feasible and compensate for partial failures.
- On session deletion or idle expiry, instruct the worker to close Chromium, delete the temporary user-data directory, stop timers/listeners, and release screenshot resources before removing central routing.
- On page deletion, close the Playwright page and release page-specific timers/listeners.

Each session launches a separate headed Chromium process controlled by the Playwright version pinned in the worker package. Use Playwright's matching managed Chromium build, not Debian's separately versioned Chromium package.

All Chromium windows share DISPLAY=:99 on a worker. Openbox should make session IDs visible in window titles or another unobtrusive debugging aid. The dashboard must clearly disclose that the noVNC view is worker-shared and can show other sessions on that worker.

## 8. Idle activity semantics

The session create request can override idle_timeout_seconds; omission uses the configurable 600-second default.

Reset last activity for:

- User-requested public operations scoped to the session or one of its pages.
- Page navigation and network activity reported by Playwright.
- Keyboard and pointer input emitted by the embedded noVNC client.

Do not reset it for:

- Scheduled screenshot capture.
- Worker heartbeat or state reconciliation.
- Automatic dashboard polling.
- VNC framebuffer traffic that contains no user input.

The worker records browser activity closest to the source and reports the timestamp. scrape-api records API/noVNC activity and forwards it to the worker. Use monotonic timers within a process and UTC timestamps across services. Expiry must be safe under duplicate notifications and moderate clock skew.

## 9. Profile templates

Store authoritative definitions as individual JSON files in a configurable scrape-api directory. Validate names to prevent path traversal, write new files atomically, reject duplicates with 409, and load/validate all files on startup.

The typed v1 schema should include:

- name and optional description
- browser enum, initially only chromium
- userAgent
- locale and timezoneId
- viewport, deviceScaleFactor, isMobile, and hasTouch
- colorScheme and reducedMotion
- javaScriptEnabled, ignoreHTTPSErrors, and acceptDownloads
- geolocation and permissions
- extraHTTPHeaders
- proxy server, bypass list, username, and password

Only include settings that can be mapped deliberately to Playwright launch or browser-context options. Do not accept arbitrary launch arguments or an untyped options object.

The generic profile is part of source control and bundled into the scrape-api image. POST /v1/profiles creates additional JSON definitions on the central filesystem. When creating a session, scrape-api resolves the selected profile and sends an immutable validated snapshot to the worker.

The worker creates a fresh temporary user-data directory for every session and deletes it at termination. Runtime cookies, storage, cache, downloads, and preferences never update the source profile.

## 10. Screenshots, diagnostics, and retention

### 10.1 Screenshots

Each worker captures every active page on a configurable interval, default 10 seconds. Avoid overlapping captures of a slow page. Store image files and small sidecar metadata on the worker filesystem, never in central memory.

Implement retention classes:

- periodic: configurable, default 10 minutes
- error: configurable, default 24 hours
- manual: no time expiry; explicit deletion

Run bounded cleanup at startup and periodically. Enforce a configurable total storage ceiling with oldest-first eviction for periodic and error classes. Surface capture failures and storage pressure without killing the browser session.

The latest screenshot is available to the dashboard through scrape-api. Manual snapshots receive opaque IDs. A manual snapshot remains addressable by its ID after the session closes until deletion or worker removal; it must not preserve a discoverable historical session record.

Worker reconciliation includes an inventory of surviving manual snapshot IDs so scrape-api can restore their worker routing after a central restart without recreating terminated session records.

### 10.2 Events and logs

Capture bounded per-session/page ring buffers for:

- browser console errors
- uncaught page errors
- failed network requests
- navigation errors
- Playwright action and timeout errors
- worker lifecycle failures
- screenshot-on-error references

Include timestamp, type, message, URL, session/page IDs while active, and safe structured context. Do not return stack traces or proxy credentials in public responses. Recent worker/service logs shown by the dashboard should be bounded and sanitized.

## 11. Dashboard and noVNC

Serve the dashboard from scrape-api. Use server-rendered Go templates plus small vanilla JavaScript modules; do not introduce a separate SPA framework.

Keep source files cleanly separated:

- HTML templates in scrape-api/web/templates
- styles in scrape-api/web/static/css
- behavior in scrape-api/web/static/js
- pinned noVNC assets in scrape-api/web/static/vendor/novnc

They may be embedded into the Go binary for deployment with go:embed, but must remain separate maintainable source files.

Dashboard views:

- Worker overview with online/offline/draining state, resources, target capacity, and active sessions.
- Session overview with worker, status, pages, last activity, and idle deadline.
- Session detail with all tabs, URLs, titles, latest screenshots, resources, recent events/logs, terminate action, and Open Browser action.
- Profile list/create form with client- and server-side validation.

Use lightweight polling for monitoring state in v1. Public real-time event WebSockets remain a future extension. Polling must not count as session activity.

The embedded noVNC client connects only to scrape-api. scrape-api resolves the session to its worker and proxies the WebSocket to that worker's shared VNC service. Reject inactive or unknown sessions. The client sends separate throttled activity notifications only for keyboard/pointer input; ordinary framebuffer updates do not keep a session alive.

Because the display is shared per worker, opening any session's browser view exposes the worker desktop, not an isolated per-session desktop. Display this fact in the UI.

## 12. Configuration

Use environment variables for services and a documented environment/config file for Proxmox scripts. Provide checked-in example files without secrets.

Central configuration includes:

- listen address and public base URL
- profile directory
- worker request timeout
- heartbeat/offline thresholds
- default and allowed session idle timeouts
- dashboard polling interval
- log level and request ID settings

Worker configuration includes:

- worker ID, version, and advertised target capacity
- scrape-api internal URL
- internal listen address
- display, resolution, and color depth
- heartbeat interval
- screenshot interval, retention classes, storage root, and storage ceiling
- Playwright action/navigation maximum timeouts
- temporary session root

Proxmox configuration includes:

- node name or automatic local-node detection
- storage, default Synology-Backup
- bridge, default vmbr0
- DHCP networking
- CTID ranges or explicit IDs
- template and instance names
- API and worker CPU, memory, swap, root disk, and start-at-boot values
- number of workers, default 3
- source/release paths and service ports

Starting resource defaults should be conservative but viable for the agreed 3-by-3 sizing target and remain easy to override:

- scrape-api: 1 vCPU, 1 GiB RAM, 8 GiB root disk
- worker: 4 vCPU, 6 GiB RAM, 16 GiB root disk

These are initial defaults, not performance guarantees; the Proxmox smoke-test guide must explain how to tune them from observed memory and CPU use.

## 13. Local Docker Compose environment

Provide a one-command local stack that runs on arm64 and mirrors production behavior closely:

- scrape-api
- one browser worker with Xvfb/Openbox/x11vnc/noVNC
- a deterministic local fixture website
- named development volumes for profile JSON and screenshots where useful
- health checks and startup ordering

Use architecture-neutral Dockerfiles and pin compatible Go, Node, Playwright, noVNC, and OS dependencies. Production is built natively for amd64 on Proxmox; local images run natively on arm64. Avoid architecture-specific binary downloads without selecting by target architecture.

The Compose environment must exercise headed Chromium, screenshots, browser actions, reconciliation, and the noVNC WebSocket path. It is the primary automated end-to-end validation environment because implementation will not have access to the Proxmox host.

## 14. Proxmox image and lifecycle automation

Place defensive Bash scripts in deploy/proxmox. They run directly on the Proxmox host and use pct, pveam, and other standard Proxmox CLI tools.

Required commands/scripts:

- Build or refresh the scrape-api template.
- Build or refresh the browser-worker template.
- Create the scrape-api LXC.
- Create one worker or the configured three-worker fleet.
- List project templates/instances with CTID, role, state, discovered IP, resources, and version.
- Delete an explicitly selected project instance.
- Reconcile worker configuration if the scrape-api IP changes.

Image build flow:

1. Resolve the latest compatible Debian stable LXC base available to the Proxmox host.
2. Record the exact resolved template name/version in build output and image metadata.
3. Create a temporary unprivileged build container.
4. Install pinned runtime dependencies and service artifacts.
5. Install the Playwright-managed Chromium build and its Linux dependencies in the worker image.
6. Install systemd units for scrape-api or the worker plus graphical services.
7. Validate expected binaries, users, directories, permissions, and health commands.
8. Sanitize machine-specific state and convert the result to a named template.
9. Clean the temporary builder only after successful template creation.

Creation flow:

1. Validate that storage supports the required template/rootdir content and that vmbr0 exists.
2. Allocate or validate exact CTIDs without colliding with existing guests.
3. Clone the appropriate unprivileged template with DHCP networking.
4. Apply configurable CPU, RAM, disk, feature, and start-at-boot settings.
5. Start scrape-api first and wait with a bounded timeout for DHCP.
6. Discover its IPv4 address from Proxmox/container state.
7. Inject that address into worker service configuration.
8. Create/start workers, discover their addresses, and wait for registration.
9. Print the dashboard URL and a concise verification summary.

Safety requirements:

- Tag/name every managed resource and restrict list/delete operations to those resources.
- Never delete by broad ranges, unresolved variables, or unvalidated globs.
- Show the exact CTID/name and require confirmation or an explicit force flag before deletion.
- Validate all prerequisites before mutating the host.
- Use traps to report partial failures without deleting pre-existing resources.
- Make repeated create/list operations idempotent where practical.
- Keep configurable defaults in a documented file rather than editing scripts.

Workers should bind their internal services only as needed on the LAN bridge. Document or automate firewall rules so only scrape-api and administration can reach worker HTTP/VNC ports. scrape-api remains the only application entry point.

No automated Proxmox execution is part of development acceptance. Instead, provide ShellCheck/Bats coverage with mocked CLI output and a precise operator-run smoke-test checklist for the real 8.3.5 host.

## 15. Implementation phases

### Phase 0 — Contracts and scaffold

- Establish Go module, worker TypeScript package, formatting, linting, test commands, and repository layout.
- Write the public and worker OpenAPI specifications with examples and problem responses.
- Select and wire deterministic Go and TypeScript code generation.
- Add configuration loaders, structured logging, request IDs, health endpoints, and version metadata.
- Add CI checks that generated code and documentation are current.

Exit criteria: both services compile, specifications validate, generated artifacts are reproducible, and health endpoints run locally.

### Phase 1 — Worker browser vertical slice

- Build the TypeScript worker and process supervisor.
- Start Xvfb, Openbox, x11vnc, websockify/noVNC dependencies.
- Launch one headed Playwright-managed Chromium process per session with a fresh temp profile.
- Implement session/page ownership and create/delete/list/get operations.
- Implement navigation, click, type, press, select, wait, evaluate, HTML, URL, title, and on-demand screenshot.
- Add strict cleanup on process exit and session deletion.

Exit criteria: direct worker integration tests control multiple isolated Chromium processes and multiple pages on one shared display.

### Phase 2 — Central registry, scheduling, and recovery

- Implement the in-memory registries and state transition services.
- Implement worker registration, heartbeats, incarnation handling, drain/resume, least-loaded placement, and offline detection.
- Implement generated worker client and public session/page routing.
- Implement central restart reconciliation and worker restart loss semantics.
- Add race, partial-failure, idempotency, and timeout tests.

Exit criteria: restarting scrape-api reconstructs live worker sessions; restarting a worker removes its sessions; clients never address a worker directly.

### Phase 3 — Complete public browser API

- Finish all interaction and inspection endpoints.
- Enforce action/navigation timeouts and map Playwright failures into stable problem responses.
- Add filters, validation, JSON evaluation serialization, binary screenshot behavior, and API examples.
- Add contract tests comparing running handlers with OpenAPI.

Exit criteria: every README browser operation is usable only through scrape-api and has documented success/error behavior.

### Phase 4 — Profiles and activity expiry

- Implement generic profile and typed JSON schema.
- Implement filesystem list/get/create with atomic writes and duplicate handling.
- Map validated profile settings deliberately into Playwright.
- Implement per-session idle timeout and all agreed activity sources.
- Ensure background monitoring does not accidentally reset idle timers.

Exit criteria: each session starts clean with the chosen profile settings, and configurable inactivity reliably closes and removes it.

### Phase 5 — Monitoring, screenshots, and diagnostics

- Add resource reporting, current browser/page state, capture scheduling, retention classes, and storage ceilings.
- Capture console, page, failed-request, navigation, action, and lifecycle errors.
- Take screenshots on errors.
- Expose bounded events, manual snapshots, latest screenshots, and status through scrape-api.

Exit criteria: active failures can be diagnosed from the central API without direct worker access and storage remains bounded.

### Phase 6 — Dashboard and embedded noVNC

- Build the server-rendered worker/session/profile views.
- Add separate CSS, template, and JavaScript sources.
- Add polling that does not count as activity.
- Vendor/pin the noVNC client and implement the central WebSocket proxy.
- Notify activity only on actual noVNC input.
- Add terminate controls and shared-display disclosure.

Exit criteria: a user can inspect state, see latest screenshots/errors, control the browser through scrape-api, and terminate sessions.

### Phase 7 — Docker development and end-to-end suite

- Build arm64-capable service images and Compose topology.
- Add a deterministic fixture site for action/error/network tests.
- Cover the complete API flow, multiple pages/sessions, screenshots, retention, expiry, noVNC handshake, API restart recovery, worker restart loss, and dashboard routes.
- Document one-command build, test, start, and cleanup workflows.

Exit criteria: the full Definition of Done passes locally in Docker Compose on arm64.

### Phase 8 — Proxmox templates and lifecycle scripts

- Add amd64 release builds, image provisioning, systemd units, and safe lifecycle scripts.
- Add IP discovery/injection and three-worker fleet creation.
- Validate scripts with mocked pct/pveam output, ShellCheck, and Bats.
- Write the real-host execution and rollback guide.

Exit criteria: automation is statically and behaviorally tested without host access and is ready for the operator-run Proxmox smoke test.

### Phase 9 — Documentation and release audit

- Update README.md to reflect every confirmed decision and remove obsolete database/auth/persistent-profile claims.
- Publish generated API reference plus example curl workflows.
- Document architecture, operations, configuration, troubleshooting, backup implications, and security assumptions.
- Run the acceptance matrix and record known limitations.

Exit criteria: documentation and implementation agree, CI is green, Docker acceptance passes on arm64, amd64 artifacts build, and the only deferred validation is the documented real-Proxmox smoke test.

## 16. Test and CI strategy

### Unit tests

- Registry transitions, scheduling, heartbeat/offline behavior, incarnation changes, and reconciliation.
- Per-session idle calculations and activity sources.
- Profile validation, atomic creation, path safety, and Playwright mapping.
- API validation/error mapping and worker-client timeouts.
- Screenshot retention and storage-ceiling eviction.
- Worker process/page ownership and cleanup.

### Contract and integration tests

- Validate both OpenAPI documents and examples.
- Assert generated code has no drift.
- Exercise Go handlers against fake workers.
- Exercise worker routes against real Playwright Chromium under Xvfb.
- Test concurrent sessions/actions and shutdown races.
- Test VNC proxy upgrade and routing.

### End-to-end tests

Against Docker Compose and the local fixture site:

1. Wait for scrape-api and worker readiness.
2. Create/list a profile.
3. Create a headed session with a short test timeout.
4. Create three pages and perform every action endpoint.
5. Evaluate JavaScript and retrieve HTML, URL, title, and screenshots.
6. Verify console/network/navigation error capture and error screenshots.
7. Verify dashboard pages and latest thumbnails.
8. Verify the noVNC WebSocket path and input activity notification.
9. Restart scrape-api and confirm session reconstruction.
10. Restart the worker and confirm sessions disappear.
11. Verify idle cleanup and configurable screenshot retention.
12. Close sessions and confirm no session history remains.

### GitHub Actions

Required jobs:

- Go format, vet, unit tests, race tests, and build.
- Worker formatting, linting, strict typecheck, unit tests, and build.
- OpenAPI linting, breaking-change awareness, and generated-code drift.
- ShellCheck and Bats tests for Proxmox scripts.
- Docker image builds for linux/amd64 and linux/arm64.
- Compose integration/end-to-end suite on an available supported runner architecture.
- Dependency and container vulnerability reporting without silently rewriting lockfiles.

Keep CI deterministic with pinned lockfiles/tool versions and cached, checksum-verified dependencies.

## 17. Documentation deliverables

- Revised README with accurate scope, architecture, quick start, and limitations.
- Public and internal OpenAPI specifications with examples.
- Generated browsable API reference.
- Local Docker Compose guide.
- Profile schema/reference with complete examples.
- Configuration reference for both services and deployment scripts.
- Proxmox 8.3.5 build/create/list/delete/reconcile guide.
- Operator-run real-host smoke-test and rollback checklist.
- Troubleshooting guide for browser launch, Xvfb, VNC, DHCP discovery, worker registration, screenshots, and session recovery.
- Security note explaining trusted-LAN assumptions, arbitrary page JavaScript, shared worker displays, plaintext profile proxy credentials, and why the service must not be exposed to untrusted networks.

## 18. Acceptance matrix

V1 is complete when the Docker Compose environment demonstrates that a client can:

1. Reach scrape-api as the sole entry point.
2. Create a clean headed Chromium session using a selected profile.
3. Have the least-loaded online worker selected automatically.
4. Create and control multiple tabs.
5. Navigate, click, type, press, select, and wait.
6. Execute JavaScript and receive JSON-serializable results.
7. Read HTML, URL, title, and screenshot data.
8. Observe worker/session/page status and bounded diagnostics.
9. See periodic and error screenshots with configured retention.
10. Use the Go-served dashboard and embedded noVNC client.
11. Produce activity from API, browser, and actual VNC input and expire when idle.
12. Restart scrape-api without losing worker-owned sessions.
13. Restart a worker and see its sessions removed.
14. Close a session and release browser, temporary profile, timers, and metadata.
15. Build arm64 local images and amd64 production artifacts.
16. Validate safe Proxmox image/lifecycle scripts without requiring live-host access.

The later operator-run Proxmox smoke test verifies image creation, DHCP discovery, central configuration injection, three-worker registration, noVNC proxying, service restart behavior, and resource tuning on the actual host.

## 19. Explicitly deferred work

- Authentication and multi-user authorization.
- PostgreSQL or another durable metadata database.
- Persistent cookies/login state between sessions.
- Historical session/event browsing after termination.
- Job queues, schedules, and scraping job definitions.
- Automatic CAPTCHA handling.
- Browser fingerprint spoofing.
- Proxy-management UI.
- Firefox and WebKit.
- Public event WebSockets and session recording.
- Kubernetes, autoscaling, and automatic Proxmox worker discovery.

These are extension points, not placeholders that should complicate the v1 implementation.
