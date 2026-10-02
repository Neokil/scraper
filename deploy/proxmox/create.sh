#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
validate_host_prerequisites

[[ "$#" -ge 3 ]] || die "usage: create.sh api|worker CTID HOSTNAME"
role="$1"
ctid="$2"
hostname="$3"
require_free_ctid "$ctid"

case "$role" in
  api)
    template_ctid="$API_TEMPLATE_CTID"
    cores="$API_CORES"
    memory="$API_MEMORY_MB"
    swap="$API_SWAP_MB"
    ;;
  worker)
    template_ctid="$WORKER_TEMPLATE_CTID"
    cores="$WORKER_CORES"
    memory="$WORKER_MEMORY_MB"
    swap="$WORKER_SWAP_MB"
    ;;
  *) die "role must be api or worker" ;;
esac

ct_exists "$template_ctid" || die "template CTID does not exist: $template_ctid"
pct config "$template_ctid" | grep -Eq "^template: 1" || die "CTID $template_ctid is not a template"

pct clone "$template_ctid" "$ctid" \
  --hostname "$hostname" \
  --full 1 \
  --storage "$STORAGE"
pct set "$ctid" \
  --cores "$cores" \
  --memory "$memory" \
  --swap "$swap" \
  --net0 "name=eth0,bridge=$BRIDGE,ip=dhcp,type=veth" \
  --onboot 1 \
  --tags "scraper;role-$role"
pct start "$ctid"
address="$(wait_for_ip "$ctid")"

if [[ "$role" == "api" ]]; then
  api_env="$(api_environment)"
  write_guest_env "$ctid" /etc/scraper/api.env "$api_env"
  pct exec "$ctid" -- systemctl enable --now scrape-api.service
  printf "Created scrape-api CTID %s at http://%s:%s\n" "$ctid" "$address" "$API_PORT"
else
  ct_exists "$API_CTID" || die "scrape-api CTID does not exist: $API_CTID"
  api_address="$(wait_for_ip "$API_CTID")"
  worker_env="$(worker_environment "$hostname" "$address" "$api_address")"
  write_guest_env "$ctid" /etc/scraper/worker.env "$worker_env"
  pct exec "$ctid" -- systemctl enable --now scraper-worker.service
  wait_for_worker_registration "$api_address" "$hostname"
  printf "Created worker %s CTID %s at %s\n" "$hostname" "$ctid" "$address"
fi
