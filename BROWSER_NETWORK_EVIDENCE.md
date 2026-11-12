# Browser network evidence: structural routes and notifications

Baseline: d1f8977ed022f6edfb82ea3a21af031ff41be26a. Diagnostics only;
this does not fix or establish the cause of ERR_NETWORK_CHANGED.

Raw route/resolver SHA256 fingerprints remain. Separate sorted route-structure
fingerprints exclude IPv4 RefCnt/Use and IPv6 reference/use counters. Route
addresses and attributes are not emitted. Malformed, unavailable or oversized
inputs have explicit status and no structural digest. Reads are capped at
262144 bytes, route tables at 2048 rows, and recorded links at 64.

A nonblocking rtnetlink subscription observes link, address and IPv4/IPv6 route
notifications before sampling starts. Only allowlisted numeric metadata is
recorded: kind/action, family, interface index, and route table/protocol/scope/type.
No packet payload, raw route, address, credential, header, cookie, query string
or trace is stored. Each receive is at most 65536 bytes and decodes at most
256 messages. The shared 512-event ring counts evictions. Truncation, malformed
messages, overruns, socket errors and unavailable monitoring are explicit.
Readers and sockets are removed/closed on exit; no monitor subprocess or thread
is introduced. Existing sampling still uses its original to_thread boundary.

Timestamps are user-space receipt times on the same monotonic clock as CDP,
not kernel event timestamps. Notifications can be updates, not topology changes.
A structural fingerprint is not a complete routing-policy or address snapshot.
Absence of notifications is not proof of no network change, especially when
monitoring is unavailable or loss/eviction is reported. No pre-subscription
history is captured. Socket errors stop the monitor rather than silently spin.

The existing acceptance script already catches failure-snapshot exceptions and
re-raises the original failure; it is unchanged. No failure-time page evaluation
retry, login POST retry, delay, browser flag/version change, acceptance assertion
change or CI gate weakening is introduced. Both browser attempts remain required.

The original 12 Python helper tests remain, with eight added route/monitor tests.
The exact existing unittest discovery command is executed locally against the
candidate. This is helper evidence, not a real browser, container, Go or CI run.
