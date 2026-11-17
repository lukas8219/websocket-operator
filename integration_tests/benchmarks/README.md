# Repeatable benchmarks

Run `bash scripts/benchmark.sh` with the repository's Go version. It records five
samples of jump-hash and resolver lookup with 8, 64 and 256 peers, using 1,024 fixed
identities. Peers stay stable; setup and warm-up are outside the timed loop. These
are serial routing microbenchmarks, not network throughput or scale-change tests.

Go's output reports `ns/op` (time per lookup), `B/op` (allocated bytes per lookup)
and `allocs/op` (allocation count). Lower values generally mean less work, but
compare the same workload, Go version, CPU and `GOMAXPROCS`. Use `benchstat` on
before/after files; a single shared-runner sample is not a performance guarantee.

The output directory includes revision/toolchain/CPU metadata, raw samples and
CPU/heap profiles. Inspect profiles with `go tool pprof reports/benchmarks/cpu.pprof`
or `go tool pprof -alloc_space reports/benchmarks/heap.pprof`. CI publishes these
artifacts without noisy timing thresholds. Command or correctness failures still
fail CI. The resolver benchmark includes its current cache behavior; it does not
assume cache hits bypass hashing.

The script also records three samples of registry traversal, remove/store pairs,
and initial population at 1,000, 10,000 and 100,000 connections in `registry.txt`.
It compares the production mutex-protected map and pointer snapshot with a
test-only `sync.Map` alternative. Connection objects are created outside timing;
each traversal checks that it visits every connection. These serial workloads
isolate allocation costs; they do not measure lock contention or network latency.

A snapshot copies pointers (8 bytes each on amd64), not connection objects or
message buffers, once per rebalance event. It releases the map lock before slow
socket operations. `sync.Map.Range` avoids the slice allocation but has different
update costs and does not provide a consistent membership snapshot during writes.
Compare all three workloads before changing the registry. `B/op` measures total
allocation per operation, not retained heap size or bytes copied.
