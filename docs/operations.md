# Operations guide

This service is intentionally state-light. Operational recovery should preserve profile definitions and accept that active browser sessions can disappear when a worker is replaced.

## Service checks

| Check | Expected result |
| --- | --- |
| `GET /healthz` | `200` when the central HTTP process is alive |
| `GET /readyz` | `200` plus the current online-worker count |
| `GET /v1/workers` | Worker status, heartbeat age, CPU, memory, target capacity, and active sessions |
| `GET /v1/sessions` | Current reconstructed session inventory |
| Dashboard `/` | Same fleet/session state for operators |

`readyz` reports state but does not fail solely because zero workers are online. Clients should treat a `503 no_available_worker` response from session creation as the authoritative capacity/availability signal.

For Compose:

```bash
docker compose ps
docker compose logs --tail=200 scrape-api worker
```

For LXC guests:

```bash
pct exec <api-ctid> -- systemctl status scrape-api
pct exec <worker-ctid> -- systemctl status scraper-worker
pct exec <api-ctid> -- journalctl -u scrape-api -n 200 --no-pager
pct exec <worker-ctid> -- journalctl -u scraper-worker -n 200 --no-pager
```

## Routine lifecycle

Drain a worker before planned maintenance. Existing sessions continue; the least-loaded scheduler excludes the worker from new placements.

```bash
curl -fsS -X POST http://scrape-api:8080/v1/workers/scrape-01/drain
```

Wait until `active_sessions` reaches zero, perform maintenance, then resume it:

```bash
curl -fsS -X POST http://scrape-api:8080/v1/workers/scrape-01/resume
```

An offline worker cannot be resumed until it has sent a recent heartbeat. A worker with a new incarnation automatically invalidates the old sessions that central state associated with its worker ID.

### Restart effects

| Action | Effect |
| --- | --- |
| Restart `scrape-api` | Central memory is empty briefly; workers register their complete state and active sessions return |
| Restart a worker service/LXC | That worker's Chromium processes and sessions are lost; users must reauthenticate in new sessions |
| Delete a session | Browser context, temporary profile, pages, timers, and central metadata are removed |
| Restart worker with its snapshot filesystem intact | Retained manual/error sidecars are scanned and republished; active sessions are still lost |
| Delete a worker LXC/volume | Its sessions and snapshots are unrecoverable |

## Snapshots and diagnostics

Create a retained manual snapshot:

```bash
snapshot_id="$({
  curl -fsS -X POST "http://scrape-api:8080/v1/pages/$page_id/snapshots" \
    -H 'content-type: application/json' \
    -d '{"format":"jpeg","quality":70,"full_page":false}'
} | jq -r .id)"

curl -fsS "http://scrape-api:8080/v1/snapshots/$snapshot_id" --output snapshot.jpg
curl -fsS -X DELETE "http://scrape-api:8080/v1/snapshots/$snapshot_id"
```

Periodic and error images are removed by age; the worker also evicts the oldest periodic/error images when `SCREENSHOT_MAX_BYTES` is exceeded. Manual images are not time-expired or size-evicted, so clients that create them are responsible for deletion. A new manual snapshot is rejected if it would leave storage above the ceiling. Metadata sidecars and image files live together under `SCREENSHOT_ROOT`.

The event buffer is in memory and bounded by `MAX_EVENTS` per active session. It includes console, page, request, navigation, action/timeout, and browser lifecycle failures. It is not an audit log and disappears with the worker/session.

## Backup policy

Back up only data whose loss matters:

- Central `PROFILES_DIR` contains operator-created JSON templates.
- Worker `SCREENSHOT_ROOT` contains retained evidence if those images are operationally important.
- Do not back up `SESSION_ROOT`; it contains disposable live Chromium profiles and is removed with sessions.
- There is no database or durable session/event history to back up.

Restore profiles before starting `scrape-api`. Restored snapshot image/JSON sidecar pairs are discovered when their worker starts; moving snapshots to a different worker is not a supported routing workflow.

## Troubleshooting

### A worker does not register

1. Confirm `SCRAPE_API_URL` in `/etc/scraper/worker.env` points to the current API DHCP address.
2. Check DNS/routing or use `pct exec <worker> -- curl -v http://<api-ip>:8080/healthz` after installing a suitable diagnostic client.
3. Inspect the worker journal for registration rejection or timeout messages.
4. On Proxmox, run `deploy/proxmox/reconcile.sh`; it rediscovers API/worker IPs, rewrites every worker environment, and restarts worker services.
5. Confirm the API's `WORKER_TIMEOUT` is comfortably longer than `WORKER_HEARTBEAT_INTERVAL`.

### Chromium does not launch

1. Check `systemctl status scraper-worker` and its journal.
2. Confirm Xvfb owns the configured display and that `DISPLAY`, `DISPLAY_RESOLUTION`, and `DISPLAY_DEPTH` are valid.
3. Confirm `/ms-playwright` exists and is readable by the `scraper` user.
4. Confirm the unprivileged LXC retains the template's `nesting=1,keyctl=1` features and has adequate memory/shared memory.
5. Ensure `SESSION_ROOT` is writable and contains enough free space.

### noVNC does not connect

1. Fetch `/v1/sessions/<id>/vnc`; a 404 means the session is no longer active.
2. Confirm `x11vnc` and `websockify` are running inside the assigned worker.
3. Check that `WORKER_VNC_URL` ends in `/websockify` and is reachable from `scrape-api`.
4. Preserve WebSocket upgrade headers in any reverse proxy placed in front of `scrape-api`.
5. Do not expose port 6080 directly; the supported client path is the central WebSocket proxy.

### Sessions never become idle

- Verify dashboard image/event URLs contain `monitoring=1`; monitoring requests must not update activity.
- Look for pages generating continuing network traffic. Playwright-observed requests intentionally count as activity.
- Check clients for repeated page/session operations or noVNC input notifications.

### Screenshots are missing

- Check free space and permissions under `SCREENSHOT_ROOT`.
- Inspect worker logs for capture errors or storage cleanup.
- Confirm `SCREENSHOT_INTERVAL`, retention values, and `SCREENSHOT_MAX_BYTES` are large enough for the workload.
- A snapshot route disappears when its metadata expires, is explicitly deleted, or the owning worker is removed.

### DHCP or Proxmox discovery fails

- Confirm `vmbr0` reaches a functioning DHCP server.
- Run `pct exec <ctid> -- ip -4 -o addr show dev eth0`.
- Increase `IP_DISCOVERY_ATTEMPTS` from its default of 60 if DHCP is slow.
- Use `deploy/proxmox/list.sh` to show discovered addresses for all scraper-tagged containers.

## Upgrades

Build new LXC templates from the desired repository revision rather than modifying an existing template in place. Use unused template CTIDs, drain/replace workers one at a time, verify registration, and remove old containers only after validation. Replacing a worker always discards its live sessions.

For local development, rebuild with `docker compose up -d --build --wait`. The root and worker lockfiles pin JavaScript tooling, Playwright, and its matching Chromium revision; `npm run generate` refreshes vendored noVNC and self-contained ReDoc files deterministically.

## Security boundary

- No authentication or user isolation exists in v1.
- `evaluate` is intentional arbitrary page JavaScript execution.
- Worker API, x11vnc, and websockify ports must be reachable only by `scrape-api` and administrators.
- All windows share one worker display; noVNC is a debugging view, not a private per-session desktop.
- Proxy credentials are stored as plaintext JSON and sent to the assigned worker.
- Browsed content is untrusted. Apply network egress controls separately if workers must not reach sensitive LAN services.
