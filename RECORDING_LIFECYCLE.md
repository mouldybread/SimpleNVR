# Recording and server lifecycle safety

API controls and event admission use recordingControlMu. NVR.mu is released during recorder waits and event pinning. Disabled starts return 409 without allocating workers. Event recorders join buffer maintenance and two bounded event workers. Completed FFmpeg CSV-reported prebuffer segments are hard-linked into unique event staging. Capacity limits, asynchronous acceptance and retained failure staging are intentional.

## Recorder cancellation and publication

FFmpeg receives SIGINT on recorder-context cancellation, with an 8-second force-kill fallback. Continuous and assembly outputs are probed for a video stream and a readable packet before publication; interrupted outputs are named _partial.mp4. Detached finalization has a 15-second budget. This does not prove completeness, duration accuracy, every-frame validity, gapless continuity or power-loss resilience. Failed capture retains staging even when empty; failed/interrupted assembly preserves inputs even after partial publication. See EVENT_FAILURE_PRESERVATION.md and EVENT_RECOVERY.md.

## SIGTERM/SIGINT and restart intent

Signal handling is installed before context-aware startup recovery. TLS/template/config initialization precedes recorder startup. Listener failures also enter cleanup.

On shutdown the request gate rejects new requests with 503, prevents new recorder starts and tracks admitted mutations. It also tracks export and live-proxy handlers, cancels their contexts and joins their cleanup. Export FFmpeg uses the interrupt/force-kill helper; proxy upstreams use the request context. Cancellation attempts to set a downstream write deadline. HTTP draining gets five seconds before connections are closed. Persistence follows recorder completion, tracked-handler completion and HTTP shutdown. The overall budget is 60 seconds; timeout/failure produces a nonzero exit rather than claiming success.

Shutdown preserves IsRecording as restart intent, whereas explicit API stop clears it. Mode changes interrupted while waiting for old workers restore the previous mode and intent rather than launch replacements. During shutdown IsRecording does not prove a worker is still running. A timed-out shutdown cannot promise final persistence completed; atomic replacement preserves the prior complete file if replacement did not finish.

Configure the Compose/TrueNAS service with an outer grace longer than the application budget:

```yaml
stop_grace_period: 90s
```

The image uses the Go executable as entrypoint and declares STOPSIGNAL SIGTERM. SIGKILL, power loss or a shorter orchestrator timeout bypass or truncate cleanup. Filesystem stalls and uninterruptible lock waits remain outside normal timing guarantees.

## Retained staging protection

Root camera *.tmp files are never deleted by periodic or manual orphan cleanup, regardless of age, size or relative modification time. Empty files may still belong to live recorders. Owning recorders retain their existing ability to remove empty failed outputs after their process has joined.

Registered camera directories are skipped by orphan cleanup. Unregistered directories containing any root .tmp entry, a nonempty temp directory, an unexpected temp entry type or an inspection error are preserved in their entirety. The response includes skipped_recovery_dirs and failures; count and size_mb include only successful deletions. Cleanup does not recursively remove opaque contents. Root media symlinks are skipped; this is not complete protection against hostile concurrent filesystem replacement.

Published root .mp4 files, including published _partial.mp4 files, remain subject to normal camera retention. Explicit camera deletion and bulk camera deletion remain destructive, including staging. Protect required evidence before using those actions. The delete-all-recordings endpoint removes published root MP4s, not nested event staging or root .tmp outputs.

### Recovery procedure and limits

Stop SimpleNVR and wait for clean shutdown before manually inspecting, moving or deleting staging. Preserve a separate copy of camera configuration, event metadata and the affected camera directory before attempting recovery. Event staging can contain pinned pre_*.mp4, event.mp4, concat.txt, recovery.json and failed recovery-*.json.tmp snapshots; retain them together. Do not blindly execute retained concat lists or claim that pinned prebuffer alone constitutes a complete event.

At startup, existing recovery probes root .tmp outputs and attempts to publish readable media as _partial.mp4. Files it cannot validate or publish remain in place. Nonempty event staging is logged and preserved, not automatically reassembled. New event jobs persist versioned recovery identity, input intent and processing/publication checkpoints; legacy staging is not migrated. See EVENT_RECOVERY.md for failure ordering, operator interpretation and power-loss limitations. Opt-in free-space and bounded staging admission checks, plus authenticated read-only storage status, are implemented; see STORAGE_ADMISSION.md, EVENT_STAGING_ADMISSION.md and STORAGE_STATUS.md. These are not runtime quotas, reservations or a complete disk-full policy. There is no automatic staging expiry or reassembly. Preserved files and pinned hard links continue to consume space; ordinary camera retention does not reclaim nested staging. Operators must monitor storage and explicitly decide when preserved artifacts can be removed. Do not interpret this preservation patch as resolving those remaining storage policies.

## Verification scope and remaining work

Filesystem tests cover preservation of old and empty root .tmp outputs, nested event staging, orphan directories containing recovery artifacts, inspection failure and symlink cases, plus continued retention of published media. Local HTTP tests cover bounded go2rtc scans, proxy cancellation, header filtering and child-process reaping before shutdown drain. The container SIGTERM smoke test uses an empty camera registry; it does not exercise live-camera media finalization. Full authenticated slow-client testing and actual FFmpeg export lifecycle integration remain gaps.

Periodic background cleanup now participates in shutdown cancellation and joining before persistence; see BACKGROUND_CLEANUP.md. Retention bounds and snapshot serialization are described in RETENTION_POLICY.md and RETENTION_SNAPSHOT.md. Scoped browser export reporting, absolute-time/DST geometry and single-camera contiguous advancement are implemented; their remaining limits are described in EXPORT_UI.md, TIMELINE_API.md and PLAYBACK_CONTINUATION.md. Remaining work includes validated operator recovery, runtime write-failure/disk-full behavior, live Dahua interruption tests, real-browser and authenticated export integration, playback failure handling, and final destructive-path, frontend/proxy security and lifecycle review. Older timeline/export remaining-work summaries still require reconciliation. The legacy non-context recovery helper remains unused by main pending consolidation. Keep the fix branch unmerged until remaining issues and the final re-audit are complete.
