# Recording-control safety: current batch

All API start/stop/mode-change and camera-deletion operations now share recordingControlMu. This intentionally serializes control operations across cameras; the NVR state mutex is released while waiting for a top-level worker or deleting directories, so state readers are not blocked by those waits. Startup runs before HTTP accepts requests and retains its existing startup path.

Starting a no_recording camera returns HTTP 409 without allocating a context, incrementing the WaitGroup or setting IsRecording. Unknown cameras return 404. Invalid mode, missing IDs, malformed/trailing JSON and unknown control-request fields are rejected.

Mode changes cancel and wait for the current top-level worker before changing the mode and starting a replacement. Changing to continuous/event_based preserves the previous API behavior of starting recording; changing to no_recording leaves it stopped. Repeating the current mode does not implicitly start a stopped camera.

Deletion uses the same control lock and validates camera paths before stopping/deleting. Bulk deletion operates on its initial snapshot, so concurrently added cameras are not silently removed from the registry. Partial deletion failures persist the remaining registry and return an error. Persistence failures explicitly report that runtime state may already have changed. Camera-list filesystem scans and template rendering use scalar snapshots outside the shared state lock.

Tests use injected top-level workers without FFmpeg and cover the disabled-start WaitGroup leak, invalid controls, concurrent transitions, waiting outside the state lock and control-request parsing. They do not establish live-camera or event-child-worker safety.

Still pending: tracking/joining all event-based child goroutines, clearing/replacing buffer managers safely, preserving pre-event segments, graceful FFmpeg finalization, request cancellation while waiting for control serialization, and live-camera shutdown/recovery tests. In particular, event-mode deletion is not yet guaranteed to wait for every child process. Do not treat this batch as completion of the recording audit.
