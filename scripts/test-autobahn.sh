#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
# Pin the test implementation, Python runtime and echo server together.
image=crossbario/autobahn-testsuite@sha256:519915fb568b04c9383f70a1c405ae3ff44ab9e35835b085239c258b6fac3074
reports=$(realpath -m "${AUTOBAHN_REPORTS:-reports/autobahn}")
mkdir -p "$reports"
# Reject reused reports so an interrupted run cannot pass on stale results.
[[ -z "$(ls -A "$reports")" ]] || { echo "Report directory must be empty: $reports" >&2; exit 1; }
fixture=$(mktemp -d)
container="websocket-autobahn-$$"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$fixture"
}
trap cleanup EXIT
CGO_ENABLED=0 go build -o "$fixture/proxy" ./integration_tests/autobahn
cp integration_tests/autobahn/{run.sh,fuzzingclient.json} "$fixture/"
timeout 15m docker run --name "$container" --network none \
  -v "$fixture:/fixture:ro" -v "$reports:/reports" "$image" sh /fixture/run.sh
python3 integration_tests/autobahn/check.py "$reports"
