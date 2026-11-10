# Timeline windows and recording intervals

GET /api/timeline/recordings accepts one RFC3339 start_time/end_time pair with explicit offsets or Z, a positive elapsed window of at most 26 hours, and no date parameter. Boundaries are normalized to UTC. Duplicate/missing boundaries, mixed date/range requests and invalid ranges return 400. The legacy date=YYYY-MM-DD path remains HKT-based for old clients; no parameters mean the current HKT day. The browser uses independent local-midnight/next-local-midnight boundaries converted to UTC, including 23-/25-hour DST days, without falling back to legacy date requests.

Responses remain JSON arrays. X-Timeline-Window-Start/End expose normalized boundaries; X-Timeline-Window-Mode is absolute or legacy-hkt. X-Timeline-Duration-Policy is probed-media. StartTime retains the original UTC filename timestamp; EndTime adds the probed video-stream duration, falling back to container duration only when the video stream lacks a usable duration. This is metadata-derived media length, not verified uninterrupted coverage or a calibrated camera clock.

Timeline and export input selection share the strict overlap rule: recording_start < window_end and recording_end > window_start. Clips beginning before the requested day are included if they overlap; clips ending exactly at the start or beginning exactly at the end are excluded. Returned starts are not clipped, preserving playback offsets. Export additionally validates its requested coverage through the planner below.

## Export range planning

The planner copies and deterministically sorts intersections by start/end/path, preserves each original clip/identity, and records each intersection's absolute start/end and input-relative In/Out offsets. Nanosecond boundaries are retained. It requires one camera and complete, non-overlapping metadata coverage of the requested interval. It does not mutate the selection input.

Missing leading/trailing footage, interior gaps, duplicate intervals and ambiguous overlaps return 409 before input pinning or video response headers. X-Export-Plan-Status is gap or overlap. No overlap is resolved by guessing a preferred clip, and no gap is padded or silently compressed. Empty input selections retain the existing 404. Invalid intervals return 400; more than 128 selected clips return 413. Comparisons are strict with no rounding tolerance; metadata rounding, filename timestamp granularity and camera timing can therefore cause nominally adjacent recordings to be rejected.

This commit implements planning and validation, not trim execution. Successful responses still concatenate full clips using stream copy. X-Export-Range-Policy is whole-clips-untrimmed; X-Export-Requested-Start/End identify the requested interval, X-Export-Media-Start/End identify the full clips' metadata-derived outer bounds, and X-Export-Trim-Required is true when the plan requires boundary trimming. These headers do not claim verified first/last-frame wall-clock times. Output may contain footage outside the requested interval. The existing UI does not yet display these response-policy headers.

The In/Out offsets are not passed to the encoder yet. Adding concat inpoint/outpoint under stream copy must not be advertised as frame-exact trimming. A separately tested trim execution policy, codec compatibility handling and user-visible range reporting remain follow-ups.

## Metadata resource and failure policy

The Linux runtime probes an already-open, regular, nonempty MP4 descriptor with ffprobe using MOV demuxing and file/pipe protocols. Each probe has a five-second context budget including semaphore waiting, with a two-second WaitDelay for process/pipe cleanup. At most two metadata probes execute concurrently. Metadata selection has a 30-second request budget and scans at most 8192 directory entries across its camera snapshot. Filesystem stalls remain outside context timing guarantees.

The shared in-memory LRU caches at most 2048 successful durations. Hits require matching file identity, size and modification time. Changes during probing or pathname replacement prevent caching. Failed probes are not cached. This is not cryptographic content validation: hostile same-inode rewrites restoring size/mtime and parent-directory replacement are not fully prevented.

Files beginning at/after the window end need no probe. Earlier files must be inspected because their overlap cannot otherwise be established. Cold or large archives can exceed the scan/time budget. Unreadable eligible media, changed files and scan limits fail the entire selection with 503; metadata deadlines return 504. X-Recording-Metadata-Status is unavailable. No partial array/export or invented ten-minute fallback is returned. A corrupt old file can block selection until inspected or repaired. Request cancellation stops probes; timeline/export/proxy jobs participate in shutdown tracking.

## Export input pinning

Selected intervals retain the file identity, size and modification time returned by duration inspection. Export reopens each input with O_NOFOLLOW and O_NONBLOCK, checks that it matches the selected identity and remains a nonempty regular file, and retains the verified descriptor through FFmpeg Run. Missing/replaced inputs fail before video response headers. Descriptor metadata is rechecked when building the command.

The concat manifest contains only generated /proc/self/fd/N references. It is created owner-only, unlinked immediately and inherited as descriptor 3; media inputs begin at descriptor 4. FFmpeg uses file/pipe protocols with concat/mov format whitelists and does not reopen original recording pathnames. Inputs unlinked or renamed after pinning remain readable through their descriptors. Original names, including quotes/backslashes, never enter the manifest.

At most two pinned exports and 128 inputs per export are admitted. Admission and preparation share the existing 30-second metadata budget; streaming uses the client/shutdown context rather than that preparation deadline. A preparation failure closes partially opened inputs and releases its slot. On all later return paths, descriptors close and the slot releases only after Run returns; cleanup is idempotent. The unlinked manifest leaves no named staging file on process exit. Open descriptors can retain disk blocks until exports finish.

X-Export-Input-Policy is pinned-descriptors. Pinning protects inode identity against unlink/pathname replacement, not immutable content against in-place writes, privileged filesystem changes or malicious MP4 behavior. Validation-to-execution content changes are still possible. This is not a media sandbox or an export correctness guarantee.

## Verification scope

Tests cover metadata/cache limits, interval overlap and synthetic short media, export pinning/cleanup and real descriptor-based FFmpeg output. Planner tests cover partial/adjacent clips, ordering, fractional offsets, gaps, duplicate/overlapping intervals and pre-stream rejection. A synthetic handler test verifies that a three-second requested interval still produces four seconds of whole-clip media with explicit untrimmed policy headers, rather than claiming trimming was implemented. Cancellation tests use a blocking writer, not a full authenticated slow-client integration.

Full authenticated browser playback/export integration, the canvas's fixed 24-hour/DST geometry, automatic segment advancement, trim execution, user-visible export error/range reporting, storage recovery policy and final re-audit remain pending. Keep the audit branch unmerged.
