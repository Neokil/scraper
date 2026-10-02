#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "$0")" && pwd)"
source "$script_dir/lib.sh"
load_config
[[ "$#" -ge 2 ]] || die "usage: create-worker.sh CTID HOSTNAME"
exec "$script_dir/create.sh" worker "$1" "$2"
