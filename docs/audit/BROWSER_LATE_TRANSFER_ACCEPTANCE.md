# Grouped native late-transfer failure and explicit recovery acceptance

Base: e3f7bf788628f20aac8b301b83e05f1f6d0e00fe, green by user report.
Actions artifacts for that head were not independently reviewed. The previous
button and observer failures remain historical; this batch does not erase them.
Its purpose is another original-audit browser gate, not another reporting-only
increment or production comparator activation.

## One coherent five-file group

Add a bounded test-only TLS media server plus native failure/recovery checks,
nine tooling regression methods, mandatory orchestrator wiring, an explicit
browser-job OpenSSL executable dependency, and this scope/evidence record.
The unchanged shell test_browser_playback*.py discovery includes the new tests.
The unchanged baseline, rendered keyboard/control contract and four existing
browser cases all remain prerequisites, in both isolated Chrome jobs.
Core/race/vet/storage/lifecycle and correspondence jobs/aggregate gates remain.
Only one workflow package-install line changes; no job or timeout is relaxed.

## Native sequence and required witnesses

1. Authenticate/read the existing real backend MP4 fixture. Require Content-Type,
   a bounded Content-Length/body and MP4 header. Hold only immutable bytes in
   memory; record their SHA256. The relay never receives cookies/passwords or
   chooses an upstream URL. Bind only to an ephemeral 127.0.0.1 TLS port.
2. Establish an ordinary application playback session through a real timeline
   seek. Pause through the real button, then direct the native video src to the
   relay's complete-body control. Resume through the button and require trusted
   native metadata/playing events, advancing clock and decoded-frame witness.
   This catches relay/TLS/source compatibility failures before fault acceptance.
3. Pause and select the relay fault source on that same native application video.
   Require native metadata, playing, advancing time and decoded frames BEFORE
   permitting the fault connection to close. The response advertises the full
   source length/range but supplies only an identical source prefix, at most
   96 KiB and leaving at least 16 KiB undelivered. Only a witnessed-frame gate
   permits an accepted cut; timeout/cleanup cuts cannot count as success.
4. Require a trusted native error with observed MediaError code 2 or 3, stopped
   shared intent, native pause, invalidated session, cleared pending metadata,
   hidden affected video and visible unavailable message. Record the observed
   code; do not infer that every decode error has a network cause in production.
   Silent continuation, source substitution or a native unsupported-source-only
   failure cannot satisfy this contract.
5. Require no automatic advancement/restart. Pressing Play without selecting a
   new recording must remain stopped and request a new time. Explicit timeline
   selection then returns to the genuine private backend URL, establishes a newer
   generation and new trusted metadata observation, and decodes native frames.

The test source assignments are deliberate failure injection into an established
real application session. Timeline/API responses are NOT rewritten, media events
are not dispatched, and HTMLMediaElement.play is not overridden. The relay creates
its own HTTPS responses on its own URL; it does NOT transparently replace a backend
response or qualify the backend's own partial-write/error behavior. The fault
source refuses subsequent GET retries with empty 503 responses so replay cannot
hide the deliberate short body. Positive control bytes remain byte-identical.
A fresh normal backend seek is the only accepted recovery.

## Bounds, cleanup and evidence

Use a one-day throwaway localhost certificate/key generated inside a private
test-owned temporary directory. CI explicitly installs OpenSSL; no application
certificate, credential, key or media file is modified. Existing test-context
self-signed-certificate handling applies; production TLS policy is unchanged.
Source cap: 4 MiB. Recorded request ceiling: 32. Sockets/handshakes: 5-second
operations. Prefix gate: 12 seconds; native waits retain existing 15-second bounds.
The full orchestrator retains the existing 300-second driver envelope and shell
limits; tooling tests retain their existing 20-second subprocess envelope.

Keep per-request path/status/range/advertised-length/bytes-written observations,
source hash, post-decode cut request/cut timestamps, native states/events/frames,
retry result and listener/handler join accounting in transport_contract inside
the existing playback-edge-report.json. Browser decoding proves the prefix was
usable, not that every written byte reached the browser or disk. Query strings,
cookies and Authorization values are not recorded. Partial failures remain in
artifacts; any running transport contract is marked failed on exception. Shut
and join the listener plus non-daemon handler threads before claiming completion.
Release the prefix waiter on cleanup, including failed browser setup. Socket
writes/errors remain diagnostic; no hidden response-success fallback is used.

Nine new regression methods cover supported/refused byte ranges, real loopback
TLS complete/range/HEAD responses, actual incomplete-body exception after prefix
cut, refused fault retries, cancellation of a blocked prefix waiter, fixed-route
and secret-recording boundaries, request ceiling and media/cut preconditions.
These tooling tests do not decode synthetic arbitrary bytes and are not counted
as native media acceptance. CI executes them and the real Chrome media sequence.

## Scope and audit reconciliation

No production source, image, playback policy, media flags, dependencies in go.mod,
recording behavior, storage/manifest behavior, retention/cleanup authority, main,
merge or deployment change. Resource-cost acceptance, runtime qualification and
positive ingestion-time coverage remain false. Existing authenticated HTTPS
slow-reader/backpressure/disconnect/shutdown Go tests remain separate evidence;
they are not recounted as new work here. Backend runtime-image backpressure and
shutdown acceptance, intentionally corrupt full-media fixtures, wider browser/OS
save-dialog behavior, remaining timeline/DST cases, live Dahua/RTSP/restart,
deployment filesystem/blocked I/O and final whole-branch review remain scoped
residuals. A green candidate supplies this tested late-transfer scenario only;
it does not certify all failures or complete the original audit automatically.
