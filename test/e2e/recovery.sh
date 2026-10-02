#!/usr/bin/env bash
set -euo pipefail

base_url="$(printenv SCRAPER_URL 2>/dev/null || true)"
if [[ -z "$base_url" ]]; then base_url="http://127.0.0.1:8080"; fi
session_id=""

cleanup() {
  if [[ -n "$session_id" ]]; then
    curl -sS -X DELETE "$base_url/v1/sessions/$session_id" >/dev/null || true
  fi
}
trap cleanup EXIT

session="$(curl -fsS -X POST "$base_url/v1/sessions" \
  -H "content-type: application/json" \
  -d '{"browser":"chromium","profile":"generic","idle_timeout_seconds":600}')"
session_id="$(jq -r '.id' <<<"$session")"
curl -fsS -X POST "$base_url/v1/sessions/$session_id/pages" \
  -H "content-type: application/json" \
  -d '{"url":"http://fixture"}' >/dev/null

docker compose restart scrape-api >/dev/null
for _ in $(seq 1 30); do
  if curl -fsS "$base_url/healthz" >/dev/null 2>&1; then break; fi
  sleep 1
done

recovered="false"
for _ in $(seq 1 30); do
  status="$(curl -sS -o /dev/null -w "%{http_code}" "$base_url/v1/sessions/$session_id")"
  if [[ "$status" == "200" ]]; then
    recovered="true"
    break
  fi
  sleep 1
done
[[ "$recovered" == "true" ]]

docker compose restart worker >/dev/null
for _ in $(seq 1 30); do
  worker_status="$(docker compose ps --format json worker | jq -r '.Health')"
  if [[ "$worker_status" == "healthy" ]]; then break; fi
  sleep 1
done

removed="false"
for _ in $(seq 1 30); do
  status="$(curl -sS -o /dev/null -w "%{http_code}" "$base_url/v1/sessions/$session_id")"
  if [[ "$status" == "404" ]]; then
    removed="true"
    session_id=""
    break
  fi
  sleep 1
done
[[ "$removed" == "true" ]]

printf "Restart recovery test passed\n"
