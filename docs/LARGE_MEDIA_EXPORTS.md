# Large-media export implementation status

This branch remains work in progress, not a completed or release-ready export redesign.

## Implemented

- Bounded disk-backed prepared-output job store, owner checks, independent seekable readers, cancellation, expiry and shutdown joining.
- Authenticated POST /api/exports/create and /api/exports/cancel.
- Authenticated GET/HEAD /api/exports/status?id=ID and /api/exports/download?id=ID. Downloads use ServeContent and independent SectionReaders for HTTP Range requests, not browser Blobs.
- Owners are derived from session identity, session token and credential version. Another login cannot inherit jobs. Event tokens cannot access these endpoints. Existing cross-origin protection applies to mutations.
- Startup and shutdown lifecycle integration. Active requests are canceled and drained, then output leases and background preparations are joined.
- The current renderer is reused with pinned-input revalidation, safe generic job failures and prepared-output duration probing. Its input restrictions remain unchanged.

## Opt-in configuration

Set NVR_EXPORT_SCRATCH_DIR to an existing absolute directory on a dedicated trusted scratch mount, e.g. /exports. Mount adequate disk space there; do not use a tmpfs or the container writable layer for large exports. The directory must be outside /recordings and must not overlap camera directories or resolve through symlinks. The service makes a private temporary subdirectory. With the variable unset, only the new endpoints are disabled (503); existing downloads continue unchanged. Invalid configured limits or unavailable configured storage prevent startup.

| Setting | Default | Meaning |
| --- | --- | --- |
| NVR_EXPORT_MAX_BYTES | 8589934592 | Maximum prepared output per job (8 GiB) |
| NVR_EXPORT_TOTAL_BYTES | 17179869184 | Aggregate job-output reservation (16 GiB) |
| NVR_EXPORT_MAX_JOBS | 8 | Maximum retained job records |
| NVR_EXPORT_PARALLEL | 1 | Concurrent preparations |
| NVR_EXPORT_TTL | 30m | Queue/processing expiry; retention resets on completion |
| NVR_EXPORT_PREPARE_TIMEOUT | 30m | Preparation deadline including queue wait |

Each job reserves its maximum output allowance, so defaults admit at most two reservations. Active expired downloads remain charged until release. Intermediate files used by the legacy trim renderer retain their existing separate bounds; this reservation is not a physical disk reservation or a bound on every filesystem allocation. Monitor free space. No originals are modified by the export path. Jobs are in memory and do not survive restart.

## API

POST create with application/json:

```json
{"camera_id":"CAMERA_ID","start_time":"2026-11-21T00:00:00Z","end_time":"2026-11-21T00:01:00Z"}
```

A 202 response contains id, state, message, bytes and expires_at, with a Location status URL. Poll status; only ready jobs expose download_url. Download that same-origin URL with the same authenticated session. POST cancel with {"id":"JOB_ID"}; success is 204. Cancellation and expiry prevent new downloads; already acquired download leases are released when their requests finish. Logout/credential changes block subsequent requests but do not promise immediate revocation of an existing response.

## Remaining

- Original-file delivery, compatible audio/video stream-copy assembly and explicit consent for keyframe-adjusted ranges.
- Optional accurate audio-preserving trimming, full-range input handling and representative media validation.
- Browser job UI replacing the 64 MiB Blob workflow and corresponding real-browser acceptance updates.
- Full HTTP/media/large-transfer/slow-client/shutdown and runtime acceptance validation.

Tests added for authentication, token scope, CSRF, strict request validation, session isolation, logout/credential changes, Range/HEAD downloads, cancellation and shutdown. Go tests have not been run in the assistant environment.
