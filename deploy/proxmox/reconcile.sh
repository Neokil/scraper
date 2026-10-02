#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
validate_host_prerequisites
require_managed_ct "$API_CTID"

if [[ "$(pct status "$API_CTID" | awk '{print $2}')" != "running" ]]; then pct start "$API_CTID"; fi
api_address="$(wait_for_ip "$API_CTID")"

while read -r ctid; do
  [[ "$ctid" == "$API_CTID" ]] && continue
  config="$(pct config "$ctid")"
  grep -Eq "^tags:.*role-worker" <<<"$config" || continue
  if [[ "$(pct status "$ctid" | awk '{print $2}')" != "running" ]]; then pct start "$ctid"; fi
  worker_address="$(wait_for_ip "$ctid")"
  hostname="$(awk '/^hostname:/ {print $2}' <<<"$config")"
  worker_env="$(worker_environment "$hostname" "$worker_address" "$api_address")"
  write_guest_env "$ctid" /etc/scraper/worker.env "$worker_env"
  pct exec "$ctid" -- systemctl restart scraper-worker.service
  wait_for_worker_registration "$api_address" "$hostname"
  printf "Reconciled worker CTID %s to scrape-api %s\n" "$ctid" "$api_address"
done < <(project_ctids)

printf "Dashboard: http://%s:%s\n" "$api_address" "$API_PORT"
