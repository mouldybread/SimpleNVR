# Large-media exports: work in progress

Do not release this branch as the completed export redesign.

## Implemented

- Bounded disk-backed job store, random IDs, owner checks, independent seekable readers, cancellation, expiry and shutdown joining.
- Authenticated POST /api/exports/create and /api/exports/cancel, plus GET/HEAD /api/exports/status?id=ID and /api/exports/download?id=ID. Event tokens cannot access these routes; jobs are bound to session and credential version.
- Startup and shutdown integration. Download responses use ServeContent with Range support.
- Browser UI creates jobs, polls state, and offers a normal same-origin download. Page JavaScript does not fetch/buffer video or impose the old 64 MiB cap. Control responses are bounded to 8 KiB.
- Range controls are retained. Cameras are processed sequentially; the next job starts after release or expiry of the prepared file.
- Cancel/release requires server acknowledgement; failures remain visible with a retry control. Lost creation responses are not retried automatically. Navigation leaves jobs to server expiry rather than canceling a download during handoff.

## Configuration

Mount trusted disk-backed scratch storage outside /recordings and camera directories (for example /exports) and set NVR_EXPORT_SCRATCH_DIR=/exports. It must exist and must not resolve through symlinks. Without it the new endpoints return 503; the job UI reports that configuration is required and does not fall back to the old Blob path. Invalid configured limits/storage fail startup. Do not put GiB exports in a tmpfs or container writable layer.

| Setting | Default | Meaning |
| --- | --- | --- |
| NVR_EXPORT_MAX_BYTES | 8589934592 | Per-job prepared output (8 GiB) |
| NVR_EXPORT_TOTAL_BYTES | 17179869184 | Aggregate job-output reservation (16 GiB) |
| NVR_EXPORT_MAX_JOBS | 8 | Retained records |
| NVR_EXPORT_PARALLEL | 1 | Concurrent preparations |
| NVR_EXPORT_TTL | 30m | Processing expiry; retention resets on completion |
| NVR_EXPORT_PREPARE_TIMEOUT | 30m | Preparation deadline including queue wait |

Each admitted job reserves its maximum output; defaults allow at most two reservations. Active expired downloads remain charged until their leases end. Legacy trim intermediates retain separate existing bounds. Reservations are not physical disk reservations or a bound on all filesystem allocations. Monitor free space. Jobs do not survive restart.

POST create JSON contains camera_id, start_time and end_time (ordered RFC3339 timestamps). A 202 response has id, state, message, bytes, expires_at and a Location status URL. Ready status adds download_url. POST cancel uses {"id":"JOB_ID"} and confirms with 204. Subsequent requests require the creating session. Cancellation/expiry prevent new leases; already-started responses may continue. Logout and credential changes block subsequent requests, not necessarily existing responses.

## Still required before release

- Original-file delivery, compatible audio/video stream-copy assembly, and consent for adjusted keyframe ranges. The current renderer and its audio/pixel-format/frame-rate restrictions are unchanged.
- Optional accurate audio-preserving trimming and representative media validation.
- Migrate the real Chrome and routed browser acceptance harnesses from Blob URLs to job create/status/download/release semantics; enable scratch mounts in runtime fixtures. Existing browser harnesses still contain old assumptions and are not expected to pass this intermediate branch. Do not delete their media, hash, duration, playback or cancellation checks.
- Real large-file, slow-client, media, Range, authenticated-session and shutdown runtime checks.

JavaScript syntax and the updated Node export UI/range regressions passed locally in UTC and America/Chicago. The 5 GiB test uses reported size metadata, not an actual 5 GiB transfer. Go and real-browser tests have not been run here.
