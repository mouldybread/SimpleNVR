# Recording and server lifecycle safety

API controls and event admission use recordingControlMu. NVR.mu is released during worker waits and event pinning. Disabled starts return 409 without allocating workers. Event recorders join buffer maintenance and two bounded event workers. Completed FFmpeg CSV-reported prebuffer segments are hard-linked into unique event staging. Capacity limits, asynchronous acceptance and retained failure staging are intentional.

## Recorder command cancellation

FFmpeg receives SIGINT on recorder-context cancellation, with an 8-second force-kill fallback. Continuous and assembly outputs are probed for a video stream and a readable packet before publication; interrupted outputs are named _partial.mp4. Detached finalization has a 15-second budget. This does not prove completeness, duration accuracy, every-frame validity, gapless media continuity or power-loss resilience. Interrupted acquisition and failed assembly retain nonempty staging for inspection.

## SIGTERM/SIGINT and restart intent

The application installs signal handling before startup recovery and accepts SIGTERM/SIGINT as shutdown requests. Startup recovery now uses the signal context and stops before launching recorders if cancellation occurs. TLS/template/configuration initialization is performed before recorder startup. A listener/server failure also enters recorder cleanup rather than immediately exiting with active recorders.

On shutdown, a request gate rejects new requests with 503. The runtime shutdown flag prevents new recording workers, including already-admitted controls that reach a start later. Existing admitted state-changing requests are tracked. Recorder cancellation is broadcast to all cameras before waiting. HTTP shutdown gets up to 5 seconds before connections are closed. Persistence waits for recorder completion, mutation-request draining and HTTP shutdown. The overall application budget is 60 seconds; failure or timeout produces a nonzero exit rather than a false clean-shutdown message.

Process shutdown cancels runtime workers without clearing the persisted IsRecording intent. Previously active cameras can resume on the next process start; paused cameras remain paused. Explicit API stop still clears intent. A mode change interrupted during its worker wait rejects the replacement and restores the previous recording intent and mode for restart. Runtime cancellation functions, buffer managers and the shutdown flag are not persisted.

During shutdown IsRecording represents restart intent, not proof that a worker is still running. A timed-out shutdown cannot promise that the final persistence operation completed; atomic configuration replacement preserves the prior complete file if replacement did not finish. Filesystem stalls remain outside normal timing guarantees.

For the SimpleNVR Compose/TrueNAS custom-app service, configure an outer stop grace longer than the application budget:

```yaml
stop_grace_period: 90s
```

The image explicitly declares STOPSIGNAL SIGTERM and uses the Go application directly as its entrypoint. SIGKILL, power loss and a shorter orchestrator stop timeout bypass or truncate graceful cleanup.

## Verification scope

Unit tests cover shutdown admission, joining injected workers, draining admitted mutations, preserving active/paused intent through save/load/resume, mode-change interruption and returning an error when shutdown is canceled. Existing synthetic FFmpeg tests cover interrupted MP4 decoding and forced-kill fallback. CI adds a real runtime-image SIGTERM smoke test with an empty camera registry, generated temporary administrator credentials and exit-code/configuration checks. The smoke test does not exercise a live camera or media finalization inside that container.

## Remaining work

Retained staging needs recovery/retention and disk-full policy; existing root-.tmp cleanup can still remove inspection files. Live Dahua RTSP/disconnect tests, timeline/export correctness, both frontend XSS paths and proxy hardening remain pending. Existing export/proxy jobs are not yet explicitly joined like recorder workers; HTTP connections are closed on shutdown but that is not a guarantee that every non-recorder subprocess has been reaped. An existing unbounded go2rtc scan or stalled mutation can cause the 60-second drain to fail; bounded/cancelable upstream requests are a follow-up. The legacy non-context recovery helper remains unused by main pending consolidation. Keep the fix branch unmerged until these issues and the final re-audit are complete.
