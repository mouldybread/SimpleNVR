# Camera disk usage (checkpoint 6)

The camera list shows one disk usage figure per camera. This describes the
checkpoint 6 implementation. Checkpoint 6 was accepted on
`d5f9665cc964d47daf4769dd4e774bad9d55f406`, run 45686302269, with all eight
required checks successful (check-run verification only; job logs were not
inspected). See [readiness](CHECKPOINT6_READINESS.md) and the
[test inventory](CHECKPOINT6_TEST_INVENTORY.md).

## What is counted

The sum of regular `.mp4` files in:

- the camera root (legacy loose files),
- every valid `YYYYMMDD` day folder, and
- `buffer/` and `temp/` staging, including nested event directories.

Not counted: files that are not regular (symlinks, FIFOs), names without the
`.mp4` extension (for example `.mp4.tmp`), `.mp4` files in any other directory,
and anything behind a symlinked day folder. Symlinks are never followed.
Previously every `.mp4` anywhere under the camera directory was counted, including
symlinks by their own size; these are deliberate behavior changes.

## Work and bounds

An inspection lists the camera root once, stats each day folder entry, and opens
only the day folders that cannot be served from the cache. Bounds: 8,192 entries per
directory, 400 day folders per camera, and the existing event staging limits of
8,192 entries and depth 4 for `buffer/` and `temp/`. Exceeding any bound makes the
inspection incomplete. Every read, stat and cache decision is counted per call;
counters are internal and tests assert them instead of timing.

## Closed-day cache

A day's total is cached only when all of these hold at the time of the scan:

- the UTC day is over by more than the maximum stamped span (24 hours) plus a
  one-minute margin, so it can no longer receive published files;
- the directory did not change while it was being read;
- its mtime and ctime are older than the scan start by more than two seconds,
  because filesystem timestamps are coarse and a change in the same tick as the
  scan could otherwise look unchanged.

A cached day is used only while its device, inode, mtime and ctime are unchanged.
New files, removals by retention, recovery publishing into an old day and a
replaced directory all change them, so the day is read again. File contents are
not rechecked: published media is immutable (no-replace publication), which is
the stated assumption, and there is no periodic revalidation. Days that vanish are
dropped from the cache.

## Rendering, concurrency and eviction

The camera list starts one inspection per camera and waits for each, in order, up
to a 10 second render deadline and no longer than the request's own context. An
inspection that is not finished shows "Unavailable" for that render but keeps
running in the background with its own two minute timeout, so the next render finds
the cache warm. Giving up on a render never cancels the shared inspection.

At most one inspection runs per camera directory at a time: a second render joins
the one in flight and receives its result, which may have started slightly before
the second request. At most four cameras are inspected at once across all renders.
No lock is held while reading the filesystem, and `nvr.mu` is not held during
inspection.

The cache is keyed by camera record directory. It is dropped for a camera when the
camera is deleted successfully, and every render drops cached directories that are
no longer registered, which also covers a changed record directory. Each eviction
advances the directory's generation, so an inspection that began before the
eviction cannot store into the state that follows it, and a camera later registered
at the same path starts fresh.

## Cold start

The cache lives in memory. After a restart, or the first time a camera is listed,
every day folder must be read once; for a camera with a year of 10-minute segments
that is about 52,560 files, and for 34 cameras about 1.79 million. The time this
takes on real storage has not been measured. Cameras that do not finish within the
render deadline show "Unavailable" and are completed in the background; a later
refresh shows their figures and then costs one stat per day folder.

## Failure

If any directory cannot be read completely, a bound is exceeded, the inspection
times out or the render deadline passes, the figure is shown as "Unavailable"
instead of a smaller number. A camera directory that does not exist yet shows
"0 B".

## Code paths

The camera list uses the tracker (`recordingDiskUsage.render`). The older
`calculateDiskUsage` function remains as a direct single-call wrapper over the same
engine and the same cache; the camera list no longer calls it.

## Limits

No run at 1.79 million files is claimed, and cold-start time is unmeasured. The
tests are source-level: focused cases, a cached-versus-fresh differential, counter
assertions at 34 cameras x 365 day folders and the staging bounds. They passed in
the checkpoint's CI run, which was read as check results only: the tracker race
output and the duration of the 34-camera fixture were not read from job logs. The
assistant has not compiled or run this code itself.
