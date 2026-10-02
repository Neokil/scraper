#!/usr/bin/env bash

proxmox_dir="$(cd "$(dirname "$0")" && pwd)"

die() {
  printf "error: %s\n" "$*" >&2
  exit 1
}

require_root() {
  [[ "$(id -u)" == "0" ]] || die "run this command as root on the Proxmox host"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command is missing: $1"
}

load_config() {
  local defaults_file="$proxmox_dir/config.example.env"
  local config_file
  local explicit_config
  [[ -f "$defaults_file" ]] || die "default configuration not found: $defaults_file"
  explicit_config="$(printenv SCRAPER_CONFIG 2>/dev/null || true)"
  config_file="$proxmox_dir/config.env"
  if [[ -n "$explicit_config" ]]; then
    config_file="$explicit_config"
    [[ -f "$config_file" ]] || die "configuration not found: $config_file"
  fi
  set -a
  source "$defaults_file"
  if [[ -f "$config_file" && "$config_file" != "$defaults_file" ]]; then source "$config_file"; fi
  set +a
}

validate_ctid() {
  [[ "$1" =~ ^[1-9][0-9]{2,8}$ ]] || die "invalid CTID: $1"
}

ct_exists() {
  pct status "$1" >/dev/null 2>&1
}

require_free_ctid() {
  validate_ctid "$1"
  if ct_exists "$1"; then die "CTID already exists: $1"; fi
}

require_managed_ct() {
  validate_ctid "$1"
  ct_exists "$1" || die "CTID does not exist: $1"
  pct config "$1" | grep -Eq "^tags:.*scraper" || die "CTID $1 is not tagged as scraper-managed"
}

validate_host_prerequisites() {
  require_root
  [[ "$SCRAPER_VERSION" =~ ^[A-Za-z0-9._-]+$ ]] || die "SCRAPER_VERSION contains unsupported characters"
  for command_name in pct pveam pvesm awk sed sort tail grep curl; do
    require_command "$command_name"
  done
  pvesm status --storage "$STORAGE" >/dev/null 2>&1 || die "storage is unavailable: $STORAGE"
  storage_config="$(pvesm config "$STORAGE")"
  grep -Eq "content.*vztmpl" <<<"$storage_config" || die "storage $STORAGE must allow vztmpl content"
  grep -Eq "content.*rootdir" <<<"$storage_config" || die "storage $STORAGE must allow rootdir content"
  ip link show "$BRIDGE" >/dev/null 2>&1 || die "network bridge is unavailable: $BRIDGE"
}

latest_debian_template() {
  pveam available --section system \
    | awk '$2 ~ /^debian-[0-9]+-standard_/ {print $2}' \
    | sort -V \
    | tail -n 1
}

wait_for_ip() {
  local ctid="$1"
  local attempts
  local address
  attempts="$(printenv IP_DISCOVERY_ATTEMPTS 2>/dev/null || true)"
  if [[ -z "$attempts" ]]; then attempts=60; fi
  for _ in $(seq 1 "$attempts"); do
    address="$(pct exec "$ctid" -- ip -4 -o addr show dev eth0 2>/dev/null | awk '{split($4,a,"/"); print a[1]; exit}')"
    if [[ "$address" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      printf "%s\n" "$address"
      return 0
    fi
    sleep 1
  done
  die "timed out waiting for DHCP address for CTID $ctid"
}

wait_for_worker_registration() {
  local api_address="$1"
  local worker_id="$2"
  local attempts
  local response
  attempts="$(printenv REGISTRATION_ATTEMPTS 2>/dev/null || true)"
  if [[ -z "$attempts" ]]; then attempts=30; fi
  for _ in $(seq 1 "$attempts"); do
    response="$(curl -fsS "http://$api_address:$API_PORT/v1/workers/$worker_id" 2>/dev/null || true)"
    if grep -Fq '"status":"online"' <<<"$response"; then
      return 0
    fi
    sleep 1
  done
  die "timed out waiting for worker registration: $worker_id"
}

write_guest_env() {
  local ctid="$1"
  local destination="$2"
  local content="$3"
  local temp_file
  temp_file="$(mktemp)"
  chmod 0600 "$temp_file"
  printf "%s\n" "$content" >"$temp_file"
  pct exec "$ctid" -- install -d -m 0750 -o scraper -g scraper /etc/scraper
  pct push "$ctid" "$temp_file" "$destination" -perms 0640
  pct exec "$ctid" -- chown scraper:scraper "$destination"
  rm -f "$temp_file"
}

api_environment() {
  printf "SERVER_ADDRESS=:%s\nPROFILES_DIR=/var/lib/scraper/profiles\nWORKER_REQUEST_TIMEOUT=%s\nWORKER_TIMEOUT=%s\nDEFAULT_IDLE_TIMEOUT=%s\nDASHBOARD_POLL_INTERVAL=%s\nLOG_LEVEL=%s\n" \
    "$API_PORT" "$WORKER_REQUEST_TIMEOUT" "$WORKER_TIMEOUT" "$DEFAULT_IDLE_TIMEOUT" "$DASHBOARD_POLL_INTERVAL" "$LOG_LEVEL"
}

worker_environment() {
  local hostname="$1"
  local worker_address="$2"
  local api_address="$3"
  printf "DISPLAY=%s\nDISPLAY_RESOLUTION=%s\nDISPLAY_DEPTH=%s\nWORKER_SCRIPT=/opt/scraper/worker/dist/index.js\nWORKER_ID=%s\nWORKER_HOSTNAME=%s\nWORKER_VERSION=%s\nWORKER_BASE_URL=http://%s:%s\nWORKER_VNC_URL=http://%s:%s/websockify\nSCRAPE_API_URL=http://%s:%s\nWORKER_CAPACITY=%s\nWORKER_HEARTBEAT_INTERVAL=%s\nACTION_TIMEOUT=%s\nMAX_EVENTS=%s\nSCREENSHOT_INTERVAL=%s\nSCREENSHOT_RETENTION=%s\nERROR_SCREENSHOT_RETENTION=%s\nSCREENSHOT_MAX_BYTES=%s\nSCREENSHOT_ROOT=/var/lib/scraper/screenshots\nSESSION_ROOT=/var/lib/scraper/sessions\nPLAYWRIGHT_BROWSERS_PATH=/ms-playwright\n" \
    "$DISPLAY_NUMBER" "$DISPLAY_RESOLUTION" "$DISPLAY_DEPTH" \
    "$hostname" "$hostname" "$SCRAPER_VERSION" "$worker_address" "$WORKER_PORT" "$worker_address" "$VNC_WEBSOCKET_PORT" \
    "$api_address" "$API_PORT" "$WORKER_CAPACITY" "$HEARTBEAT_INTERVAL" "$ACTION_TIMEOUT" "$MAX_EVENTS" \
    "$SCREENSHOT_INTERVAL" "$SCREENSHOT_RETENTION" "$ERROR_SCREENSHOT_RETENTION" "$SCREENSHOT_MAX_BYTES"
}

project_ctids() {
  pct list | awk 'NR > 1 {print $1}' | while read -r candidate; do
    if pct config "$candidate" 2>/dev/null | grep -Eq "^tags:.*scraper"; then
      printf "%s\n" "$candidate"
    fi
  done
}
