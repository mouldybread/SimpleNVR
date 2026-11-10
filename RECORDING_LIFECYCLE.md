# Recording and event lifecycle safety

API start/stop/mode-change, event admission and camera deletion share recordingControlMu. NVR.mu is released during worker waits and event pinning. Starting no_recording returns 409 without adding a WaitGroup task. Mode changes stop the old top-level worker before starting a replacement. Camera-list disk scans use scalar snapshots outside NVR.mu.

## Event workers and prebuffer

Each event-based camera has one buffer segmenter, one cleanup loop and two event workers. All are joined before the camera task completes. Publication/clearing of buffer managers uses NVR.mu. FFmpeg CSV close reports, rather than modification-time guesses, admit completed regular files into the rolling prebuffer. Accepted events pin available completed segments with same-filesystem hard links into unique staging directories.

Two total event slots bound admission and capture work. Excess requests return 429; unavailable/starting/stopping recorders return 503. Durations are 1-3600 seconds. Event labels are limited to 128 bytes and camera names to 256 bytes; duplicate names return 409. Responses distinguish recording_queued from footage confirmed saved. Metadata persistence can fail after admission, so automatic retries may duplicate an event. Stopped/continuous/no_recording cameras can still log events without capture.

The prebuffer is the available completed trailing window, not a guarantee of exactly 30 seconds. Post-trigger media still comes from a separate RTSP connection. Gaps, keyframe alignment and source continuity require live-camera tests and potentially a single-source segment-promotion design.

## Graceful command cancellation and partial files

Recorder FFmpeg commands disable stdin interaction and send SIGINT on context cancellation. Go's WaitDelay allows up to 8 seconds before force-killing a command that does not exit. This is a bounded attempt to let FFmpeg write MP4 trailers; it is not a guarantee that every interrupted file is recoverable.

Continuous segments and event assembly outputs are validated using a video-stream probe and at least one readable video packet. Outputs from failed/interrupted commands that pass these checks are published with _partial.mp4 names. Finalization uses a separate bounded 15-second context so cancellation does not immediately cancel the probes. File publication refuses an already-existing destination. Validation does not prove duration, completeness or every frame; _partial must not be treated as a complete interval.

Interrupted event acquisition retains nonempty event media and pinned prebuffer staging for inspection instead of immediately assembling under a canceled context. Canceled queued jobs retain staging only if it contains pinned prebuffer. Failed assembly retains staging and nonempty temporary output if it cannot be published. Successful assembly cleans staging. Slots are released in all these paths. Logs identify the camera/event, not camera URLs or passwords.

Startup attempts to publish validated root .tmp files as partial recordings and leaves invalid/nonvalidated files for inspection. Event staging is not automatically reassembled. Filesystem stalls can exceed application-level timing expectations.

## Verification and remaining limitations

CI installs FFmpeg and requires the media tests (no missing-tool skip in CI). A synthetic lavfi clip is interrupted after frames are produced, published as a partial MP4 and fully decoded. A helper process ignores SIGINT to test the forced-kill fallback. Other tests cover pinning/pruning, bounded admission, cancellation/joining, retained canceled prebuffer, destination protection and preserving unvalidated bytes. These tests are not yet a substitute for live Dahua RTSP/disconnect tests or runtime-image media tests.

Server/container SIGTERM handling remains unimplemented: this batch handles cancellation when the recorder context is canceled by API lifecycle controls, not process-wide shutdown. Do not assume docker stop invokes this flow yet. Power-loss/crash resilience of ordinary MP4 remains limited.

Retention/recovery still needs follow-up: preserved staging can grow and requires manual recovery/cleanup; old stale-root-.tmp cleanup can still delete inspection files. Disk-full policy, timeline durations, export boundaries, cancelable control waits and in-flight authenticated-stream revocation remain pending. Keep the branch unmerged until the remaining audit and integration tests are complete.
