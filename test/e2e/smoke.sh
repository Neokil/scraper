#!/usr/bin/env bash
set -euo pipefail

base_url="$(printenv SCRAPER_URL 2>/dev/null || true)"
if [[ -z "$base_url" ]]; then base_url="http://127.0.0.1:8080"; fi
temp_dir="$(mktemp -d)"
session_id=""

cleanup() {
  if [[ -n "$session_id" ]]; then
    curl -sS -X DELETE "$base_url/v1/sessions/$session_id" >/dev/null || true
  fi
  case "$temp_dir" in
    /tmp/*|/private/tmp/*|/var/folders/*|/private/var/*) rm -rf "$temp_dir" ;;
  esac
}
trap cleanup EXIT

json_post() {
  curl -fsS -X POST "$1" -H "content-type: application/json" -d "$2"
}

workers="$(curl -fsS "$base_url/v1/workers")"
[[ "$(jq '.workers | length' <<<"$workers")" -ge 1 ]]
[[ "$(jq -r '.workers[0].status' <<<"$workers")" == "online" ]]

profile_name="e2e-$(date +%s)"
profile_payload="$(jq -nc --arg name "$profile_name" '{
  name: $name,
  description: "End-to-end fixture profile",
  browser: "chromium",
  locale: "en-US",
  timezoneId: "Europe/Berlin",
  viewport: {width: 1280, height: 720}
}')"
json_post "$base_url/v1/profiles" "$profile_payload" >"$temp_dir/profile.json"
[[ "$(jq -r '.name' "$temp_dir/profile.json")" == "$profile_name" ]]

session_payload="$(jq -nc --arg profile "$profile_name" '{
  browser: "chromium",
  profile: $profile,
  idle_timeout_seconds: 600
}')"
json_post "$base_url/v1/sessions" "$session_payload" >"$temp_dir/session.json"
session_id="$(jq -r '.id' "$temp_dir/session.json")"
[[ "$session_id" == sess_* ]]
[[ "$(jq -r '.status' "$temp_dir/session.json")" == "running" ]]

json_post "$base_url/v1/sessions/$session_id/pages" '{"url":"http://fixture"}' >"$temp_dir/page.json"
page_id="$(jq -r '.id' "$temp_dir/page.json")"
[[ "$page_id" == page_* ]]

json_post "$base_url/v1/pages/$page_id/type" '{"selector":"#search","text":"headed browser"}' >/dev/null
json_post "$base_url/v1/pages/$page_id/click" '{"selector":"#submit"}' >/dev/null
json_post "$base_url/v1/pages/$page_id/wait" '{"selector":".result","state":"visible"}' >/dev/null
json_post "$base_url/v1/pages/$page_id/select" '{"selector":"#region","value":"us"}' >/dev/null
json_post "$base_url/v1/pages/$page_id/evaluate" '{"expression":"document.querySelector(\".result\").textContent"}' >"$temp_dir/evaluate.json"
[[ "$(jq -r '.result' "$temp_dir/evaluate.json")" == "headed browser" ]]

curl -fsS -X POST "$base_url/v1/sessions/$session_id/pages" >"$temp_dir/page-2.json"
page_2_id="$(jq -r '.id' "$temp_dir/page-2.json")"
json_post "$base_url/v1/pages/$page_2_id/navigate" \
  '{"url":"http://fixture","wait_until":"domcontentloaded"}' >/dev/null
json_post "$base_url/v1/pages/$page_2_id/type" '{"selector":"#search","text":"press test"}' >/dev/null
json_post "$base_url/v1/pages/$page_2_id/press" '{"selector":"#search","key":"Enter"}' >/dev/null
json_post "$base_url/v1/pages/$page_2_id/wait" '{"selector":".result","state":"visible"}' >/dev/null

json_post "$base_url/v1/sessions/$session_id/pages" '{"url":"http://fixture/result.html"}' >"$temp_dir/page-3.json"
page_3_id="$(jq -r '.id' "$temp_dir/page-3.json")"
curl -fsS "$base_url/v1/sessions/$session_id/pages" >"$temp_dir/pages.json"
[[ "$(jq '.pages | length' "$temp_dir/pages.json")" == "3" ]]
curl -fsS "$base_url/v1/pages/$page_3_id" >"$temp_dir/page-3-state.json"
[[ "$(jq -r '.title' "$temp_dir/page-3-state.json")" == "Fixture result" ]]
curl -fsS -X DELETE "$base_url/v1/pages/$page_3_id" >/dev/null
curl -fsS "$base_url/v1/sessions/$session_id/pages" >"$temp_dir/pages-after-delete.json"
[[ "$(jq '.pages | length' "$temp_dir/pages-after-delete.json")" == "2" ]]

json_post "$base_url/v1/pages/$page_id/evaluate" \
  '{"expression":"(() => { console.error(\"e2e-console\"); fetch(\"http://127.0.0.1:1/unreachable\").catch(() => null); return \"diagnostics\"; })()"}' \
  >"$temp_dir/diagnostics.json"
[[ "$(jq -r '.result' "$temp_dir/diagnostics.json")" == "diagnostics" ]]

navigation_status="$(curl -sS -o "$temp_dir/navigation-error.json" -w "%{http_code}" \
  -X POST "$base_url/v1/pages/$page_2_id/navigate" \
  -H "content-type: application/json" \
  -d '{"url":"http://127.0.0.1:1/unreachable","timeout":1000}')"
[[ "$navigation_status" == "502" ]]

curl -fsS "$base_url/v1/pages/$page_id/url" >"$temp_dir/url.json"
[[ "$(jq -r '.url' "$temp_dir/url.json")" == "http://fixture/" ]]
curl -fsS "$base_url/v1/pages/$page_id/title" >"$temp_dir/title.json"
[[ "$(jq -r '.title' "$temp_dir/title.json")" == "Scraper fixture" ]]
curl -fsS "$base_url/v1/pages/$page_id/html" >"$temp_dir/page.html"
rg -q "headed browser" "$temp_dir/page.html"

curl -fsS "$base_url/v1/pages/$page_id/screenshot?format=jpeg&quality=60" >"$temp_dir/page.jpg"
[[ "$(wc -c <"$temp_dir/page.jpg")" -gt 1000 ]]

json_post "$base_url/v1/pages/$page_id/snapshots" '{"format":"png","full_page":true}' >"$temp_dir/snapshot.json"
snapshot_id="$(jq -r '.id' "$temp_dir/snapshot.json")"
curl -fsS "$base_url/v1/snapshots/$snapshot_id" >"$temp_dir/snapshot.png"
[[ "$(wc -c <"$temp_dir/snapshot.png")" -gt 1000 ]]
curl -fsS -X DELETE "$base_url/v1/snapshots/$snapshot_id" >/dev/null

failure_status="$(curl -sS -o "$temp_dir/action-error.json" -w "%{http_code}" \
  -X POST "$base_url/v1/pages/$page_id/wait" \
  -H "content-type: application/json" \
  -d '{"selector":"#never-present","timeout":100}')"
[[ "$failure_status" == "504" ]]

error_snapshot=""
diagnostics_ready="false"
for _ in $(seq 1 10); do
  curl -fsS "$base_url/v1/sessions/$session_id/events" >"$temp_dir/events.json"
  error_snapshot="$(jq -r '[.events[] | select(.snapshot_id != null)][-1].snapshot_id // empty' "$temp_dir/events.json")"
  diagnostics_ready="$(jq '[.events[].type] | (index("console_error") != null and index("request_failed") != null and index("navigation_error") != null and index("playwright_error") != null)' "$temp_dir/events.json")"
  if [[ -n "$error_snapshot" && "$diagnostics_ready" == "true" ]]; then break; fi
  sleep 1
done
[[ -n "$error_snapshot" ]]
[[ "$diagnostics_ready" == "true" ]]

snapshot_status=""
for _ in $(seq 1 10); do
  snapshot_status="$(curl -sS -o "$temp_dir/error-snapshot.jpg" -w "%{http_code}" "$base_url/v1/snapshots/$error_snapshot")"
  if [[ "$snapshot_status" == "200" ]]; then break; fi
  sleep 1
done
[[ "$snapshot_status" == "200" ]]
[[ "$(wc -c <"$temp_dir/error-snapshot.jpg")" -gt 1000 ]]

curl -fsS "$base_url/sessions/$session_id" >"$temp_dir/dashboard.html"
rg -q "$session_id" "$temp_dir/dashboard.html"
curl -fsS "$base_url/v1/sessions/$session_id/vnc" >"$temp_dir/vnc.html"
rg -q "/static/js/vnc.js" "$temp_dir/vnc.html"
curl -fsS "$base_url/static/vendor/novnc/lib/rfb.js" >"$temp_dir/rfb.js"
[[ "$(wc -c <"$temp_dir/rfb.js")" -gt 1000 ]]
SCRAPER_WS_URL="ws://127.0.0.1:8080/v1/sessions/$session_id/vnc/websocket" node test/e2e/vnc-websocket.mjs

curl -fsS -X DELETE "$base_url/v1/sessions/$session_id" >/dev/null
session_id=""

json_post "$base_url/v1/sessions" '{"browser":"chromium","profile":"generic","idle_timeout_seconds":2}' >"$temp_dir/idle.json"
idle_id="$(jq -r '.id' "$temp_dir/idle.json")"
json_post "$base_url/v1/sessions/$idle_id/pages" '{}' >"$temp_dir/idle-page.json"
idle_page_id="$(jq -r '.id' "$temp_dir/idle-page.json")"
sleep 1
curl -fsS "$base_url/v1/sessions/$idle_id" >/dev/null
sleep 1
[[ "$(curl -sS -o /dev/null -w "%{http_code}" "$base_url/v1/sessions/$idle_id")" == "200" ]]
for _ in $(seq 1 4); do
  curl -sS -o /dev/null "$base_url/v1/pages/$idle_page_id/screenshot?monitoring=1" || true
  sleep 1
done
idle_status="$(curl -sS -o /dev/null -w "%{http_code}" "$base_url/v1/sessions/$idle_id")"
[[ "$idle_status" == "404" ]]

printf "End-to-end smoke test passed\n"
