#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
export PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
apt-get update
apt-get install -y --no-install-recommends ca-certificates tzdata nodejs npm xvfb openbox x11vnc websockify dbus-x11 procps

install -d /opt/scraper/src
tar -xzf /root/scraper-source.tgz -C /opt/scraper/src
cd /opt/scraper/src/worker
npm ci
npx playwright install --with-deps chromium
npm run build
npm prune --omit=dev

install -d /opt/scraper/worker
cp -a dist node_modules package.json /opt/scraper/worker/
install -m 0755 /opt/scraper/src/deploy/docker/worker-entrypoint.sh /usr/local/bin/worker-entrypoint
id scraper >/dev/null 2>&1 || useradd --system --home /var/lib/scraper --create-home scraper
install -d -m 0750 -o scraper -g scraper /etc/scraper /var/lib/scraper/screenshots /var/lib/scraper/sessions
chown -R scraper:scraper /opt/scraper/worker /ms-playwright
install -m 0644 /opt/scraper/src/deploy/systemd/scraper-worker.service /etc/systemd/system/scraper-worker.service
printf "%s\n" "$SCRAPER_VERSION" >/opt/scraper/VERSION
test -f /opt/scraper/worker/dist/index.js
command -v Xvfb x11vnc websockify >/dev/null
systemctl daemon-reload
systemctl disable scraper-worker.service >/dev/null 2>&1 || true

rm -rf /opt/scraper/src /root/scraper-source.tgz /root/provision.sh
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/*
truncate -s 0 /etc/machine-id
