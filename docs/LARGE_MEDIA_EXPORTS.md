# Large-media exports

SimpleNVR prepares exports on the server and serves finished files through authenticated downloads. Page JavaScript does not buffer video or impose the old 64 MiB limit. Server output limits still apply.

## How it works

- Disk-backed jobs have random IDs, session ownership, independent seekable readers, cancellation, expiry, and shutdown joining.
- The browser creates a job, polls its status, then offers a normal same-origin download with HTTP Range support. Control responses are bounded to 8 KiB.
- The UI processes cameras sequentially. The next job starts after the previous prepared file is released or expires.
- Cancel/release requires server acknowledgement. Failures remain visible with a retry control. A lost creation response is not retried automatically.
- Navigation leaves jobs to expire rather than cancelling a download during browser handoff. Jobs do not survive restart.
- Copy and Accurate are explicit choices. Copy never silently falls back to re-encoding. See [export rendering](EXPORT_RENDERING.md).

## Configure export storage

Mount a trusted, private, disk-backed directory outside `/recordings` and every camera directory, for example `/exports`, and set:

```text
NVR_EXPORT_SCRATCH_DIR=/exports
```

The configured container path must be absolute, already exist, and not resolve through symlinks. The container user must be able to write it. Camera storage and export scratch must not overlap in either direction. Do not place multi-GiB exports in tmpfs or the container's writable layer. The [README Compose example](../README.md#install-with-docker-compose) includes the mount and setting.

When the setting is absent, the application can start but prepared export endpoints return 503. There is no browser-memory fallback. Invalid configured limits or unsuitable configured storage cause startup to fail.

| Setting | Default | Meaning |
| --- | --- | --- |
| `NVR_EXPORT_MAX_BYTES` | 8589934592 | Maximum prepared output per job (8 GiB) |
| `NVR_EXPORT_TOTAL_BYTES` | 17179869184 | Aggregate job-output reservation (16 GiB) |
| `NVR_EXPORT_MAX_JOBS` | 8 | Maximum retained job records |
| `NVR_EXPORT_PARALLEL` | 1 | Concurrent preparations |
| `NVR_EXPORT_TTL` | 30m | Initial job expiry; retention resets when preparation finishes |
| `NVR_EXPORT_PREPARE_TIMEOUT` | 30m | Preparation deadline, including queue wait |

Byte limits are positive integers in bytes. Counts must be positive; maximum jobs cannot exceed 64 and parallel preparations cannot exceed maximum jobs. Total bytes must be at least the per-job limit. Durations use Go duration syntax such as `30m` or `1h`, must be positive, and cannot exceed 24 hours.

Each admitted job reserves its maximum output, including queued jobs, so the default byte budget permits at most two concurrent reservations. This does not mean two renders run in parallel: the default parallel setting is one. Retained records also count toward the job limit.

Expired or released outputs with active readers stay charged until their readers finish. Reservations are accounting limits, not preallocated physical disk space or a bound on every filesystem allocation. Monitor free space and underlying storage health.

## API

All routes require the creating authenticated dashboard session; event tokens cannot use them. Jobs are bound to session and credential version.

- `POST /api/exports/create`
- `GET` or `HEAD /api/exports/status?id=JOB_ID`
- `GET` or `HEAD /api/exports/download?id=JOB_ID`
- `POST /api/exports/cancel`

POST requests use `Content-Type: application/json`. Create accepts these fields; unknown fields are rejected:

| Field | Meaning |
| --- | --- |
| `camera_id` | Camera to export |
| `start_time`, `end_time` | Ordered RFC 3339 timestamps; end is exclusive |
| `mode` | `copy` (default) or `accurate` |
| `accepted_start`, `accepted_end` | Copy only: the proposed video interval explicitly approved by the user |

A `202` response returns `id`, `state`, `message`, `bytes`, and `expires_at`, with a `Location` header pointing to status. Ready status adds `download_url`. Wait for ready before downloading; a queued or failed job has no ready download.

### Copy range approval

Subsection copy returns `409` JSON with `code: copy_range_approval_required` before job admission or output reservation. It contains `requested_start`, `requested_end`, `export_start`, `export_end`, and `audio_allowance_ms`. Show the proposed video interval and possible AAC packet overhang to the user, even when the video boundaries already match keyframes.

After explicit approval, repeat the original camera, start/end and mode with `accepted_start` and `accepted_end` set to the proposed `export_start` and `export_end`. Do not replace the original requested start/end. The server recomputes the proposal and refuses stale video boundaries; a changed proposal needs fresh approval.

Other common outcomes include `409` with `X-Export-Plan-Status: gap` or `overlap` for missing/ambiguous coverage, `413` for too many clips, `422` for an unsupported mode/profile combination, `429` for exhausted job capacity, and `503` for unavailable export service or metadata. No gap is silently skipped or overlapping source selected automatically.

### Cancel, release and expiry

Cancel takes `{"id":"JOB_ID"}` and confirms with `204`. Cancellation/expiry prevents new download leases. An already-started response can continue until its lease ends. Logout and credential changes block subsequent requests, not necessarily already-started responses. A new sign-in session cannot inherit an older session's jobs.

## Verification and limitations

The prepared-job Go and live-browser checks passed on PR #8's recorded integration head. See [verification evidence](PREPARED_EXPORT_VERIFICATION.md) for the exact commit and scope.

Prepared copies are written to scratch; this is not zero-copy original-file delivery. Subsection copy requires non-reordered video with usable keyframe timestamps. Representative real-camera timing/colour behaviour, full-resolution media, multi-GiB transfers, and heavy-load cancellation remain outside the recorded synthetic qualification. Do not treat an 8 GiB configured limit or a metadata-only large-file test as evidence of an actual 8 GiB transfer.
