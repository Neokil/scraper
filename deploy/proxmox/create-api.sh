#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
ctid="$API_CTID"
hostname="$API_HOSTNAME"
if [[ "$#" -gt 0 ]]; then ctid="$1"; fi
if [[ "$#" -gt 1 ]]; then hostname="$2"; fi
exec "$script_dir/create.sh" api "$ctid" "$hostname"
