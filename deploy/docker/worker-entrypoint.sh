#!/usr/bin/env bash
set -euo pipefail

display="$(printenv DISPLAY 2>/dev/null || true)"
if [[ -z "$display" ]]; then display=":99"; fi
resolution="$(printenv DISPLAY_RESOLUTION 2>/dev/null || true)"
if [[ -z "$resolution" ]]; then resolution="1920x1080"; fi
depth="$(printenv DISPLAY_DEPTH 2>/dev/null || true)"
if [[ -z "$depth" ]]; then depth="24"; fi

children=()
cleanup() {
  trap - TERM INT EXIT
  if [[ "${#children[@]}" -gt 0 ]]; then
    kill "${children[@]}" 2>/dev/null || true
    wait "${children[@]}" 2>/dev/null || true
  fi
}
trap cleanup TERM INT EXIT

Xvfb "$display" -screen 0 "$resolution"x"$depth" -ac +extension RANDR -nolisten tcp &
children+=("$!")

display_number="$(printf "%s" "$display" | tr -d ':')"
for _ in $(seq 1 50); do
  [[ -S "/tmp/.X11-unix/X$display_number" ]] && break
  sleep 0.1
done
if [[ ! -S "/tmp/.X11-unix/X$display_number" ]]; then
  echo "Xvfb did not become ready" >&2
  exit 1
fi

DISPLAY="$display" openbox &
children+=("$!")
x11vnc -display "$display" -forever -shared -nopw -noxdamage -rfbport 5900 -quiet &
children+=("$!")
websockify 6080 127.0.0.1:5900 &
children+=("$!")

worker_script="$(printenv WORKER_SCRIPT 2>/dev/null || true)"
if [[ -z "$worker_script" ]]; then worker_script="/app/dist/index.js"; fi
node "$worker_script" &
worker_pid="$!"
children+=("$worker_pid")
wait "$worker_pid"
