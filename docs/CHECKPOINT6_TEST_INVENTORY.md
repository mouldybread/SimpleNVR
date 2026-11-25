# Checkpoint 6 regression inventory

Baseline: `fa6f7854574c1378b029c37774fa31711b43c50a`. Coverage below describes
committed source, not execution. CI is requested only when the checkpoint is
complete, and no 1.79-million-file or timing benchmark is claimed.

## Evidence by slice

- disk_usage_test.go (slice A): what is counted and ignored (27 bytes in the mixed
  fixture), cache hit, miss, new file, removal and vanished day, racy and open days
  are never cached, 400/401 day folders and 8,192/8,193 day entries, read failure,
  cancellation, missing directory and the compatibility wrapper.
- disk_usage_tracker_test.go (slice B): one shared inspection per camera, waiters with
  independent deadlines that never cancel the shared inspection, concurrency bound,
  eviction, pruning of unregistered directories, stale-generation stores, and camera
  deletion evicting the production cache through deleteManagedCameras.
- disk_usage_scale_test.go (slice C):
  - cached results equal a fresh uncached walk and an independently computed total
    after: a new file in an old day, a retention-style file removal, removal of a whole
    day, a day replaced by a different directory, a day replaced by a symlink, a
    partial file, a loose root file, staging file additions and removals, and an empty
    day folder;
  - a change in the same timestamp tick as a scan is never served from the cache;
  - 34 cameras x 365 day folders (12,410 folders): the second render makes exactly one
    stat and one name read per day folder and no day, staging or file reads, and after
    one day per camera changes only those 34 days are read;
  - buffer/temp staging exactly at and one past the entry and depth limits.

## Changed tests

None. Slices A to C add tests and change no existing assertion. The only existing
production caller of the usage figure, camerasHandler, changed in slice B; the
template and the strings passed to it are unchanged. No test that calls
camerasHandler was read, so any test that depends on the old synchronous behavior
will only be found by running the suite.

## Limits and review notes

- Tests pause 50 ms between a recorded state and a change so the change falls in a
  later timestamp tick. A filesystem with coarser timestamps could make those steps
  flaky.
- Two tracker tests assert absence of an event for 100 ms and expiry of a 20 ms
  deadline. Neither asserts a performance figure.
- The unreadable-directory case is covered with an injected read failure, not by
  changing permissions, so it does not depend on the user running the tests.
- The staging depth test builds a chain of eventStagingScanDepth directories and
  assumes that constant is a small integer.
- Cached days trust that published media is immutable; there is no periodic
  revalidation, which was the default for decision 4 in the plan.
- Not covered: a production-scale run, a real camera list page render, and the race
  detector result, which only the checkpoint's CI run can show.
