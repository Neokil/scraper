# Browser Scraping Infrastructure

## 1. Overview

Build a self-hosted browser automation infrastructure running on a Proxmox server.

The system provides a central HTTP API that other applications can use to create and control persistent browser sessions without knowing where or how the browser is running.

The initial version focuses exclusively on **interactive browser sessions**. A job/queue-based scraping API is intentionally out of scope for the first version and may be added later.

### Goals

* Run multiple isolated Chromium browser instances.
* Use lightweight LXC containers as browser workers.
* Provide a normal graphical browser environment using:

  * Xvfb
  * Openbox
  * Chromium
  * Playwright
* Allow humans to remotely inspect/control browsers through noVNC.
* Provide screenshots and browser status for monitoring/debugging.
* Provide a central REST API.
* Automatically assign sessions to available workers.
* Support persistent browser profiles/cookies.
* Keep the public API independent of the underlying worker implementation.
* Make it possible to replace LXC workers with VMs or another backend later.

### Non-goals for the initial version

Do **not** implement:

* Scraping jobs / job queues.
* Scheduled scraping.
* Automatic CAPTCHA solving.
* Proxy management UI.
* Browser fingerprint spoofing.
* Kubernetes/container orchestration.
* Automatic scaling.

These can be added later without fundamentally changing the API.

---

# 2. High-Level Architecture

```text
                         ┌──────────────────────────┐
                         │       scrape-api         │
                         │                          │
                         │ REST API                  │
                         │ Session manager           │
                         │ Worker manager            │
                         │ Profile manager           │
                         │ Monitoring API            │
                         └────────────┬─────────────┘
                                      │
                         Worker protocol / HTTP
                                      │
             ┌────────────────────────┼────────────────────────┐
             │                        │                        │
             ▼                        ▼                        ▼
      ┌──────────────┐        ┌──────────────┐        ┌──────────────┐
      │ scrape-01    │        │ scrape-02    │        │ scrape-03    │
      │ LXC          │        │ LXC          │        │ LXC          │
      │              │        │              │        │              │
      │ Chromium     │        │ Chromium     │        │ Chromium     │
      │ Playwright   │        │ Playwright   │        │ Playwright   │
      │ Xvfb         │        │ Xvfb         │        │ Xvfb         │
      │ Openbox      │        │ Openbox      │        │ Openbox      │
      │ noVNC        │        │ noVNC        │        │ noVNC        │
      └──────────────┘        └──────────────┘        └──────────────┘
```

The API server must not expose the worker implementation directly.

Clients communicate only with `scrape-api`.

---

# 3. Browser Worker

Each worker is an LXC container containing a complete graphical browser environment.

## Components

```text
LXC
├── Xvfb
│   └── DISPLAY=:99
├── Openbox
├── Chromium
├── Playwright
├── x11vnc
└── noVNC
```

### Xvfb

Creates a virtual X display without requiring a physical GPU/display.

Default:

```text
DISPLAY=:99
Resolution: 1920x1080
```

### Openbox

Provides basic window management.

A full desktop environment such as XFCE is not required.

### Chromium

Chromium runs in non-headless mode against Xvfb.

This is intentional: the browser must be visible through noVNC and behave like a normal graphical browser.

### Playwright

Playwright is used to control Chromium.

The worker should expose a worker API to the central API service.

### noVNC

Provides browser-based remote access to the virtual display.

Example:

```text
https://scraper.example/vnc/{session-id}
```

A user should be able to open this URL and see/interact with the browser.

---

# 4. Worker Lifecycle

Workers should register themselves with the API server.

Example:

```http
POST /internal/workers/register
```

```json
{
  "worker_id": "scrape-01",
  "hostname": "scrape-01",
  "version": "1.0.0",
  "capacity": 5
}
```

The API server maintains worker state.

Possible states:

```text
online
offline
draining
```

A worker should periodically send heartbeats.

```http
POST /internal/workers/{workerId}/heartbeat
```

The heartbeat should include basic resource/session information.

Example:

```json
{
  "cpu_percent": 31.4,
  "memory_bytes": 1874329600,
  "active_sessions": 2,
  "capacity": 5
}
```

If heartbeats stop for a configurable period, the worker should be considered offline.

---

# 5. Session Model

A **session** represents one persistent browser context.

Example:

```text
Session
└── Browser
    ├── Page / Tab 1
    ├── Page / Tab 2
    └── Page / Tab 3
```

A session is assigned to exactly one worker.

The client should not need to know which worker is being used.

Example:

```json
{
  "id": "sess_8f31c2",
  "worker": "scrape-03",
  "status": "running"
}
```

---

# 6. Browser Profiles

Sessions may optionally use a named persistent browser profile.

Example:

```text
profiles/
├── amazon-de
├── ebay-de
├── github
└── generic
```

A profile should be able to contain:

* Cookies
* Local storage
* Browser preferences
* Session state
* Locale
* Timezone
* Optional proxy configuration

The exact storage mechanism is implementation-dependent.

A profile must not be shared simultaneously by multiple browser contexts unless explicitly supported.

For example:

```text
amazon-de
    ↓
session-123
```

is valid.

But:

```text
amazon-de
├── session-123
└── session-456
```

should either be prohibited or use isolated copies of the profile.

---

# 7. Public REST API

Base URL:

```text
/v1
```

## Sessions

### Create session

```http
POST /v1/sessions
```

Request:

```json
{
  "browser": "chromium",
  "profile": "amazon-de",
  "headless": false
}
```

Response:

```json
{
  "id": "sess_8f31c2",
  "worker": "scrape-03",
  "status": "running",
  "browser": "chromium",
  "vnc_url": "/v1/sessions/sess_8f31c2/vnc"
}
```

The worker should be selected automatically.

Worker selection should initially use a simple least-loaded strategy.

---

### List sessions

```http
GET /v1/sessions
```

Optional filters:

```text
?status=running
?worker=scrape-03
```

---

### Get session

```http
GET /v1/sessions/{sessionId}
```

Response:

```json
{
  "id": "sess_8f31c2",
  "worker": "scrape-03",
  "status": "running",
  "created_at": "2026-09-30T07:30:00Z",
  "last_activity": "2026-09-30T07:32:12Z",
  "pages": 3
}
```

---

### Delete session

```http
DELETE /v1/sessions/{sessionId}
```

This terminates the browser session and releases the worker resources.

---

# 8. Pages / Tabs

A page represents a browser tab.

### Create page

```http
POST /v1/sessions/{sessionId}/pages
```

Request:

```json
{
  "url": "https://example.com"
}
```

Response:

```json
{
  "id": "page_a72f91",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

If `url` is omitted, create a blank page.

---

### List pages

```http
GET /v1/sessions/{sessionId}/pages
```

---

### Get page

```http
GET /v1/pages/{pageId}
```

---

### Close page

```http
DELETE /v1/pages/{pageId}
```

---

# 9. Navigation

```http
POST /v1/pages/{pageId}/navigate
```

Request:

```json
{
  "url": "https://example.com/products",
  "wait_until": "domcontentloaded"
}
```

Supported `wait_until` values should map to Playwright:

```text
load
domcontentloaded
networkidle
commit
```

The API should enforce a configurable maximum timeout.

---

# 10. Page Interaction

Initial interaction endpoints:

```text
POST /v1/pages/{pageId}/click
POST /v1/pages/{pageId}/type
POST /v1/pages/{pageId}/press
POST /v1/pages/{pageId}/select
POST /v1/pages/{pageId}/wait
```

### Click

```json
{
  "selector": "button#accept",
  "timeout": 10000
}
```

### Type

```json
{
  "selector": "#search",
  "text": "3D printer"
}
```

### Press

```json
{
  "selector": "#search",
  "key": "Enter"
}
```

### Wait

```json
{
  "selector": ".results",
  "timeout": 10000
}
```

The API should use Playwright's normal selector/action semantics rather than attempting to implement its own browser automation engine.

---

# 11. JavaScript Evaluation

Expose a controlled endpoint for executing JavaScript inside the page.

```http
POST /v1/pages/{pageId}/evaluate
```

Request:

```json
{
  "expression": "document.querySelector('.price')?.textContent"
}
```

Response:

```json
{
  "result": "€129.99"
}
```

The implementation should use Playwright's evaluation mechanism.

The API must clearly document that arbitrary JavaScript execution is possible and therefore this endpoint must not be exposed publicly without authentication.

---

# 12. Page Data

Useful inspection endpoints:

```http
GET /v1/pages/{pageId}/html
GET /v1/pages/{pageId}/screenshot
GET /v1/pages/{pageId}/url
GET /v1/pages/{pageId}/title
```

### Screenshot

Support:

```text
GET /v1/pages/{pageId}/screenshot
```

Optional parameters:

```text
?full_page=true
?format=jpeg
?quality=60
```

---

# 13. Monitoring

The infrastructure should continuously capture browser state.

Each active page should have a screenshot taken approximately every 10 seconds.

The screenshot interval must be configurable.

Example:

```text
SCREENSHOT_INTERVAL=10s
```

Screenshots should be stored temporarily.

Recommended retention:

```text
Normal screenshots:
    last 5–10 minutes

Error screenshots:
    retained longer

Manual snapshots:
    retained until explicitly deleted
```

Do not retain unlimited screenshots.

---

# 14. Monitoring Dashboard

Provide a small web UI served by the API server.

The dashboard should show:

```text
Workers
├── scrape-01    ● online   2/5 sessions
├── scrape-02    ● online   4/5 sessions
└── scrape-03    ● online   0/5 sessions

Sessions
├── sess_123
│   ├── worker: scrape-01
│   ├── pages: 3
│   └── status: running
│
└── sess_456
    ├── worker: scrape-02
    ├── pages: 1
    └── status: running
```

For every page display the latest screenshot.

Example:

```text
┌─────────────────────┐
│ scrape-01           │
│ session sess_123    │
│                     │
│  [latest screenshot]│
│                     │
│ ● running           │
└─────────────────────┘
```

Clicking a session should provide a detailed view.

The detail page should include:

* All browser tabs
* Current URLs
* Page titles
* Latest screenshots
* Session status
* Worker
* CPU/memory information
* Recent logs
* Link to noVNC
* Ability to terminate the session

---

# 15. Live Browser Access

Every session should have a noVNC endpoint.

Example:

```text
/v1/sessions/{sessionId}/vnc
```

The dashboard should provide an **Open Browser** button.

The user should be able to manually interact with the actual browser.

This is important for debugging:

```text
Automation running
        ↓
Something unexpected happens
        ↓
Open noVNC
        ↓
See exact browser state
        ↓
Manually investigate/interact
```

---

# 16. Error Diagnostics

The system should capture more than screenshots.

For each page/session collect:

* Current URL
* Page title
* Last activity timestamp
* Browser console errors
* Playwright errors
* Failed network requests
* Navigation errors
* Screenshot on errors

Example error record:

```json
{
  "timestamp": "2026-09-30T07:45:12Z",
  "session": "sess_123",
  "page": "page_456",
  "type": "page_error",
  "message": "Timeout waiting for selector .results",
  "url": "https://example.com/products",
  "screenshot": "/screenshots/error_123.jpg"
}
```

---

# 17. Worker ↔ API Communication

Keep the internal worker protocol separate from the public API.

The public API might receive:

```text
POST /v1/pages/page_123/click
```

The API server then sends an internal command to the worker.

Conceptually:

```text
Client
  │
  │ REST
  ▼
scrape-api
  │
  │ internal worker protocol
  ▼
scrape-01
  │
  ▼
Playwright
  │
  ▼
Chromium
```

The internal protocol can initially be HTTP/JSON.

Do not expose worker ports directly to external clients.

---

# 18. Persistence

Use PostgreSQL for persistent metadata.

Suggested entities:

```text
workers
sessions
pages
profiles
events
```

Possible schema:

```text
workers
-------
id
hostname
version
status
capacity
last_heartbeat
created_at

sessions
--------
id
worker_id
profile_id
status
created_at
last_activity

pages
-----
id
session_id
url
title
status
created_at
last_activity

profiles
--------
id
name
created_at
updated_at

events
------
id
session_id
page_id
type
message
created_at
```

Screenshots should **not** be stored in PostgreSQL.

Store them on filesystem/object storage and retain only their metadata in the database.

---

# 19. Authentication

The API must support authentication from the beginning.

For the initial implementation, a simple API token is sufficient.

Example:

```http
Authorization: Bearer <token>
```

All public endpoints should require authentication except potentially health/readiness endpoints.

Authentication should be implemented in a way that allows a more sophisticated mechanism to be added later.

---

# 20. Networking

Recommended layout:

```text
LAN
 │
 ├── scrape-api
 │       :8080
 │
 └── workers
         scrape-01
         scrape-02
         scrape-03
```

Workers should preferably be reachable only from the API server and administration network.

Do not expose Chromium, Playwright, Xvfb, or worker APIs directly to the LAN/internet.

Only the API/dashboard should be externally accessible.

---

# 21. Configuration

Configuration should be environment-variable based.

Example:

```text
SERVER_ADDRESS=:8080

DATABASE_URL=postgres://...

AUTH_TOKEN=...

WORKER_HEARTBEAT_INTERVAL=10s
WORKER_TIMEOUT=30s

SCREENSHOT_INTERVAL=10s
SCREENSHOT_RETENTION=10m

DEFAULT_BROWSER=chromium
DEFAULT_DISPLAY=:99
DEFAULT_WIDTH=1920
DEFAULT_HEIGHT=1080
```

---

# 22. API Client

The API should be designed so client libraries can easily be generated later.

A Go client might eventually look like:

```go
client := scraper.NewClient("http://scrape-api:8080", token)

session, err := client.Sessions.Create(ctx, scraper.CreateSessionRequest{
    Profile: "amazon-de",
})

page, err := session.Pages.Create(ctx, scraper.CreatePageRequest{
    URL: "https://example.com",
})

err = page.Navigate(ctx, "https://example.com/products")

price, err := page.Evaluate(ctx,
    `document.querySelector(".price")?.textContent`,
)
```

The REST API is the source of truth; client libraries are convenience wrappers.

---

# 23. Future Extensions

The architecture should leave room for:

### Scraping jobs

```text
POST /v1/jobs
GET  /v1/jobs/{id}
DELETE /v1/jobs/{id}
```

A job could acquire a session, execute a predefined scraper, and return structured data.

### Worker auto-discovery

Automatically discover/register new Proxmox workers.

### Proxy management

```text
profiles
    ↓
proxy configuration
    ↓
browser session
```

### Browser types

Potentially:

```text
chromium
firefox
webkit
```

The initial implementation only needs Chromium.

### Session recording

Potentially record browser sessions for later debugging.

### WebSocket events

Eventually expose:

```text
/v1/sessions/{id}/events
```

for real-time dashboard updates.

---

# 24. Technology Recommendation

Suggested initial stack:

```text
API server:        Go
HTTP framework:    standard net/http or chi
Database:          PostgreSQL
Database access:   sqlc
Browser automation:Playwright
Browser:           Chromium
Virtual display:   Xvfb
Window manager:    Openbox
Remote desktop:    x11vnc + noVNC
Worker OS:         Debian-based LXC
Deployment:        Proxmox
```

Avoid introducing Redis or another queue system until it is actually needed.

The initial architecture does not require a queue.

---

# 25. Initial Implementation Phases

## Phase 1 — Worker

Create a Debian-based LXC image containing:

* Xvfb
* Openbox
* Chromium
* Playwright
* x11vnc
* noVNC

Verify:

1. Chromium starts.
2. Chromium uses Xvfb.
3. noVNC displays the browser.
4. Playwright can control Chromium.
5. Multiple tabs work simultaneously.

---

## Phase 2 — Worker API

Implement:

* Worker registration
* Heartbeats
* Session creation
* Session destruction
* Page creation
* Page destruction
* Navigation
* Click
* Type
* Press
* Wait
* Evaluate
* Screenshot

---

## Phase 3 — Central API

Implement:

* Worker management
* Worker selection
* Session management
* Page management
* Profile management
* Authentication
* API error handling

The central API should hide worker details from clients.

---

## Phase 4 — Monitoring

Implement:

* Worker status
* Session status
* Page status
* 10-second screenshots
* Screenshot retention
* Browser console/error collection
* Dashboard
* noVNC links

---

## Phase 5 — Persistence

Implement PostgreSQL persistence for:

* Workers
* Sessions
* Pages
* Profiles
* Events

---

# 26. API Design Principles

1. **Clients should never need to know which worker hosts a session.**
2. **A session belongs to exactly one worker.**
3. **A session can contain multiple pages/tabs.**
4. **Browser state should remain persistent for the lifetime of the session.**
5. **Named profiles provide persistent login/browser state.**
6. **The public API must not expose Playwright directly.**
7. **The worker implementation must remain replaceable.**
8. **Screenshots are primarily a debugging/monitoring mechanism, not permanent storage.**
9. **The system should work with one worker and scale to many workers without API changes.**
10. **Do not implement the future scraping-job system prematurely.**

# 27. Definition of Done

The first version is complete when another application can:

```text
1. Authenticate with scrape-api
2. Create a browser session
3. Receive an automatically selected worker
4. Create multiple tabs
5. Navigate to websites
6. Click/type/interact with pages
7. Execute JavaScript
8. Read page information
9. Capture screenshots
10. Monitor the session through the web UI
11. Open the actual browser through noVNC
12. Close the session
```

The application using the API should **not need to know that the browser is running inside Proxmox, LXC, Xvfb, or Chromium**.
