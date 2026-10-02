#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
require_root
require_command pct

[[ "$#" -ge 1 ]] || die "usage: delete.sh CTID [--force]"
ctid="$1"
require_managed_ct "$ctid"
hostname="$(pct config "$ctid" | awk '/^hostname:/ {print $2}')"

confirmed="false"
if [[ "$#" -gt 1 && "$2" == "--force" ]]; then confirmed="true"; fi
if [[ "$confirmed" != "true" ]]; then
  printf "Permanently destroy scraper container %s (%s)? Type the CTID to confirm: " "$ctid" "$hostname"
  read -r answer
  [[ "$answer" == "$ctid" ]] || die "confirmation did not match; nothing deleted"
fi

if [[ "$(pct status "$ctid" | awk '{print $2}')" == "running" ]]; then
  pct shutdown "$ctid" --timeout 30 || pct stop "$ctid"
fi
pct destroy "$ctid" --purge 1
printf "Destroyed CTID %s (%s); its local sessions and snapshots cannot be recovered\n" "$ctid" "$hostname"
