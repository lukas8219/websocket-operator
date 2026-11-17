#!/usr/bin/env bash
# Run every campaign even if an earlier regression still fails.
set -uo pipefail
cd "$(dirname "$0")/.."
export GOMAXPROCS="${GOMAXPROCS:-4}"
behavior_logs="${BEHAVIOR_LOG_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/websocket-behavior.XXXXXX")}"
mkdir -p "$behavior_logs"
behavior_failed=0
run_check() {
  local name="$1"
  shift
  if "$@" >"$behavior_logs/$name.log" 2>&1; then
    printf 'PASS %s\n' "$name"
    printf '%s\t0\n' "$name" >>"$behavior_logs/results.tsv"
  else
    local status=$?
    printf 'FAIL %s (exit %s; see %s/%s.log)\n' "$name" "$status" "$behavior_logs" "$name"
    printf '%s\t%s\n' "$name" "$status" >>"$behavior_logs/results.tsv"
    behavior_failed=1
  fi
}
: >"$behavior_logs/results.tsv"
run_check unit go test -count=1 -timeout=90s ./...
run_check race go test -race -count=1 -timeout=90s ./...
while read -r package target; do
  run_check "$target" go test "$package" -run '^$' -fuzz "^${target}$" -fuzztime="${FUZZTIME:-30s}" -parallel=2 -timeout=3m
done <<'TARGETS'
./internal/diff FuzzDifferenceModel
./internal/consistent_hashing FuzzJumpHashOrderIndependence
./internal/peer_discovery FuzzKubernetesPeerSnapshots
./internal/transports FuzzHTTPTransportEnvelope
./internal/transports FuzzHTTPTransportResponseStatus
./cmd/sidecar FuzzSidecarMessageBody
TARGETS
printf 'Results: %s\n' "$behavior_logs"
exit "$behavior_failed"
