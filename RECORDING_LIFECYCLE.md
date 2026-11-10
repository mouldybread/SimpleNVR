# Recording and event lifecycle safety

API start/stop/mode-change, event admission and camera deletion share recordingControlMu. NVR.mu is released during worker waits and event pinning. Starting no_recording returns 409 without adding a WaitGroup task. Mode changes stop the old top-level worker before starting a replacement. Camera-list disk scans use scalar snapshots outside NVR.mu.

## Event workers and completed prebuffer

Each event-based camera runs one buffer segmenter, one cleanup loop and two event workers. All four are joined before the camera's top-level task completes. Event recording and assembly commands derive from the camera context; stop/mode-change/delete cancels them and waits for them before deleting storage or starting another recorder. Buffer-manager publication and clearing use NVR.mu.

FFmpeg emits a CSV segment-list entry when a buffer segment closes. Only those reported regular files enter the prebuffer. At event admission, completed segments in the available trailing window are hard-linked into a unique staging directory on the same filesystem. Cleanup can unlink the rolling source names without deleting the pinned inputs. No cross-filesystem hard-link fallback is provided; storage errors reject admission.

There are two total event slots, including admitted jobs not yet picked up. This avoids a long queue that would start post-trigger capture late. Excess events return HTTP 429 with Retry-After. A starting/stopping recorder returns 503. Duration is limited to 1-3600 seconds; labels are limited to 128 bytes and camera names to 256 bytes for event submissions. Duplicate camera names make name-based event submission ambiguous and return 409.

Successful responses retain success/event_id and add recording_queued. Success means event metadata was persisted and, if requested, a capture was admitted; it does not mean footage has finished saving. Stopped, continuous and no_recording cameras can still log events without starting event capture. Admission failure does not append event metadata. A later persistence failure can occur after a capture was admitted and returns 500 explicitly; automatic retries can produce duplicates.

Event staging names are unique. Concat lists use generated relative basenames with safe mode enabled. Assembly writes a temporary file, probes for a video stream, then renames into the visible recordings directory. Capture/assembly failures and cancellation are logged and remove that job's staging; there is no completed-capture status API yet.

## Limitations and pending work

The prebuffer is the available completed window, not a guaranteed exact 30 seconds: startup, keyframe spacing, source interruptions and delayed segment closure affect coverage. Post-trigger footage still comes from a separate RTSP connection; gapless/keyframe-accurate continuity is not established. Completed event filenames use the earliest pinned segment's start, but timeline durations/export boundaries still need their separate fix.

Commands currently use context-driven forced termination. Joining them fixes orphan workers but does not guarantee MP4 finalization on interruption. Graceful FFmpeg shutdown, server SIGTERM handling, live-camera tests, cancelable control waits and in-flight authenticated-stream revocation remain pending.

Startup preserves event temp staging left by a crash instead of wiping it; assembly recovery is not automated and these retained directories need manual recovery/cleanup until the retention/recovery follow-up. Existing stale-root-.tmp cleanup can still remove old partial files. Disk-full and crash behavior require integration tests.

Unit tests exercise actual hard links and pruning, exclusion of unreported active files, capacity limits, cancellation/draining and joining with injected maintenance/event tasks. They do not execute FFmpeg or prove media continuity. CI and live-camera verification must pass before deployment.
