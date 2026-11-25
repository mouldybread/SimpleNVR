# Checkpoint 7 documentation audit (slice A)

Baseline: `d5f9665cc964d47daf4769dd4e774bad9d55f406`. This note records what was read, what was
verified against source, and exactly which edits slices B and C will make. It changes no other file
and claims nothing about execution. Plan v2 step 7 is the scope: README, export documents and the
retention/storage documents describe the layout, the inclusion rule, the changed headers, the limits,
and that old flat files are not read.

## Documents read and verdict

| Document | Finding | Planned action |
| --- | --- | --- |
| `README.md` | No mention of the day-folder layout, stamped-name selection, retention or the disk usage figure. The upgrade section says to keep existing recordings but not that flat files are not read by the timeline and exports. The camera removal sentence does not mention that removal can be refused. | Slice B: add sections and an upgrade note; keep every existing statement |
| `docs/LARGE_MEDIA_EXPORTS.md` | Jobs, limits, API fields and failure codes are accurate for what they cover, but nothing says sources are selected by stamped names without probing, that a wrong stamped end gives a wrong range without an error, or what happens when a scan limit or metadata failure stops planning. | Slice C: add a source selection section |
| `docs/EXPORT_API_MODES.md` | Already current: it has a recording selection section with the name-end policy, the `X-Recording-Duration-Policy: name-end` header, day-segment `file_path` values and the retired list endpoint. | No edit |
| `docs/EXPORT_RENDERING.md` | Contains no layout or selection statements. Its mention of duration probing refers to validating prepared output, which is unchanged. | No edit |
| `docs/STORAGE_DEPLOYMENT_BOUNDARIES.md` | A dated candidate-batch note about shutdown and mount loss. It does not mention the layout and carries status text for an earlier base. | Leave content untouched (decision 3) |
| `docs/RECORDING_LAYOUT.md`, `RECORDING_SERVING.md`, `RETENTION_SELECTION.md`, `ORPHAN_DAY_CLEANUP.md`, `DISK_USAGE.md` | Written during checkpoints 3 to 6 and reconciled in their readiness documents. | Link to them; no edit |
| `docs/audit/*` (about 85 dated notes) | `docs/audit/README.md` already says the remaining notes are historical records to be read through the final closeout ledger. Three read notes contain statements this PR superseded: `TIMELINE_API.md` (probed video durations, an 8192-entry scan limit and a 2048-result duration cache), `RETENTION_DELETION_ENGINE.md` (selection limited to top-level files) and `ORPHAN_CLEANUP_BOUNDARIES.md` (unbounded enumeration, flat directories). Other notes about deletion, orphan cleanup, storage status and playback continuation were not read and may overlap. | Slice C: add a short superseded-statements section to `docs/audit/README.md` that points at the current documents; do not edit the dated records |
| `docs/PREPARED_EXPORT_VERIFICATION.md`, `AUTHENTICATION.md`, `LIVE_PROXY_BOUNDARIES.md`, `browser-first-run-setup.md` | Not read; by subject they concern prepared-export evidence, sign-in, the live proxy and first-run setup, none of which this PR changed. | No edit; slice C re-checks `PREPARED_EXPORT_VERIFICATION.md` only if a link needs it |

## Tests and workflows that could depend on documentation text

- `deployment_storage_boundaries_test.go` was read in full: it exercises mount, free-space and publication behavior and reads no documentation file.
- Searches of the code index for the document names (`STORAGE_DEPLOYMENT_BOUNDARIES`, `LARGE_MEDIA_EXPORTS`) and for `README.md` and `docs/` in Go files found references only inside other Markdown files, not in tests. The index covers the main branch and was unreliable earlier in this work, so this is supporting evidence, not proof.
- `.github/workflows/checkpoint-ci.yml` and `safety-ci.yml` were read in full: they build, vet, run the race suite and browser checks, and assert only that `go.mod` and `go.sum` are unchanged. They contain no Markdown or link check.
- Not read: the Python, shell and CommonJS helpers in `tests/`, the other three workflows and the Dockerfile. No documentation dependence is known; this is a residual risk that the checkpoint's CI run will settle.

## Facts verified against source (branch head `d5f9665`)

- Timeline: `GET` or `HEAD /api/timeline/recordings`, one `start_time` and `end_time` in RFC 3339, end exclusive, window positive and at most 26 hours; the legacy `date` mode is HKT; repeated `camera_id` values are deduplicated, an unknown one is 404 and an empty one 400 (`timeline_window.go`).
- Timeline response headers: `X-Timeline-Window-Start`, `X-Timeline-Window-End`, `X-Timeline-Window-Mode` (`absolute` or `legacy-hkt`), `X-Timeline-Duration-Policy: name-end` and `X-Timeline-Unparseable-Count`; each segment's `file_path` is `/recordings/<camera>/<day>/<file>` (`timeline_window.go`).
- Direct download: `/api/download` takes exactly one `camera_id`, `start_time` and `end_time`; it returns 404 when no recording matches and the header `X-Recording-Duration-Policy: name-end` (`playback.go`).
- Retired endpoint: `/api/recordings/list` returns 410 Gone with a pointer to the timeline endpoint (`playback.go`).
- Scan or metadata failure: 503 with `X-Recording-Metadata-Status: unavailable`, or 504 for a deadline, never a partial result; nothing is written when the request context is already done (`recording_intervals.go`).
- Retention: the cleanup routine ticks every 5 minutes (`cleanup.go`, checked on main); a camera's retention is 1 to 365 days and the camera list shows 7 when it is unset (`handlers.go`).
- Camera removal: a refused removal returns 409 and leaves the camera registered and stopped (`camera_deletion.go`).
- Legacy flat files: the retention and deletion engine, camera deletion and the disk usage figure still handle direct-root `.mp4` files, while the timeline and export scan reads only day folders (`published_deletion_engine.go`, `camera_deletion.go`, `disk_usage.go`, `recording_stamp_scan.go`). The exact retention rule for legacy names is the existing start-prefix then mtime fallback documented in `RETENTION_SELECTION.md`.
- Observation, not a documentation edit: `maxRecordingScanEntries` in `recording_intervals.go` appears unused now that the scan uses its own folder bound. It is dead code left for a separate cleanup decision.

## Planned edits

### Slice B: `README.md`

- New section on where recordings are stored: `/recordings/<camera>/<YYYYMMDD>/<start>_E<end>_<label>.mp4`, the day is the UTC start date, published partial files, and a link to `docs/RECORDING_LAYOUT.md`.
- How footage is selected: stamped start and end names, strict overlap, no duration probing, so a wrong stamped end shows up as a wrong range, with unparseable names excluded and counted.
- Retention: what expires (a recording whose stamped end is before the cutoff), the 5-minute cleanup, the 1 to 365 day setting, the newer-day exception, and a link to `docs/RETENTION_SELECTION.md`.
- Disk usage: what is counted, when it shows "Unavailable" or "0 B", the first-render delay after a restart, and a link to `docs/DISK_USAGE.md`.
- Upgrade note for installs that have flat recordings from before the day-folder layout: the files stay on disk, are not shown by the timeline or exports, are not migrated, and are still counted, expired by retention and removed with their camera.
- Camera removal sentence: add that removal is refused, with the camera left stopped, when a day folder holds anything other than valid stamped recordings.
- Preserve every existing instruction, setting and statement unchanged.

### Slice C: other documents

- `docs/LARGE_MEDIA_EXPORTS.md`: a source selection section covering the points above, the 503 and 504 responses with `X-Recording-Metadata-Status`, the 20,000-candidate and 8,192-entry bounds, the exact-equality limit on playback continuing across clips, and a link to `docs/RECORDING_LAYOUT.md`.
- `docs/audit/README.md`: a superseded-statements section naming `TIMELINE_API.md`, `RETENTION_DELETION_ENGINE.md` and `ORPHAN_CLEANUP_BOUNDARIES.md` and pointing to `RECORDING_LAYOUT.md`, `RETENTION_SELECTION.md`, `ORPHAN_DAY_CLEANUP.md` and `DISK_USAGE.md`.
- Check that every relative link in a touched document resolves, by reading the target path listing.
- Not edited: `EXPORT_API_MODES.md`, `EXPORT_RENDERING.md`, `STORAGE_DEPLOYMENT_BOUNDARIES.md` and the dated audit records themselves.

## Open decisions

The defaults recorded in the checkpoint 7 plan apply: flat recordings documented as not read or migrated, README sections that link instead of a new combined document, the historic storage boundaries document left untouched, no `camera_id` frontend filter (the API's repeated `camera_id` is documented), and no release notes or version numbers.
