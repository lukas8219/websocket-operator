# Autobahn protocol checks

Run `bash scripts/test-autobahn.sh` from the repository root with Go, Docker,
Python 3 and GNU `timeout`. Reports go to `reports/autobahn`; use a new empty
`AUTOBAHN_REPORTS` directory for subsequent runs. Open `index.html` to inspect
individual frames and close behavior. CI uploads the full report even on failure.

The pinned stock Autobahn image runs its `echoserver` and `fuzzingclient`, with the
production Go load balancer between them. Discovery uses one in-memory peer.
`User-Agent` is the configured identity header because stock fuzzingclient sends
it already. No payload adapter or fork changes the cases. Everything runs inside
one network-isolated container.

The profile covers groups 1–7: framing, ping/pong, reserved bits/opcodes,
fragmentation, UTF-8 and closing. Both message and closing results must be `OK`
or `INFORMATIONAL`, with one explicit timing allowance: cases 3.2–3.4,
4.1.3–4.1.5, 4.2.3–4.2.5, 5.15 and 7.1.6 may report `NON-STRICT` when a later
invalid/close frame closes the proxy before the upstream echo of earlier valid
data arrives. These outcomes remain visible in the logs/report; their closing
checks must still pass. UTF-8 failures and other non-strict outcomes fail.
Every selected case must appear, and missing reports fail.
Performance groups, extensions/compression, TLS, Kubernetes discovery and live
rebalancing are outside this protocol profile. The sidecar requires JSON routing
messages and is covered by the separate system and Go fuzz tests.

Autobahn tests protocol conformance; Go fuzzers generate application inputs and
check properties. Neither replaces the other, and a passing profile is not a
claim that every optional WebSocket feature is implemented.
