# Large-media export implementation status

This branch is work in progress. The first commit adds an isolated server-side export job store and tests only. It does NOT yet register HTTP routes, change the UI, remove the browser Blob limit, or broaden rendering compatibility. Do not release this commit as the completed export redesign.

## Implemented foundation

- Bounded jobs and preparation concurrency, random job IDs, and explicit owner checks.
- Private disk-backed output, immediately unlinked and retained by file descriptor; no recording cleanup or source modification.
- Independent seekable download leases for eventual HTTP Range delivery, without page-level whole-file buffering.
- Cancellation, expiry, disk-reservation accounting, bounded writes, and shutdown joining. Existing transfers retain their leases until released.
- Safe public status messages; raw renderer errors are not exposed.

The renderer callback must validate the export and join its subprocesses before returning. The future HTTP integration must bind owners to authenticated sessions/credential versions, never caller-supplied owner strings. It must cancel and drain downloads before waiting for store shutdown.

## Planned startup configuration

These settings are parsed by the new helper but are NOT wired into startup yet:

| Setting | Default | Meaning |
| --- | --- | --- |
| NVR_EXPORT_MAX_BYTES | 8589934592 | Maximum prepared output per job (8 GiB) |
| NVR_EXPORT_TOTAL_BYTES | 17179869184 | Aggregate reserved output capacity (16 GiB) |
| NVR_EXPORT_MAX_JOBS | 8 | Maximum retained job records |
| NVR_EXPORT_PARALLEL | 1 | Concurrent preparations |
| NVR_EXPORT_TTL | 30m | Queue/processing expiry; retention resets on completion |
| NVR_EXPORT_PREPARE_TIMEOUT | 30m | Deadline including queue admission wait |

Each admitted job reserves its maximum output allowance; default aggregate capacity therefore admits at most two such reservations at once. Expired files held by active downloads remain charged. Invalid limits fail closed. Filesystem exhaustion can still fail a job; reservations do not reserve physical blocks or replace filesystem capacity monitoring. Use trusted private scratch storage with sufficient free space.

## Remaining integration

1. Authenticated POST creation/cancellation and GET status/download endpoints; no state-changing GET requests or bearer-token bypass.
2. Startup, shutdown-gate and HTTP transfer cancellation wiring.
3. Original-file delivery, compatible audio/video stream-copy assembly, and explicit preview/consent for keyframe-adjusted boundaries.
4. Optional accurate trimming preserving audio timing and handling full-range input correctly; no silent audio removal or conversion fallback.
5. Browser job UI without whole-file Blob buffering; update real-browser acceptance tests.
6. Media, authentication, quota, expiry, cancellation, shutdown, large-transfer and source-preservation integration tests.

The Go tests accompanying the first commit have not been executed in the assistant environment; Go is unavailable there.
