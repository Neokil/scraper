#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
require_root
require_command pct

printf "%-8s %-22s %-10s %-16s %-8s %-6s %-8s %-8s %-16s\n" \
  "CTID" "HOSTNAME" "STATUS" "IP" "ROLE" "CPU" "RAM" "DISK" "VERSION"
while read -r ctid; do
  config="$(pct config "$ctid")"
  hostname="$(awk '/^hostname:/ {print $2}' <<<"$config")"
  tags="$(awk '/^tags:/ {$1=""; sub(/^ /,""); print}' <<<"$config")"
  status="$(pct status "$ctid" | awk '{print $2}')"
  cores="$(awk '/^cores:/ {print $2}' <<<"$config")"
  memory="$(awk '/^memory:/ {print $2}' <<<"$config")"
  disk="$(sed -n 's/^rootfs:.*size=\([^,]*\).*$/\1/p' <<<"$config")"
  version="$(sed -n 's/^description:.*version=\([^;]*\).*/\1/p' <<<"$config")"
  address="-"
  if [[ "$status" == "running" ]]; then
    address="$(pct exec "$ctid" -- ip -4 -o addr show dev eth0 2>/dev/null | awk '{split($4,a,"/"); print a[1]; exit}')"
    if [[ -z "$address" ]]; then address="pending"; fi
    guest_version="$(pct exec "$ctid" -- sh -c 'test -f /opt/scraper/VERSION && sed -n 1p /opt/scraper/VERSION' 2>/dev/null || true)"
    if [[ -n "$guest_version" ]]; then version="$guest_version"; fi
  fi
  role="$(tr ';' '\n' <<<"$tags" | awk -F- '/^role-/ {print $2; exit}')"
  if [[ -n "$memory" ]]; then memory="${memory}M"; else memory="-"; fi
  if [[ -z "$cores" ]]; then cores="-"; fi
  if [[ -z "$disk" ]]; then disk="-"; fi
  if [[ -z "$version" ]]; then version="unknown"; fi
  printf "%-8s %-22s %-10s %-16s %-8s %-6s %-8s %-8s %-16s\n" \
    "$ctid" "$hostname" "$status" "$address" "$role" "$cores" "$memory" "$disk" "$version"
done < <(project_ctids)
