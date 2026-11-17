#!/usr/bin/env bash
# Keep failures readable through both Actions logs and check-run annotations.
set -uo pipefail
check_log=$(mktemp)
trap 'rm -f "$check_log"' EXIT
"$@" 2>&1 | tee "$check_log"
status=${PIPESTATUS[0]}
if (( status != 0 )) && [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
  while IFS= read -r line; do
    line=${line//'%'/'%25'}
    line=${line//$'\r'/'%0D'}
    printf '::error::%s\n' "$line"
  done < <(tail -60 "$check_log")
fi
exit "$status"
