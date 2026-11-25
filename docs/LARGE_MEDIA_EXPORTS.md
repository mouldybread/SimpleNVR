# Large-media exports

SimpleNVR prepares exports on the server and serves finished files through authenticated downloads. Page JavaScript does not buffer video or impose the old 64 MiB limit. Server output limits still apply.

## How it works

- Disk-backed jobs have random IDs, session ownership, independent seekable readers, cancellation, expiry, and shutdown joining.
- Jobs that cannot start yet wait in a bounded server queue (state `queued`) and start in order as capacity frees up.
- The browser creates a job, polls its status, then offers a normal same-origin download with HTTP Range support. Control responses are bounded to 8 KiB, and the queue list to 64 KiB.
- The UI queues one job per camera selected for playback, in selection order. The server prepares them in order, as many at a time as `NVR_EXPORT_PARALLEL` allows. A prepared file can be downloaded as soon as it is ready; it does not have to be released before the next one starts.
- The Export queue panel lists the session's jobs (`GET /api/exports/list`) with state, camera, mode, requested and saved range, queue position, estimated and actual size, a Download link for ready jobs, Cancel or Release, and Clear all. The camera, mode and requested range come from the server's own record of each job, so they are still shown after a page reload. The list is read when the panel opens, on Refresh, and after create, cancel, release and clear actions; there is no background polling of the list.
- Cancel/release requires server acknowledgement. If it cannot be confirmed, the row stays visible with a Retry cancellation/release button. A failed or lost creation response is never retried automatically.
- Navigation leaves jobs to expire rather than cancelling a download during browser handoff. Jobs do not survive restart.
- Copy and Accurate are explicit choices. Copy never silently falls back to re-encoding. See [export rendering](EXPORT_RENDERING.md).

## Source recordings

A job, and the direct `GET /api/download`, read their sources from the camera's UTC start-day folders, `/recordings/<camera>/<YYYYMMDD>/<start>_E<end>_<label>.mp4`, and choose them by file name alone. See [recording layout](RECORDING_LAYOUT.md) for the name format and the full selection rules.

- A recording is a source when it starts before the requested end and ends after the requested start; recordings that only touch the range do not count. No file is opened to measure its duration. A published partial recording keeps its stamped start and end, with `_partial` before `.mp4`, so the same rule selects it.
- The end comes from the stamped name. A wrong stamped end gives a wrong range, or a false gap or overlap, rather than an error. Copy extension and keyframe cuts still read the real media, so a wrong name can also move a cut. Recording files must still open and match the planned input when the job pins them, or the job fails.
- Names that are not valid stamped names for their folder are left out without an error. Only the timeline reports how many were skipped, in `X-Timeline-Unparseable-Count`.
- Recordings written directly in a camera folder by older versions are not read.
- Each scan is bounded: at most 400 day folders per camera (a very long range can reach this), 8,192 names read in any one day folder (every name counts, valid or not), and 20,000 matching recordings across the whole scan. A scan that exceeds a bound, meets a matching recording that is empty, not a regular file, unreadable or gone, runs out of its 30 second budget or is canceled fails as a whole and returns nothing partial. Job creation then returns `503` (`504` when the budget ran out) with `X-Recording-Metadata-Status: unavailable`.
- A Copy export that has to cut inside a recording scans a second time over the extended range, and that scan has the same bounds.
- The direct download reports `X-Recording-Duration-Policy: name-end`.

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
| `NVR_EXPORT_MAX_JOBS` | 8 | Maximum retained job records (queued, running, finished and expired) |
| `NVR_EXPORT_MAX_QUEUED` | 8 | Maximum jobs waiting to start |
| `NVR_EXPORT_PARALLEL` | 1 | Concurrent preparations |
| `NVR_EXPORT_TTL` | 30m | How long a finished job is kept; the timer starts when the job completes |
| `NVR_EXPORT_PREPARE_TIMEOUT` | 30m | Preparation deadline, including queue wait |

Byte limits are positive integers in bytes. Counts must be positive; maximum jobs and maximum queued cannot exceed 64 and parallel preparations cannot exceed maximum jobs. Total bytes must be at least the per-job limit. Durations use Go duration syntax such as `30m` or `1h`, must be positive, and cannot exceed 24 hours.

### Queue and reservations

A new job enters the queue. The oldest queued job starts when a parallel slot is free and its reservation fits the remaining byte budget; the queue is never reordered, so a large job is not starved by smaller ones behind it. Queued jobs reserve nothing. When the queue is full or the retained-job limit is reached, create returns `429`.

When a job starts it reserves an estimate of its output size, worked out from the source recordings it reads, instead of the full per-job maximum. Copy reserves the share of each source file that the export uses plus 15% and 8 MiB. Accurate reserves four times that share, because re-encoded output can be much larger than a low-bitrate camera stream. Both are capped at `NVR_EXPORT_MAX_BYTES`, and if the sizes are unknown the full maximum is reserved. The reservation is also the job's output cap: a job that writes more than it reserved fails with a message saying so. When a job becomes ready its reservation shrinks to the actual size, which frees the rest for queued jobs.

A queued or preparing job does not expire. The preparation timeout, which includes time spent queued, bounds it. The retention timer starts when the job completes, whether it became ready, failed or was canceled, and `expires_at` shows the preparation deadline until then.

When the retention period ends, the prepared file is deleted and its reservation is released, but the job stays in the list as `expired` until the session clears it. Expired records count towards `NVR_EXPORT_MAX_JOBS`, so when that limit is reached a new job first evicts the oldest expired records, then is admitted. If no record has expired, create returns `429`.

Expired or released outputs with active readers stay charged until their readers finish. Reservations are accounting limits, not preallocated physical disk space or a bound on every filesystem allocation. Monitor free space and underlying storage health.

## API

All routes require the creating authenticated dashboard session; event tokens cannot use them. Jobs are bound to session and credential version.

- `POST /api/exports/create`
- `GET` or `HEAD /api/exports/status?id=JOB_ID`
- `GET` or `HEAD /api/exports/list`
- `GET` or `HEAD /api/exports/download?id=JOB_ID`
- `POST /api/exports/cancel`
- `POST /api/exports/clear`

POST requests use `Content-Type: application/json`. Create accepts these fields; unknown fields (including the former `accepted_start` and `accepted_end`) are rejected:

| Field | Meaning |
| --- | --- |
| `camera_id` | Camera to export |
| `start_time`, `end_time` | Ordered RFC 3339 timestamps; end is exclusive |
| `mode` | `copy` (default) or `accurate` |

A `202` response carries the job fields below, with a `Location` header pointing to status. Wait for ready before downloading; a queued or failed job has no ready download.

### Job fields

Create, status and list return the same fields for a job.

| Field | Meaning |
| --- | --- |
| `id` | Job ID |
| `state` | `queued`, `preparing`, `ready`, `failed`, `canceled` or `expired` |
| `message` | Short human-readable text; it never contains internal error details |
| `reason` | Stable code explaining a `failed`, `canceled` or `expired` job (see below); absent otherwise |
| `camera_id`, `camera_name` | Camera the job exports and its display name when the job was created |
| `mode` | `copy` or `accurate`, as requested |
| `requested_start`, `requested_end` | The range that was requested, as RFC 3339 UTC |
| `bytes` | Actual output size once the job is ready, otherwise 0 |
| `estimated_bytes` | Size the job reserves while it is `queued` or `preparing` |
| `queue_position` | Place in the queue, starting at 1, while `queued` |
| `saved_start`, `saved_end` | RFC 3339 UTC range the job will save |
| `expires_at` | Preparation deadline until the job completes, then completion time plus `NVR_EXPORT_TTL` |
| `download_url` | Present when the job is `ready` |

| `reason` | Meaning |
| --- | --- |
| `output_limit` | The output reached `NVR_EXPORT_MAX_BYTES` |
| `estimate_exceeded` | The output grew past the size reserved for it; try a shorter range |
| `deadline_exceeded` | The preparation deadline passed, including time spent queued |
| `empty_output` | The render finished without producing any output |
| `render_failed` | The render failed for another reason; details are in the server log, not the response |
| `canceled` | The session canceled the job |
| `cleared` | The session cleared its jobs |
| `expired` | The retention period ended |
| `service_stopping` | The server shut down |

### List and clear

`GET /api/exports/list` returns `{"jobs":[...]}` with the session's jobs, oldest first, including expired ones. Other sessions' jobs are never included. It takes no query parameters.

`POST /api/exports/clear` (body `{}`) cancels and releases every job the session owns, expired ones included, and returns `{"cleared":N}`. Cleared jobs disappear from list and status at once and no longer count towards `NVR_EXPORT_MAX_JOBS`. A job that is still preparing, or a download that is already in progress, finishes winding down in the background and is not interrupted mid-transfer.

### Copy range extension

Subsection copy does not need approval. The server extends the requested range outward to the nearest safe video cut, by at most 30 seconds at each end, and creates the job from that range. The first create request is accepted with `202`, and `saved_start`/`saved_end` report the extended range, which can be wider than `start_time`/`end_time` but never narrower. Clients should show the saved range to the user. AAC packet boundaries can reach slightly outside the video interval, by up to one AAC frame.

If the nearest safe cut is more than 30 seconds beyond the request, or no safe cut exists, create returns `422` and no job or output reservation is made. Choose a wider range or Accurate. Accurate reports the requested range as the saved range.

Other common outcomes include `409` with `X-Export-Plan-Status: gap` or `overlap` for missing/ambiguous coverage, `413` for too many clips, `422` for an unsupported mode/profile combination, `429` for a full queue or exhausted job capacity, and `503` for unavailable export service or metadata. No gap is silently skipped or overlapping source selected automatically.

### Cancel, release and expiry

Cancel takes `{"id":"JOB_ID"}` and confirms with `204`. Canceling a job that already failed, was canceled or expired changes nothing, keeps its recorded reason and still returns `204`. Cancellation/expiry prevents new download leases. An already-started response can continue until its lease ends. Logout and credential changes block subsequent requests, not necessarily already-started responses. A new sign-in session cannot inherit an older session's jobs.

## Verification and limitations

The prepared-job Go and live-browser checks passed on PR #8's recorded integration head. See [verification evidence](PREPARED_EXPORT_VERIFICATION.md) for the exact commit and scope.

Prepared copies are written to scratch; this is not zero-copy original-file delivery. Subsection copy requires non-reordered video with usable keyframe timestamps. Representative real-camera timing/colour behaviour, full-resolution media, multi-GiB transfers, and heavy-load cancellation remain outside the recorded synthetic qualification. Do not treat an 8 GiB configured limit or a metadata-only large-file test as evidence of an actual 8 GiB transfer.

### Long recordings and Copy

A Copy export that has to cut inside a single recording file longer than about two hours will fail. To find safe cut points the server lists that file's video packets, and the listing is capped at 16 MiB (roughly 200,000 packets, about two hours at 25 fps). A larger file is refused as an unsafe cut and the request returns `422`. Choose Accurate, which does not use this listing. Copy of whole, untrimmed recordings is not affected.
