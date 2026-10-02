FROM node:22-bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
WORKDIR /app

COPY worker/package.json worker/package-lock.json ./
RUN npm ci \
    && npx playwright install --with-deps chromium \
    && apt-get update \
    && apt-get install -y --no-install-recommends xvfb openbox x11vnc websockify dbus-x11 procps \
    && rm -rf /var/lib/apt/lists/*

COPY worker/tsconfig.json ./
COPY worker/src ./src
RUN npm run build && npm prune --omit=dev

COPY deploy/docker/worker-entrypoint.sh /usr/local/bin/worker-entrypoint
RUN chmod 0755 /usr/local/bin/worker-entrypoint \
    && install -d -m 0770 /var/lib/scraper/screenshots /var/lib/scraper/sessions

ENV DISPLAY=:99
ENV DISPLAY_RESOLUTION=1920x1080
ENV DISPLAY_DEPTH=24
ENV WORKER_ADDRESS=0.0.0.0
ENV WORKER_PORT=8081
ENV SCREENSHOT_ROOT=/var/lib/scraper/screenshots
ENV SESSION_ROOT=/var/lib/scraper/sessions
EXPOSE 8081 6080
HEALTHCHECK --interval=5s --timeout=3s --retries=12 CMD ["node", "-e", "fetch('http://127.0.0.1:8081/healthz').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"]
ENTRYPOINT ["worker-entrypoint"]
