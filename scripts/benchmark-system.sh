#!/usr/bin/env bash
# Run only as the command passed to test-system.sh.
set -euo pipefail
[[ "${WS_E2E_CLUSTER:-}" == websocket-e2e-* ]] || { echo 'System fixture required' >&2; exit 1; }
output=$(realpath -m "${BENCHMARK_REPORTS:-reports/system-benchmarks}")
mkdir -p "$output"
{
  git rev-parse HEAD
  go version
  uname -a
  kubectl version -o yaml
  kubectl get nodes -o wide
} > "$output/environment.txt"
kubectl port-forward deployment/main-ws-server --address=127.0.0.1 0:3001 > "$output/direct-port.log" 2>&1 &
forward_pid=$!
trap 'kill "$forward_pid" 2>/dev/null || true; wait "$forward_pid" 2>/dev/null || true' EXIT
deadline=$((SECONDS + 30))
while (( SECONDS < deadline )); do
  port=$(awk '/Forwarding from 127.0.0.1:.* -> 3001/ {split($3,a,":");print a[2];exit}' "$output/direct-port.log")
  if [[ -n "$port" ]]; then break; fi
  kill -0 "$forward_pid"
  sleep 0.2
done
[[ -n "$port" ]] || { cat "$output/direct-port.log"; exit 1; }
export WS_E2E_DIRECT_URL="ws://127.0.0.1:$port"
go test -tags=e2e -run '^$' -bench BenchmarkSystemRoundTrip -benchmem \
  -benchtime=1s -count=3 -timeout=5m ./integration_tests/system | tee "$output/round-trip.txt"
