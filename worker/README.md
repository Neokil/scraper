# worker

The internal TypeScript/Playwright browser worker. Clients never address this service directly; `scrape-api` schedules and proxies all operations.

This folder owns:

- the strict-TypeScript worker process;
- one headed Chromium process and clean temporary user-data directory per session;
- page actions, inspection, diagnostics, activity tracking, and screenshots;
- registration and heartbeat reconciliation with `scrape-api`;
- the worker container image.

Run its checks from this directory:

```bash
npm ci --ignore-scripts
npm run lint
npm test
```

The image includes Playwright's matching Chromium build, Xvfb, Openbox, x11vnc, and websockify. Its Dockerfile expects the repository root as its build context:

```bash
docker build -f worker/Dockerfile .
```

See the root [README](../README.md) for worker configuration, lifecycle semantics, and the Compose environment.
