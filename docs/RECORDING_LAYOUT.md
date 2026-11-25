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
400. Omitting `camera_id` scans all registered cameras. The playback page sends one
`camera_id` for each camera selected for playback and makes no request when none is
selected, so a problem in one camera's files does not affect another camera's view.
It does this from `static/js/timeline-camera-filter.js`, a small script that wraps
the page's timeline request, because `playback.js` itself is unchanged.

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
- `X-Timeline-Unreadable-Count`: valid in-window recordings that the timeline left
  out because they were empty, not regular files or gone before inspection. See
  [failure handling](#failure-handling).
- `X-Timeline-Window-Start`, `X-Timeline-Window-End` and `X-Timeline-Window-Mode`.

Unparseable names are excluded from timeline coverage and returned segment sizes,
and interval selection never probes them. The older `date` query remains an HKT
calendar-day compatibility interface; the browser uses explicit absolute bounds.

The playback page shows a warning when `X-Timeline-Unreadable-Count` is above zero,
because a skipped recording otherwise looks like a gap in the footage.

`/api/recordings/list` is retired and returns 410 Gone. Use the timeline API.

## Scan work and limits

Selecting recordings reads names, not media. A call (one timeline request, one
direct download, or one export planning pass) is bounded in three ways:

| Bound | Value | What it counts | When exceeded |
| --- | --- | --- | --- |
| Day folders per camera | 400 | Folders the window needs. A window of up to 26 hours needs at most four. | The call fails. A very long export range can hit this. |
| Physical entries per opened folder | 8,192 | Every name read in one folder: valid, malformed, non-`.mp4` and out-of-window alike. | The whole call fails. The bound is per folder, not per call. |
| Relevant candidates per call | 20,000 | Valid stamped names, in the folder matching their start day, that overlap the window, across all cameras and folders. Candidates the timeline skips as unreadable still count. | The whole call fails before the 20,001st candidate is inspected. |

Only the folders the window can overlap are looked up, so the work of a call grows
with the requested window and the number of cameras, not with how many days are
retained. A folder that is opened is still enumerated by name, so a folder near its
8,192-entry bound costs that many name reads even when none overlap the window.
Malformed names do not spend the candidate budget, but they do count towards the
physical bound of their folder. Missing, symlinked and non-directory day entries
are skipped and never followed. Names are read in batches of up to 256; a failing
call can already have read the rest of a batch when it stops processing names.
The physical bound limits entries processed, not batch read-ahead.

### Candidate inspection

Each relevant candidate within the budget gets one inspection, relative to the
already-held day-directory descriptor with no-follow semantics (`O_PATH`,
`O_NOFOLLOW`). Candidate 20,001 is refused without inspection. A FIFO or device is
never opened for data, and a symlink is seen as a symlink. The candidate must be a
regular, non-empty file; otherwise it is unusable (see
[failure handling](#failure-handling)). The inspection keeps the file identity that
export pinning later compares (`sameMediaFile`).

### Failure handling

An unusable candidate is a relevant candidate that is empty, is not a regular file,
vanishes after enumeration or cannot be inspected.

Downloads and prepared-export planning are strict. Any unusable candidate, or a
cancellation or timeout, fails the entire scan. No partial interval list is returned,
and previously accumulated healthy-camera intervals are discarded too.

The timeline is tolerant of an unusable candidate. The recording is left out of the
result, counted in `X-Timeline-Unreadable-Count`, and the rest of the timeline is
returned. The skipped recording appears as a gap. It is still inspected once and
still counts towards the 20,000-candidate bound, so skipping cannot hide an oversized
scan. Everything else still fails the whole timeline request with no partial JSON:
cancellation or timeout, any other inspection error, a folder that cannot be read
completely, and the three bounds above.

While the request context is still active, timeline and download scan errors
return 503 with `X-Recording-Metadata-Status: unavailable`, or 504 for a metadata
deadline (normally the 30-second metadata budget). These responses can contain an
explanatory text body, not partial timeline JSON or video. If the request context
has already been canceled or expired, the error writer returns without writing
an error response; cancellation does not promise a 503 or 504.

An initial export-selection failure happens before input pinning. Copy planning
can also rescan an expanded range after acquiring input pins; failure in that
second scan returns no renderer and the held pins are closed on return. Neither
case starts a rendered export from partial scan results.

### Malformed names

A `.mp4` name in a scanned folder that is not a valid stamped name for that folder
is counted in `X-Timeline-Unparseable-Count` and ignored. That covers names without
a stamped end, a reversed or zero span, a span over 24 hours, and a valid name
stored under the wrong day. It does not fail the call. Names without the `.mp4`
suffix are ignored silently. A valid, in-window name that is not a regular file is
not malformed: it is an unusable candidate, which fails downloads and exports and is
skipped and counted, as unreadable and not as unparseable, by the timeline.

### Work counters

Each call carries its own counters: day lookups, days opened, all names returned
by directory reads (including batch read-ahead on failure), relevant candidates
identified after name/day/overlap filtering and before admission, and metadata
inspections actually started. Candidate 20,001 increases RelevantCandidates but
not MetadataCalls. Counters are internal, not HTTP fields; tests use them rather
than timing.

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
validation still use them. Trim-profile and keyframe inspection share a dedicated
two-slot probe admission pool instead of the removed cache's semaphore; this is
not a claim that every validation probe uses that pool.

### Behavior change: corrupt media

The timeline used to probe every file it scanned and refuse corrupt media. It now
trusts the name, so corrupt but non-empty media can appear in the timeline; a name
alone does not guarantee playback or export success. Publication validation
remains in place; selection adds no further media-validity check.

## Retention and deletion

In inspected folders, valid stamped names expire only when their end is strictly
before the UTC cutoff; equality is retained. Unparseable names use a parseable
start prefix first, otherwise mtime for sufficiently long names. Short names are
preserved. Retention conservatively skips day folders strictly after the cutoff's
UTC day. This agreed exception delays cleanup of malformed or misplaced expired
entries inside those skipped folders until the folder ages into the inspected
range. It does not broaden deletion eligibility. Direct-root fallback, generic
selector behavior and delete-all selection are unchanged.

Root day-name enumeration remains necessary. Retention counts all real day
folders, including skipped ones, toward 400; root and opened folders retain the
8,192-entry processing bound. Skipped contents are not inspected for that bound.
Inspected old/boundary folders still use individual-file selection, full candidate
preflight and immediate identity revalidation, never deletion based only on the
folder date. Shared ownership, policy/registry/shutdown guards, directory syncs,
fd-relative no-follow operations and empty-only rmdir remain. See
[retention selection](RETENTION_SELECTION.md).

Camera deletion keeps the direct-root regular .mp4 compatibility rule. Inside a
day folder, every member must be a regular valid stamped .mp4 with a UTC start day
matching the folder. Unexpected/malformed/misplaced entries, symlinks, special
files, nested content and nonempty staging/buffer block deletion before unlink.
Stop/join/persist/lease sequencing is unchanged.

Orphan cleanup skips registered and hidden camera directories and descends only
one day level. Root/day .tmp evidence, nonempty or opaque temp/buffer, and invalid
day members preserve associated media. Every repeated inspection spends the same
pass budget: 8,192 entries per directory, 400 day-shaped entries per camera,
262,144 observed entries and 4,096 directory-read/open requests per pass. Existing
flat-tree budget behavior is retained. Staging scanning counts root day names but
does not descend into published day contents; root .tmp and actual temp contents
still count. See [camera/orphan cleanup](ORPHAN_DAY_CLEANUP.md).

Deletion is not transactional: failure can follow successful unlinks. Published
engine count/bytes report successful file unlinks; later unlink, guard or sync
failure marks the pass incomplete without restoring files. Orphan cleanup also
reports failures, interruption, budget exhaustion and skipped evidence directories;
directory removal is not counted as deleted media. Empty-only rmdir preserves
late entries. Inspect partial outcomes before retrying. The orphan path retains
its trusted-ancestor/pathname-race limitation and does not claim full descriptor
confinement.

## Test evidence and known limits

Committed scanner regressions cover more than 20,000 physical files, 34 cameras,
exact candidate/folder limits, boundaries, offsets, malformed/wrong-day names,
metadata failures and cancellation. Checkpoint 5 adds 34-camera retention work
comparisons, exact deletion bounds and day-level failure/preservation evidence.
See the [test inventory](CHECKPOINT5_TEST_INVENTORY.md) and
[readiness reconciliation](CHECKPOINT5_READINESS.md) for source-level mappings;
these are not claims of successful execution on the current head.

Limits that remain:

- The design target is 34 cameras and 365 days of 10-minute segments. Tests use
  tens of thousands of directory entries, not the roughly 1.79 million files of
  that target, and no run at that scale is claimed. 256 cameras were not measured.
- The name end is wall-clock. An event's end is its planned end, and a recovered
  file's end comes from its modification time, so those intervals are approximate.
- Playback continues to the next clip only when its start equals the previous
  clip's end exactly. A wall-clock end will often differ slightly, so continuation
  across clips is not guaranteed. The playback script is unchanged.
- The timeline skips an unusable recording and the playback page reports it only as
  a transient warning, so the recording looks like a gap. Downloads and exports
  still fail if they need that recording.
- Opened folders are enumerated by name; repeated orphan evidence checks may
  exhaust whole-pass budgets and leave partial cleanup.
- Old flat recordings are not read by timeline/export selection. Explicit deletion
  retains its documented direct-root compatibility behavior.

## Acceptance status

Issue #11 records checkpoints 1 to 6 complete, each with all eight required checks
successful on its own head; those results do not cover later commits. Checkpoint 7
(documentation) was recorded complete without a CI run, by owner decision. The
camera filter, the timeline's skip-and-report behavior and the unreadable-recording
warning were committed afterwards and have no CI result, so no passing result is
claimed for them. Any later code or docs commit requires a fresh run. CI is
requested explicitly at the end, not per slice. See [checkpoint CI](CHECKPOINT_CI.md).
