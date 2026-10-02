#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends ca-certificates tzdata golang-go npm make

install -d /opt/scraper/src
tar -xzf /root/scraper-source.tgz -C /opt/scraper/src
cd /opt/scraper/src/scrape-api
npm ci --ignore-scripts
npm run generate
go build -trimpath -ldflags="-s -w -X main.version=$SCRAPER_VERSION" -o /usr/local/bin/scrape-api ./cmd/scrape-api

id scraper >/dev/null 2>&1 || useradd --system --home /var/lib/scraper --create-home scraper
install -d -m 0750 -o scraper -g scraper /etc/scraper /var/lib/scraper/profiles
cp /opt/scraper/src/scrape-api/profiles/generic.json /var/lib/scraper/profiles/generic.json
chown scraper:scraper /var/lib/scraper/profiles/generic.json
install -m 0644 /opt/scraper/src/deploy/systemd/scrape-api.service /etc/systemd/system/scrape-api.service
printf "%s\n" "$SCRAPER_VERSION" >/opt/scraper/VERSION
scrape-api version >/dev/null
systemctl daemon-reload
systemctl disable scrape-api.service >/dev/null 2>&1 || true

rm -rf /opt/scraper/src /root/scraper-source.tgz /root/provision.sh
apt-get clean
rm -rf /var/lib/apt/lists/* /tmp/*
truncate -s 0 /etc/machine-id
