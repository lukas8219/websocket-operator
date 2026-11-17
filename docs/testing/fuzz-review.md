# Behavioral fuzz review

Tested revision: `fb016fed66d663a7d6202ccfb416d776635f0e69` (PR #32), with Go 1.24.9 on Linux/amd64. This report records the initial run before the fixes. The framework PR contains reusable fixtures, four passing fuzz campaigns, the runner, and this historical report. Failing regression tests, three additional fuzz targets, and minimized inputs are introduced alongside their fixes so every PR can pass independently.

## Outcome

Six distinct defects were reproduced. The existing six tests (plus the rebalance subtest) passed a fresh race-enabled run. Four passing, 30-second fuzz campaigns reported **693,548 executions** in total. Three additional fuzz targets found failures; targeted race and liveness tests found three more distinct defects.

Execution counts below are Go's counters, including seed/mutation attempts; they are not a count of unique behaviors or an exhaustive proof of correctness. Inputs are bounded to keep the tests useful and reproducible.

| Passing fuzz target | Executions | Property checked |
| --- | ---: | --- |
| `FuzzDifferenceModel` | 608,081 | Membership and delta match an independent set model; replay is idempotent |
| `FuzzJumpHashOrderIndependence` | 66,207 | Selection belongs to the peer set, is independent of input order, and rejects empty membership |
| `FuzzKubernetesPeerSnapshots` | 7,804 | Random snapshot transitions preserve the exact host set and emit correct additions/removals, including zero hosts |
| `FuzzHTTPTransportEnvelope` | 11,456 | Arbitrary payload bytes and opcodes reach the HTTP endpoint unchanged with the expected recipient header |

The Kubernetes snapshot fuzzer exercises the discovery update implementation without a cluster. The sidecar fuzzer starts the real sidecar entry point in a subprocess, a real WebSocket echo server, and an HTTP fixture for Kubernetes list/watch. It first verifies a successful echo before submitting each fuzz input. This establishes application request behavior, not a new full-cluster integration result.

PR #32's existing GitHub Actions Kubernetes integration run (`43525778059`) and CI run (`43525778094`) were already successful when inspected. They were not rerun by this testing task.

## Confirmed defects

### 1. Resolver stalls without a version-notification subscriber

Location: `internal/resolver/resolver.go:67`; the sidecar starts this resolver without reading `VersionUpgradeChannel`.

After processing the first peer update, `Init` blocks sending to the unbuffered version channel. A second discovery update never invalidates the cached hosts. The regression changes the only peer from `10.0.0.1:3000` to `10.0.0.2:3000`; the resolver continues returning the first address.

Impact: the default sidecar can retain stale routing state as pods change. A notification consumer should not be able to stop discovery processing; notification delivery needs an explicit nonblocking/coalescing or subscription design.

```sh
go test ./internal/resolver -run '^TestBehaviorResolverUpdatesWithoutVersionSubscriber$' -count=1 -timeout=10s
```

### 2. Failed HTTP delivery is reported as success

Location: `internal/transports/http.go:49`–`56`.

`HttpTransport.Write` checks network errors but never checks `resp.StatusCode`. The fuzzer found HTTP 359 returning a nil error. Explicit regression cases reproduced the same behavior for **404** (recipient not found) and **503** (unavailable upstream).

Impact: callers cannot detect messages that were rejected or never delivered. Return an error for unsuccessful response statuses and decide explicitly how redirects should be handled.

```sh
go test ./internal/transports -run '^FuzzHTTPTransportResponseStatus/f5dfef5e73fc6513$' -count=1
go test ./internal/transports -run '^TestBehaviorHTTPTransportRejectsFailedDelivery$' -count=1
```

### 3. Empty sidecar message body panics

Location: `cmd/sidecar/main.go:104`.

With a registered `ws-user-id`, an empty `POST /message` indexes `body[0]`. The minimized fuzz case is `[]byte("")`. The HTTP server logs an index-out-of-range panic and the client receives EOF instead of an HTTP error response.

The standard HTTP server recovers this request panic; this reproduction does **not** establish that the whole sidecar process exits. Validate the framing byte before indexing the body and return an explicit client error for malformed input.

```sh
go test ./cmd/sidecar -run '^FuzzSidecarMessageBody/caf81e9797b19c76$' -count=1 -timeout=20s
```

The first fuzz attempt stopped during fixture setup because of a transient listener bind conflict while other HTTP campaigns were active. It was not counted as an application defect. After those campaigns finished, the run found the empty-body panic and a separate corpus replay reproduced it.

### 4. IPv6 peer socket addresses are invalid

Location: `internal/peer_discovery/peer_discovery.go:37`.

Concatenating hostname and port with `%s:%d` omits brackets around IPv6 addresses. The saved fuzz case constructs `3030:3030:3030:3030:3030:3030:3030:3030:2955`; `net.SplitHostPort` rejects it with “too many colons in address.” Use `net.JoinHostPort` to support both address families.

```sh
go test ./internal/peer_discovery -run '^FuzzPeerSocketAddress/d339ea90e6de492f$' -count=1
```

### 5. In-memory peer discovery races during concurrent reads and updates

Locations: `internal/peer_discovery/in-memory.go:26`, `38`, and `47`.

Concurrent `CurrentHosts` and `AddPeer`/`RemovePeer` operations access the same unsynchronized hashicorp set. The race detector reports conflicting map reads and writes. Synchronize access and return independent snapshots.

This defect concerns the in-memory backend; it does not contradict the passing race tests for the separately locked Kubernetes backend.

```sh
go test -race ./internal/peer_discovery -run '^TestBehaviorInMemoryConcurrentSnapshots$' -count=1 -timeout=20s
```

### 6. Rendezvous transactions deadlock

Location: `internal/consistent_hashing/rendezvous.go:50`.

`Transaction` locks `r.mu`, then invokes `Add`/`Remove`, which lock the same non-reentrant mutex. Adding the first peer never completes. Use internal helpers that assume the transaction already owns the lock.

The current applications select jump hashing. This defect affects the alternative rendezvous implementation, not the successfully fuzzed jump-hash path.

```sh
go test ./internal/consistent_hashing -run '^TestBehaviorRendezvousTransactionCompletes$' -count=1 -timeout=5s
```

## Follow-up validation: sidecar connection-map race

After the six fixes above, a longer sidecar campaign failed during session setup.
A race-enabled corpus replay confirmed unsynchronized access to the sidecar's
connection map during registration, message lookup, and cleanup. This is a seventh
independent defect. The payload values minimized after the process became
unavailable were not reliable reproductions of the concurrency failure.

The fixture now propagates race reports and fatal runtime errors from its child
process, and a concurrent-session regression exercises registration, delivery, and
cleanup. Previously, a successful parent `go test -race` exit did not establish
that its sidecar subprocess was free of races. The initial counts above remain a
historical record; complete-stack validation must use the corrected fixture.

```sh
go test -race ./cmd/sidecar -run '^TestBehaviorSidecarConcurrentSessions$' -count=1 -timeout=30s
```

## Repeating the campaigns

Run the unit suite, race detector, and the fuzz targets available at the current revision with (four in the framework PR, seven after the fix stack):

```sh
FUZZTIME=30s bash scripts/test-behavior.sh
```

The runner continues after failures, writes a log for every command plus `results.tsv`,
and exits nonzero if any check fails. Set `BEHAVIOR_LOG_DIR` to choose the output directory.
Each PR must pass the runner. Regression assertions and saved inputs are preserved
with their corresponding fixes; they are not skipped or treated as expected failures.

From the repository root with Go 1.24.9 on PATH:

```sh
go test ./internal/diff -run '^$' -fuzz '^FuzzDifferenceModel$' -fuzztime=30s -parallel=2
go test ./internal/consistent_hashing -run '^$' -fuzz '^FuzzJumpHashOrderIndependence$' -fuzztime=30s -parallel=2
go test ./internal/peer_discovery -run '^$' -fuzz '^FuzzKubernetesPeerSnapshots$' -fuzztime=30s -parallel=2
go test ./internal/transports -run '^$' -fuzz '^FuzzHTTPTransportEnvelope$' -fuzztime=30s -parallel=2
```

The three originally failing fuzz targets retain their minimized inputs in the corresponding `testdata/fuzz` directories once their fix PRs are included. Each regression command above is available from its associated fix onward. The complete stack retains every assertion. The subprocess entry-point helper skips when invoked outside its fixture.

Local raw logs are retained in `/workspace/fuzz-review/logs`. The prepared cloud shell can activate the test toolchain with `source /workspace/pr28-review/env.sh`.

## Independently passing review order

1. [#34](https://github.com/lukas8219/websocket-operator/pull/34): reusable fixtures, four passing fuzz campaigns, runner, and this report.
2. [#35](https://github.com/lukas8219/websocket-operator/pull/35): resolver notifications and stale-host/coalescing regressions.
3. [#36](https://github.com/lukas8219/websocket-operator/pull/36): HTTP delivery errors, status fuzzing, saved input, and redirect regressions.
4. [#41](https://github.com/lukas8219/websocket-operator/pull/41): sidecar connection synchronization, concurrent-session regressions, and package-based image build.
5. [#37](https://github.com/lukas8219/websocket-operator/pull/37): empty sidecar body validation, message-body fuzzing, and saved input.
6. [#38](https://github.com/lukas8219/websocket-operator/pull/38): IPv6 socket formatting, address fuzzing, and saved input.
7. [#39](https://github.com/lukas8219/websocket-operator/pull/39): in-memory discovery synchronization and concurrent snapshot regressions.
8. [#40](https://github.com/lukas8219/websocket-operator/pull/40): rendezvous transaction locking and deadlock/concurrency regressions.

The sidecar connection fix precedes the empty-body fix so real-process message
regressions can run under the race detector. No production fixes are bundled into
the framework PR. The final test set is the same as the original complete stack.
