# Recording layout and window selection

## Published files

Published recordings use `/recordings/<camera>/<YYYYMMDD>/<start>_E<end>_<label>.mp4`.
The timestamps use `20060102_150405` in UTC. The day directory is the UTC start date,
not the browser's local date or the recording's end date. Published partial files
have the `_partial` suffix before `.mp4`.

For example, a recording crossing UTC midnight can be stored as:

```
/recordings/id_front/20261110/20261110_235958_E20261111_000008_continuous.mp4
```

Publication truncates the start to seconds and rounds the end up to seconds so the
name does not understate the interval. A stamped interval must be positive and no
longer than 24 hours. This development layout does not provide a legacy-file
migration mechanism.

## Timeline API and browser paths

Use `GET /api/timeline/recordings` with one `start_time` and one `end_time`, encoded
as RFC3339 timestamps. The end is exclusive; the window must be positive and no
longer than 26 hours. Optional repeated `camera_id` parameters select cameras.
Duplicates are deduplicated, an unknown ID returns 404, and an empty ID returns
400. Omitting `camera_id` scans all registered cameras.

A recording overlaps the window exactly when `start < windowEnd` and
`end > windowStart`. Touching edges do not overlap. The scanner opens only UTC
start-day folders from the day of `(windowStart - 24h)` through the day containing
the last instant before `windowEnd`. Instants are compared as instants, so a window
written in any UTC offset selects the same recordings. See
[scan work and limits](#scan-work-and-limits) for the bounds.

The response remains a JSON array. Each segment's `file_path` contains the camera,
day and filename. Clients must use this path directly; do not reconstruct it from
a basename or from a local calendar date. The playback module already assigns
`segment.file_path` to the video source. The recordings module handles bulk
deletion and refreshes the timeline; it is not a separate file-list browser.

Response headers include:

- `X-Timeline-Duration-Policy: name-end`.
- `X-Timeline-Unparseable-Count`: invalid or misplaced stamped `.mp4` names found
  in the scanned day folders, not a whole-storage audit.
- `X-Timeline-Window-Start`, `X-Timeline-Window-End` and `X-Timeline-Window-Mode`.

Unparseable names are excluded from timeline coverage and returned segment sizes,
and interval selection never probes them. The older `date` query remains an HKT
calendar-day compatibility interface; the browser uses explicit absolute bounds.

`/api/recordings/list` is retired and returns 410 Gone. Use the timeline API.

## Scan work and limits

Selecting recordings reads names, not media. A call (one timeline request, one
direct download, or one export planning pass) is bounded in three ways:

| Bound | Value | What it counts | When exceeded |
| --- | --- | --- | --- |
| Day folders per camera | 400 | Folders the window needs. A window of up to 26 hours needs at most four. | The call fails. A very long export range can hit this. |
| Physical entries per opened folder | 8,192 | Every name read in one folder: valid, malformed, non-`.mp4` and out-of-window alike. | The whole call fails. The bound is per folder, not per call. |
| Relevant candidates per call | 20,000 | Valid stamped names, in the folder matching their start day, that overlap the window, across all cameras and folders. | The whole call fails before the 20,001st candidate is inspected. |

Only the folders the window can overlap are looked up, so the work of a call grows
with the requested window and the number of cameras, not with how many days are
retained. A folder that is opened is still enumerated by name, so a folder near its
8,192-entry bound costs that many name reads even when none overlap the window.
Malformed names do not spend the candidate budget, but they do count towards the
physical bound of their folder. Missing, symlinked and non-directory day entries
are skipped and never followed.

### Candidate inspection

Each relevant candidate gets exactly one inspection, relative to the already-held
day-directory descriptor with no-follow semantics (`O_PATH`, `O_NOFOLLOW`). A FIFO
or device is never opened for data, and a symlink is seen as a symlink. The
candidate must be a regular, non-empty file. The inspection keeps the file identity
that export pinning later compares (`sameMediaFile`).

### Failure is all or nothing

If any relevant candidate is empty, is not a regular file, vanishes after
enumeration, cannot be inspected, or the call is canceled or times out, the entire
call fails. No partial interval list, timeline body or export is produced, and a
healthy camera's results are discarded with a failing camera's. The timeline and
download endpoints answer 503 with `X-Recording-Metadata-Status: unavailable`, or
504 when the 30-second metadata budget ends. Export planning returns an error and
no renderer, and no input is pinned.

### Malformed names

A `.mp4` name in a scanned folder that is not a valid stamped name for that folder
is counted in `X-Timeline-Unparseable-Count` and ignored. That covers names without
a stamped end, a reversed or zero span, a span over 24 hours, and a valid name
stored under the wrong day. It does not fail the call. Names without the `.mp4`
suffix are ignored silently. A valid, in-window name that is not a regular file is
not malformed: it is a failed candidate and fails the call.

### Work counters

Each call carries its own counters: day lookups, days opened, entries read and
candidate inspections. They are internal to the scanner, are not exposed in HTTP
responses, and exist so tests can assert bounded work without timing.

## Exports

Both direct download selection and prepared-export planning use the stamped
interval scan and its limits. `/api/download` reports
`X-Recording-Duration-Policy: name-end`. Recording ends are not inferred by probing
media duration. Track/profile checks, keyframe inspection, rendering and output
validation are separate export steps and remain in place. See
[export modes](EXPORT_API_MODES.md).

An incorrect stamped end can produce incorrect timeline coverage or an incorrect
export range; name-based selection does not establish actual media completeness.

### Removed duration infrastructure

The recording duration cache, its constructor, its global instance and every cache
parameter on interval collection, timeline serving and download serving are gone.
No selection step runs `ffprobe`.

The duration-decoding and bounded-output helpers (`probeVideoDuration`,
`decodeVideoDuration`, `limitedProbeOutput`) are deliberately kept in
`export_probe.go`, because export profile and keyframe inspection and output
validation still use them. Those inspections now share a dedicated two-slot probe
admission pool instead of the removed cache's semaphore.

### Behavior change: corrupt media

The timeline used to probe every file it scanned and refuse corrupt media. It now
trusts the name, so a corrupt but non-empty file that is already on disk appears in
the timeline and fails later, at playback or export profile inspection. Media is
validated once when a recording is published; selection adds no further check.

## Retention and deletion

For a valid stamped name, automatic retention expires the file only when its end
is strictly before the cutoff; equality is retained. Unparseable names retain the
existing fallback: a parseable start prefix is used first, otherwise mtime is used
for names long enough for that fallback. Short names remain preserved by retention.

Retention and camera deletion inspect at most 400 day folders per camera and
8,192 entries per folder. Production mutation admission retains the shared
`recordingMediaOwnership` lease. Day directories are opened no-follow; deletion
uses pinned descriptors, fd-relative checks and individual unlinks, not recursive
removal. Camera deletion refuses unexpected entries and nonempty staging or buffer
directories. A day folder is removed only with an empty-directory rmdir, so late
entries cannot be swept recursively. A failed pass can follow partial deletion;
review the reported outcome before retrying.

## Test evidence and known limits

Scanner regressions assert per-call counters, not timing: more than 20,000 physical
files with a narrow window; 34 cameras grown from three retained days to a full
year with identical counters and results; exactly 20,000 and 20,001 candidates
across cameras; two folders of exactly 8,192 entries and one more; window-edge,
24-hour-span, midnight, misfiled-name and equivalent-offset cases; and fail-closed
cases for empty, directory, symlink and FIFO candidates, vanished files, metadata
errors and cancellation, through the scanner, timeline, download and export
planning.

Limits that remain:

- The design target is 34 cameras and 365 days of 10-minute segments. Tests use
  tens of thousands of directory entries, not the roughly 1.79 million files of
  that target, and no run at that scale is claimed. 256 cameras were not measured.
- The name end is wall-clock. An event's end is its planned end, and a recovered
  file's end comes from its modification time, so those intervals are approximate.
- Playback continues to the next clip only when its start equals the previous
  clip's end exactly. A wall-clock end will often differ slightly, so continuation
  across clips is not guaranteed. The playback script is unchanged.
- Folders that are opened are enumerated by name; the per-folder bound limits that
  cost but does not remove it.
- Old flat recordings are not read.

## Acceptance status

Documentation describes the implementation, not a successful acceptance run.
Checkpoint 3 (serving) is recorded complete in the issue on its own exact head.
Checkpoint 4 (scan) may be recorded complete only when all eight required checks
succeed on the exact final PR head; older results do not carry forward and any
later commit needs a new run. CI is explicitly requested only at the end of the
checkpoint. See [checkpoint CI](CHECKPOINT_CI.md).
