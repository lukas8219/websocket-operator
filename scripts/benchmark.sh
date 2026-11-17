#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
output="${BENCHMARK_REPORTS:-reports/benchmarks}"
mkdir -p "$output"
{
  git rev-parse HEAD
  go version
  uname -a
  go env GOOS GOARCH
  printf 'GOMAXPROCS=%s\n' "${GOMAXPROCS:-runtime default}"
  if command -v lscpu >/dev/null; then lscpu; fi
} > "$output/environment.txt"
go test -run '^$' -bench BenchmarkRouting -benchmem -benchtime=1s -count=5 \
  ./integration_tests/benchmarks | tee "$output/routing.txt"
go test -run '^$' -bench 'BenchmarkRouting/hash/peers=64$' -benchtime=2s \
  -cpuprofile "$output/cpu.pprof" -memprofile "$output/heap.pprof" \
  -o "$output/benchmarks.test" ./integration_tests/benchmarks > "$output/profile.txt"
