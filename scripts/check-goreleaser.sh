#!/usr/bin/env bash
set -euo pipefail

goreleaser=${GORELEASER:-goreleaser}
check_output=$(mktemp)
check_plain=$(mktemp)
trap 'rm -f "$check_output" "$check_plain"' EXIT

if NO_COLOR=1 "$goreleaser" check >"$check_output" 2>&1; then
  cat "$check_output"
  exit 0
fi

cat "$check_output"
sed $'s/\033\[[0-9;]*m//g' "$check_output" >"$check_plain"

# GoReleaser 2.17 makes `check` fail for every deprecated block. Rome
# intentionally uses the still-supported `brews` publisher because Milestone 7
# requires a traditional formula rather than an unsigned cask. Accept only that
# one known deprecation; syntax errors, other deprecations, and invalid fields
# still fail this target. A snapshot release performs a second full load/build
# of the same configuration.
deprecated_count=$(grep -c 'DEPRECATED:' "$check_plain" || true)
if [[ "$deprecated_count" -eq 1 ]] &&
  grep -Eq 'DEPRECATED:[[:space:]]+brews[[:space:]]+should not be used anymore' "$check_plain" &&
  grep -Fq 'configuration is valid, but uses deprecated properties' "$check_plain"; then
  echo "accepted the documented GoReleaser brews deprecation for tap-only formula publishing"
  exit 0
fi

exit 1
