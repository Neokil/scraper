#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
validate_host_prerequisites

if ct_exists "$API_CTID"; then
  require_managed_ct "$API_CTID"
  if [[ "$(pct status "$API_CTID" | awk '{print $2}')" != "running" ]]; then pct start "$API_CTID"; fi
  api_address="$(wait_for_ip "$API_CTID")"
  printf "Using existing scrape-api CTID %s at %s\n" "$API_CTID" "$api_address"
else
  "$script_dir/create-api.sh" "$API_CTID" "$API_HOSTNAME"
fi

"$script_dir/reconcile.sh"

for index in $(seq 1 "$WORKER_COUNT"); do
  ctid=$((WORKER_CTID_START + index - 1))
  printf -v hostname "scrape-%02d" "$index"
  if ct_exists "$ctid"; then
    require_managed_ct "$ctid"
    printf "Worker CTID %s already exists; leaving it unchanged\n" "$ctid"
    continue
  fi
  "$script_dir/create-worker.sh" "$ctid" "$hostname"
done

"$script_dir/list.sh"
