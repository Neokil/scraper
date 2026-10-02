#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
validate_host_prerequisites

role=""
if [[ "$#" -gt 0 ]]; then role="$1"; fi
case "$role" in
  api)
    template_ctid="$API_TEMPLATE_CTID"
    template_name="scrape-api-template"
    provision_script="$script_dir/provision-api.sh"
    cores="$API_CORES"
    memory="$API_MEMORY_MB"
    swap="$API_SWAP_MB"
    disk="$API_DISK_GB"
    ;;
  worker)
    template_ctid="$WORKER_TEMPLATE_CTID"
    template_name="scrape-worker-template"
    provision_script="$script_dir/provision-worker.sh"
    cores="$WORKER_CORES"
    memory="$WORKER_MEMORY_MB"
    swap="$WORKER_SWAP_MB"
    disk="$WORKER_DISK_GB"
    ;;
  *) die "usage: build-template.sh api|worker" ;;
esac

require_free_ctid "$template_ctid"
pveam update
debian_template="$(latest_debian_template)"
[[ -n "$debian_template" ]] || die "no Debian standard LXC template is available"
if ! pveam list "$STORAGE" | awk '{print $1}' | grep -Eq "/$debian_template$"; then
  pveam download "$STORAGE" "$debian_template"
fi
template_volume="$STORAGE:vztmpl/$debian_template"

repo_root="$(cd "$script_dir/../.." && pwd)"
temp_dir="$(mktemp -d)"
archive="$temp_dir/scraper-source.tgz"
cleanup() {
  if [[ "$temp_dir" == /tmp/* || "$temp_dir" == /var/tmp/* ]]; then rm -rf "$temp_dir"; fi
}
trap cleanup EXIT

tar -C "$repo_root" \
  --exclude=.git \
  --exclude=node_modules \
  --exclude=scrape-api/node_modules \
  --exclude=worker/node_modules \
  --exclude=worker/dist \
  --exclude=scrape-api/web/static/vendor \
  --exclude=data \
  --exclude=screenshots \
  -czf "$archive" .

pct create "$template_ctid" "$template_volume" \
  --hostname "$template_name" \
  --unprivileged 1 \
  --features nesting=1,keyctl=1 \
  --cores "$cores" \
  --memory "$memory" \
  --swap "$swap" \
  --rootfs "$STORAGE:$disk" \
  --net0 "name=eth0,bridge=$BRIDGE,ip=dhcp,type=veth" \
  --onboot 0 \
  --tags "scraper;template;role-$role"

pct start "$template_ctid"
wait_for_ip "$template_ctid" >/dev/null
pct push "$template_ctid" "$archive" /root/scraper-source.tgz -perms 0600
pct push "$template_ctid" "$provision_script" /root/provision.sh -perms 0700
pct exec "$template_ctid" -- env SCRAPER_VERSION="$SCRAPER_VERSION" bash /root/provision.sh
pct stop "$template_ctid"
pct set "$template_ctid" --description "scraper $role template; version=$SCRAPER_VERSION; base=$debian_template"
pct template "$template_ctid"

printf "Built %s template at CTID %s from %s\n" "$role" "$template_ctid" "$debian_template"
