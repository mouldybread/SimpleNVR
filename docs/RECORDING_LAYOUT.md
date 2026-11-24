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
the last instant before `windowEnd`, with an 8,192-entry limit per scanned folder.

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

## Exports

Both direct download selection and prepared-export planning use the stamped
interval scan. `/api/download` reports `X-Recording-Duration-Policy: name-end`.
Recording ends are not inferred by probing media duration. Track/profile checks,
keyframe inspection, rendering and output validation are separate export steps
and remain in place. See [export modes](EXPORT_API_MODES.md).

An incorrect stamped end can produce incorrect timeline coverage or an incorrect
export range; name-based selection does not establish actual media completeness.

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

## Acceptance status

Documentation describes the implementation, not a successful acceptance run.
Checkpoint 3 requires all required jobs to pass on its exact final PR head; older
checkpoint results do not carry forward. CI is explicitly requested only at the
end of the checkpoint. See [checkpoint CI](CHECKPOINT_CI.md).
