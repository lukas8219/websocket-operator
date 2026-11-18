#!/usr/bin/env bash
# Keep failures readable through both Actions logs and check-run annotations.
set -uo pipefail
check_log=$(mktemp)
trap 'rm -f "$check_log"' EXIT
"$@" 2>&1 | tee "$check_log"
status=${PIPESTATUS[0]}
if (( status != 0 )) && [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
  # Cleanup logs must not hide the earlier Go failure. Use one multiline
  # annotation because Actions limits the number of annotations per step.
  details=$(awk '/\.go:[0-9]+|--- FAIL:|^FAIL|^Error:|^error:|^panic:|fatal error:|lastTermination/ {print}' "$check_log" | head -40)
  details+=$'\n'
  details+=$(tail -30 "$check_log")
  details=${details//'%'/'%25'}
  details=${details//$'\r'/'%0D'}
  details=${details//$'\n'/'%0A'}
  printf '::error::%s\n' "$details"
fi
exit "$status"
