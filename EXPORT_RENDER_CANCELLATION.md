# Authenticated trim cancellation before response headers

## Scope

This tests-only batch complements EXPORT_NETWORK_VALIDATION.md. The earlier streaming tests begin after a video response exists; these new cases must observe the real libx264 trim-render child while the authenticated HTTPS client is still waiting for headers. They exercise client cancellation and application shutdown before the prepared video response can start. No production handler, recording, export, authentication or recovery policy changes.

The test reuses the session-authenticated loopback TLS fixture from export_network_test.go. Before any media request, its test-only source is replaced with a bounded synthetic 8-second, 512x288, 10-fps yuv420p clip containing temporal noise. The requested trim is 0.2-7.8 seconds. Fixture generation has a 20-second deadline and a 32 MiB size cap; these are test limits, not new product limits. The larger fixture provides a render-observation window, but the test must fail rather than pretend it exercised cancellation if no renderer is observed in time.

## Deterministic forced-stop condition

The harness identifies its own child using the inherited source descriptor and verifies libx264/filter-complex arguments. It opens a Linux pidfd, rechecks the source descriptor and sends SIGSTOP through that pidfd. It requires an observed stopped state, an occupied export slot, an anonymous unlinked trim-output descriptor and no completed client response. No fake FFmpeg executable, replacement renderer or process-global production hook is used.

SIGSTOP is deliberate fault injection. A stopped child cannot process the existing SIGINT cancellation request, so cancellation/shutdown must reach the existing interruptible-command force-kill allowance, reap the child and return from rendering before pins/output are released. These are tests of a stalled renderer and forced cleanup, not graceful decoder cancellation or a claim that ordinary renders require force-killing.

Pidfds keep signal cleanup bound to the observed process identity rather than an unrelated reused PID. Test cleanup attempts SIGCONT and closes the pidfd if a failure occurs before normal cancellation completes. HTTP worker cancellation and joining, fixture connection shutdown and tracked-handler drain are also part of cleanup. Linux procfs and pidfd support are requirements for these integration tests on the existing Ubuntu runner; they are not new production endpoint requirements.

## Required observations

For both client cancellation and application shutdown:

- Rendering is observed and suspended before response completion; unexercised cases fail.
- The original source bytes remain unchanged.
- The observed child is reaped before media-handler completion is accepted.
- Export admission is released, pinned inputs are closed, and named/anonymous export temporary resources are gone.
- Any returned HTTP response has no video Content-Type, attachment Content-Disposition or Trim-Applied success header. A canceled transport may instead return an error; no exact canceled HTTP status is invented.

The shutdown case additionally holds the existing test-only post-media cleanup latch. Persistence must not run while that tracked handler cleanup is held; after release, shutdown must join and persist. As in the streaming batch, shutdownApplication receives no HTTP server argument, isolating application job cancellation/join/persistence. The TLS fixture is cleaned separately; this is not a container SIGTERM test or validation of HTTP server draining with active recorders.

## Evidence and limits

Preparation ran only separate native probes: real FFmpeg generated an observed approximately 14.8 MB, 8.000-second, single-video 512x288 yuv420p fixture with matching 10/1 rates. A Python-owned real FFmpeg process was stopped through a pidfd; it did not exit after SIGINT while stopped, and SIGKILL terminated it. These establish fixture/signal behavior on that environment, not Go renderer integration, Go Cmd.WaitDelay timing, HTTPS cancellation, resource cleanup or passing repository tests.

The assistant has not compiled or executed the new Go tests because Go is unavailable in the preparation environment. Their first actual Go integration execution will be in Safety verification after a separately approved push. The preceding 117ac28 streaming-test commit was reported green by the user before this batch was proposed for push; that result does not cover these new tests.

Two forced-stop cases intentionally add roughly two existing force-kill grace intervals to normal suite runtime. Total race-suite duration, scheduler behavior and Linux pidfd availability must be measured in CI; this draft does not silently increase workflow timeouts or weaken exercise/cleanup assertions. Existing go test -race -count=1 -timeout=120s ./... discovers the tests automatically.

This batch does not cover cancellation during metadata/profile probing, normal graceful trim cancellation without SIGSTOP, live-camera acquisition, active-recorder SIGTERM, disk/inode exhaustion, blocked filesystems, remote transport behavior, power loss, or real-browser decoding/disk saving. Keep all broader AUDIT_ACCEPTANCE.md gates open and the audit branch unmerged.
