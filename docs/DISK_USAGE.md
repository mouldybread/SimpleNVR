# Camera disk usage (checkpoint 6)

The camera list shows one disk usage figure per camera. This describes the engine
introduced in slice A. It is committed source, not an execution claim, and the
final documentation is reconciled in slice D.

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

A render lists the camera root once, stats each day folder entry, and opens only
the day folders that cannot be served from the cache. Bounds: 8,192 entries per
directory, 400 day folders per camera, and the existing event staging entry and
depth limits for `buffer/` and `temp/`. Every read, stat and cache decision is
counted per call; counters are internal and tests assert them instead of timing.

## Closed-day cache

A day's total is cached only when all of these hold at the time of the scan:

- the UTC day is over by more than the maximum stamped span (24 hours) plus a
  one-minute margin, so it can no longer receive published files;
- the directory did not change while it was being read;
- its mtime and ctime are older than the scan start by more than two seconds,
  because filesystem timestamps are coarse and a change in the same tick as the
  scan could otherwise look unchanged.

A cached day is used only while its device, inode, mtime and ctime are unchanged.
New files, removals by retention and a replaced directory all change them, so the
day is read again. File contents are not rechecked: published media is
immutable (no-replace publication), which is the stated assumption. Days that
vanish are dropped from the cache. The cache is keyed by camera record directory;
eviction on camera deletion, the request deadline and concurrency control are added
in slice B.

## Failure

If any directory cannot be read completely, a bound is exceeded, or the request is
cancelled, the figure is shown as "Unavailable" instead of a smaller number. A
camera directory that does not exist yet shows "0 B".

## Limits

No run at 1.79 million files is claimed. Slice A has focused tests only; the scale,
differential and concurrency evidence is slice C. Nothing here has been compiled
or run by the assistant.
