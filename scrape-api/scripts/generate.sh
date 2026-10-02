#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_dir"

node scripts/vendor-novnc.mjs
mkdir -p web/static/docs

generate_doc() {
  local source_file="$1"
  local output_file="$2"
  local title="$3"
  local temporary_file="$output_file.tmp"
  ./node_modules/.bin/redocly build-docs "$source_file" \
    --output "$output_file" \
    --title "$title" \
    --disableGoogleFont
  {
    printf '<!-- Regenerate from %s with npm run generate; manual edits will be overwritten. -->\n' "$source_file"
    sed '1{/^<!-- Regenerate from /d;}' "$output_file"
  } >"$temporary_file"
  mv "$temporary_file" "$output_file"
}

generate_doc api/public.openapi.yaml web/static/docs/public.html "Browser Scraping API"
generate_doc api/worker.openapi.yaml web/static/docs/worker.html "Browser Worker Internal API"
