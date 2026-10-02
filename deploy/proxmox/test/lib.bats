#!/usr/bin/env bats

setup() {
  source "$BATS_TEST_DIRNAME/../lib.sh"
}

@test "validate_ctid accepts a normal Proxmox ID" {
  run validate_ctid 210
  [ "$status" -eq 0 ]
}

@test "validate_ctid rejects broad and non-numeric targets" {
  run validate_ctid /
  [ "$status" -ne 0 ]
  run validate_ctid abc
  [ "$status" -ne 0 ]
  run validate_ctid 0
  [ "$status" -ne 0 ]
}

@test "latest_debian_template selects the highest available version" {
  pveam() {
    printf "system debian-12-standard_12.7-1_amd64.tar.zst\n"
    printf "system debian-13-standard_13.1-1_amd64.tar.zst\n"
    printf "system ubuntu-24.04-standard_24.04-2_amd64.tar.zst\n"
  }
  run latest_debian_template
  [ "$status" -eq 0 ]
  [ "$output" = "debian-13-standard_13.1-1_amd64.tar.zst" ]
}

@test "require_managed_ct rejects an untagged container" {
  pct() {
    case "$1" in
      status) return 0 ;;
      config) printf "hostname: unrelated\n" ;;
    esac
  }
  run require_managed_ct 210
  [ "$status" -ne 0 ]
}

@test "load_config applies defaults before explicit overrides" {
  printf "WORKER_COUNT=5\n" >"$BATS_TEST_TMPDIR/override.env"
  export SCRAPER_CONFIG="$BATS_TEST_TMPDIR/override.env"
  load_config
  [ "$STORAGE" = "Synology-Backup" ]
  [ "$BRIDGE" = "vmbr0" ]
  [ "$WORKER_COUNT" = "5" ]
}

@test "worker_environment includes discovered addresses and version" {
  : >"$BATS_TEST_TMPDIR/override.env"
  export SCRAPER_CONFIG="$BATS_TEST_TMPDIR/override.env"
  load_config
  run worker_environment scrape-01 192.0.2.11 192.0.2.10
  [ "$status" -eq 0 ]
  [[ "$output" == *"WORKER_VERSION=dev"* ]]
  [[ "$output" == *"WORKER_BASE_URL=http://192.0.2.11:8081"* ]]
  [[ "$output" == *"SCRAPE_API_URL=http://192.0.2.10:8080"* ]]
}
