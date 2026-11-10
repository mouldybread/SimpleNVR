# Timeline windows and recording intervals

GET /api/timeline/recordings accepts one RFC3339 start_time/end_time pair with explicit offsets or Z, a positive elapsed window of at most 26 hours, and no date parameter. Boundaries are normalized to UTC. Duplicate/missing boundaries, mixed date/range requests and invalid ranges return 400. The legacy date=YYYY-MM-DD path remains HKT-based for old clients; no parameters mean the current HKT day. The browser uses independent local-midnight/next-local-midnight boundaries converted to UTC, including 23-/25-hour DST days, without falling back to legacy date requests.

Responses remain JSON arrays. X-Timeline-Window-Start/End expose normalized boundaries; X-Timeline-Window-Mode is absolute or legacy-hkt. X-Timeline-Duration-Policy is probed-media. StartTime retains the original UTC filename timestamp; EndTime adds the probed video-stream duration, falling back to container duration only when the video stream lacks a usable duration. This is metadata-derived media length, not verified uninterrupted coverage or a calibrated camera clock.

Timeline and export input selection share the strict overlap rule: recording_start < window_end and recording_end > window_start. Clips beginning before the requested day are included if they overlap; clips ending exactly at the start or beginning exactly at the end are excluded. Returned starts are not clipped, preserving playback offsets. Export selects whole overlapping clips; trimming, gaps and duplicate/overlap policy are not implemented.

## Metadata resource and failure policy

The Linux runtime probes an already-open, regular, nonempty MP4 descriptor with ffprobe using MOV demuxing and file/pipe protocols. Each probe has a five-second context budget including semaphore waiting, with a two-second WaitDelay for process/pipe cleanup. At most two metadata probes execute concurrently. Metadata selection has a 30-second request budget and scans at most 8192 directory entries across its camera snapshot. Filesystem stalls remain outside context timing guarantees.

The shared in-memory LRU caches at most 2048 successful durations. Hits require matching file identity, size and modification time. Changes during probing or pathname replacement prevent caching. Failed probes are not cached. This is not cryptographic content validation: hostile same-inode rewrites restoring size/mtime and parent-directory replacement are not fully prevented.

Files beginning at/after the window end need no probe. Earlier files must be inspected because their overlap cannot otherwise be established. Cold or large archives can exceed the scan/time budget. Unreadable eligible media, changed files and scan limits fail the entire selection with 503; metadata deadlines return 504. X-Recording-Metadata-Status is unavailable. No partial array/export or invented ten-minute fallback is returned. A corrupt old file can block selection until inspected or repaired. Request cancellation stops probes; timeline/export/proxy jobs participate in shutdown tracking.

## Export input pinning

Selected intervals retain the file identity, size and modification time returned by duration inspection. Export reopens each input with O_NOFOLLOW and O_NONBLOCK, checks that it matches the selected identity and remains a nonempty regular file, and retains the verified descriptor through FFmpeg Run. Missing/replaced inputs fail before video response headers. Descriptor metadata is rechecked when building the command.

The concat manifest contains only generated /proc/self/fd/N references. It is created owner-only, unlinked immediately and inherited as descriptor 3; media inputs begin at descriptor 4. FFmpeg uses file/pipe protocols with concat/mov format whitelists and does not reopen original recording pathnames. Inputs unlinked or renamed after pinning remain readable through their descriptors. Original names, including quotes/backslashes, never enter the manifest.

At most two pinned exports and 128 inputs per export are admitted. Too many selected clips return 413 with a request to reduce the time range. Admission and preparation share the existing 30-second metadata budget; streaming uses the client/shutdown context rather than that preparation deadline. A preparation failure closes partially opened inputs and releases its slot. On all later return paths, descriptors close and the slot releases only after Run returns; cleanup is idempotent. The unlinked manifest leaves no named staging file on process exit. Open descriptors can retain disk blocks until exports finish.

X-Export-Input-Policy is pinned-descriptors. Pinning protects inode identity against unlink/pathname replacement, not immutable content against in-place writes, privileged filesystem changes or malicious MP4 behavior. Validation-to-execution content changes are still possible. This is not a media sandbox or an export correctness guarantee.

## Verification scope

Tests cover metadata decoding/cache limits, overlap boundaries and synthetic short media, plus export identity propagation, pre-pin replacement rejection, unlink/path replacement after pinning, symlink rejection, partial preparation failures, cancellation, admission limits, pre-stream errors and real FFmpeg concat/decode from inherited descriptors. Cancellation tests exercise the command/cleanup path with a blocking writer, not a full authenticated slow-client integration.

Full authenticated browser playback/export integration, the canvas's fixed 24-hour/DST geometry, automatic segment advancement, export trimming/gaps, storage recovery policy and final re-audit remain pending. Keep the audit branch unmerged.
