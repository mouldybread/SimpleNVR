# CI browser-network isolation and diagnostics

Prepared against 90eac990a7fb584fb2e15786110a6a18d37bca0e.

The reported ERR_NETWORK_CHANGED recurred in CI during a root-document redirect
and later during the login POST. Local runtime checks succeeded. The underlying
cause is NOT established by those observations or by this diagnostic batch.

## Mandatory gates

Core Go build/race/vet, dependency integrity, isolated storage failures, active
runtime restart, and SIGTERM smoke checks run in `core`. Browser acceptance runs
in two fresh `browser` matrix jobs on separate runners, without preceding core
Docker/container activity on those hosts. Both attempts are required; this is
not retry-until-green. Each builds the exact checkout and executes all existing
HTTPS/playback/export assertions. The existing `verify` job ID becomes an
always-evaluated aggregate requiring core and browser results to equal success.
Failure, cancellation, or skipping must not produce aggregate success. Repository
branch-protection settings are not modified or claimed inspected by this patch.

APT index failures are fatal, with bounded step/network timeouts. Per-job limits
are not increased. Core and browser results are independently visible. Browser
artifacts include commit, workflow-attempt and matrix-attempt identifiers.

## Login and diagnostics

The real login button is clicked once. Response and requestfailed listeners are
registered first; a failed POST terminates promptly with its network error code.
No observed response means server-side login outcome is unknown, not proof the
POST was never processed. There is no credential resubmission. HTTP 303/root,
authenticated root, application readiness, and all media assertions stay intact.
The existing one-GET redirect recovery remains limited to a successful POST and
an observed root-document ERR_NETWORK_CHANGED; it is not extended to POST failure.

`browser-network-evidence.json` contains at most 512 timestamped metadata events
with an explicit dropped-event count. It records interface names/state, route
and resolver fingerprints, and allowlisted CDP request/response/failure metadata.
URLs are reduced to categories: no query strings, request/response bodies,
cookies, authentication headers, full CDP payloads, HAR, traces or raw NetLogs
are written by the collector. CDP attachment/sampler failures are explicit. Host
sampling is every 250 ms, not an event-complete kernel network monitor. A temporal
correlation does not prove a network change caused Chrome's failure. Standard
existing browser reports and synthetic runtime-container logs remain retained.

The pinned Playwright 1.56.0 and moving installed Chrome channel are unchanged
while environment isolation is investigated. The collector reports Playwright's
actual version alongside the existing Chrome version. Compatibility remains an
investigation target, not a verified supported pairing or demonstrated cause.

## Validation and stopping rule

Standard-library helper tests cover success, POST failure, unrelated/duplicate
events, HTTP refusal, timeout, cancellation, no resubmission, credential-field
exclusion, CDP setup/failure, bounded storage and evidence flushing on exceptions.
Those unit tests use isolated fake pages/CDP objects, never mocked runtime
acceptance responses. They do not establish browser protocol compatibility.

Next evidence is the exact-commit core result, both unmodified browser acceptance
runs, and their network metadata if a failure recurs. This patch does not claim
to fix the underlying Chrome network error, certify the whole audit, or authorize
a merge. No production Go, frontend, authentication, or recorder code changes.
