# Authenticated HTTPS export lifecycle regression batch

## Purpose and scope

This batch adds Go integration-test drafts for the existing session-authenticated /api/download handler, real FFmpeg/FFprobe synthetic media, actual loopback HTTPS transport, pinned export resources and shutdownGate. It changes no production export implementation or security/media policy. It is validation work, not an export feature or an audit clearance.

Both whole-clip stream-copy and boundary-trim paths are exercised. One 2-second, 256x144, 10-fps yuv420p MPEG-4 fixture is generated per case using temporal noise so the payload exceeds socket buffers while remaining below an 8 MiB test cap. Whole requests select 0-2 seconds; trims select 0.2-1.8 seconds. The existing restricted trim implementation is used unchanged.

The server uses httptest HTTPS with its test certificate trusted by the provided client transport. Login goes through the real auth middleware, obtaining a real Secure session cookie. Sessionless and event-bearer-only requests must fail before export pinning. The successful media handler must not receive credentials after authentication middleware processing. No deployment credentials, live camera or external service is used.

## Case matrix

1. Successful whole-clip export to a deliberately slow-reading HTTPS client.
2. Successful trimmed export to the same type of client.
3. Whole-clip stream-copy response with client cancellation/disconnect during sustained network backpressure.
4. Trimmed response with client cancellation/disconnect during sustained network backpressure.
5. Whole-clip stream-copy response with shutdown during sustained network backpressure.
6. Trimmed response with shutdown during sustained network backpressure.

Successful responses are fully received within the fixture cap, checked for the expected policy/type, decoded by real FFmpeg and duration-probed against the synthetic request with a two-frame tolerance. This is stronger than an ftyp check but is still fixture validation, not a general guarantee of footage completeness or calibrated timing.

## Genuine transport backpressure

The cancellation/shutdown cases configure small real TCP send/receive buffers on the test sockets, read one response byte, then stop reading. A transparent ResponseWriter observer counts outstanding calls to the real network writer; it does not inject a blocking Write or fabricate an I/O error. The test requires a sustained outstanding write while the export slot is held and the media handler has not returned. It fails if the response finishes before this condition is observed, rather than reporting unexercised slow-client cleanup as passing.

The successful slow-reader cases use normal client receive buffering, small reads and an application delay. They validate successful slow consumption; they do not claim to force the same kernel-backpressure state. HTTP/1.x is required by the harness. These are loopback TCP/TLS tests, not browser download tests or deployment-network tests.

## Cleanup and shutdown observations

The test input pool records actual pinned input descriptors. After the media handler returns, tests require released admission, closed pinned inputs, no named export staging, no remaining descriptors pointing into that test's temporary output directory, and unchanged source-recording bytes. /proc observations are Linux test instrumentation and do not introduce production process-control behavior.

During stream-copy backpressure the test identifies the actual child by its inherited source descriptor. After cancellation and media-handler return it requires that observed child to be gone, alongside resource release. For trims, rendering completes before video response headers; those streaming cases require no matching active render child at that point. They do not exercise cancellation during trim rendering.

A test-only cleanup latch holds the authenticated handler after serveDownload has returned. Shutdown must cancel the media work and release media resources, but must not persist final state while that tracked handler cleanup is held. Releasing the latch must allow joining and persistence. The test calls shutdownApplication with no HTTP server argument to isolate application cancellation/join/persistence ordering; fixture server connections/listener are cleaned separately. It is not a full runAuthenticatedServer/container SIGTERM test.

## Remaining gaps

This batch does not validate cancellation before headers or during trim rendering/probing, real RTSP acquisition, active recorders under SIGTERM, real bytes/inode exhaustion, blocked mounts, power loss, actual browser decoding/autoplay, browser disk saving, remote proxy behavior or production TLS/network characteristics. Socket buffering and scheduling can vary across kernels; fixture size and timeout behavior must be measured in CI. A failed exercise condition is not permission to weaken the assertions or report a pass.

The existing Safety verification race-enabled go test ./... command automatically discovers this test file. Its required FFmpeg environment flag already prevents missing FFmpeg/FFprobe from silently skipping media tests. No new workflow trigger, test opt-in or fake FFmpeg executable is introduced.

Preparation verified the fixture generator locally with real FFmpeg/FFprobe: the observed sample was about 1 MiB, 2.000 seconds, one yuv420p video stream, 256x144 and matching reported 10/1 rates. Generated noise means exact file size is not fixed. The Go/HTTPS integration tests have not been compiled or executed by the assistant because Go is unavailable in the preparation environment. No passing CI result is claimed for this batch. The user reported preceding commit 607bf98 green before this batch was proposed for push; that result does not cover these new tests. Keep the branch unmerged under AUDIT_ACCEPTANCE.md.
