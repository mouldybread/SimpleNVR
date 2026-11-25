# Checkpoint 6: disk usage readiness and acceptance

Plan v2 checkpoint 6 only; checkpoint 7 documentation (README, large media export
docs, retention docs) and the repository-wide final audit are not included. This
document is a source-level reconciliation, not proof that the changes compile or
pass. No CI is requested before the checkpoint's final acceptance request.

## Implementation history

- `6906cb5c7f4769d40a63cf9ca58cc20797b0750c` (slice A): disk usage engine with
  bounded descriptor-relative reads, closed-day cache, per-call counters, focused
  tests and documentation; the old full-tree walk is removed.
- `e57f4c2d95cf92219f5ea837caad6c7215217e47` (slice B): per-camera inspection
  tracker with a render deadline and bounded concurrency, cache generations and
  pruning, eviction on camera deletion, tracker tests.
- `ea0ce2b6a7ce37ab001c74ae1a2e4476d5bb57f6` (slice C): cached-versus-fresh
  differential, same-tick, 34-camera scale and staging bound regressions, and the
  test inventory. Tests only.
- Slice D (the commit containing this file): documentation reconciliation.

## Requirement reconciliation

| Plan requirement | Implementation and committed evidence | Assessment before CI |
| --- | --- | --- |
| Size each closed day folder once and cache it, validated by directory identity and mtime | `disk_usage.go`: cache entry holds device, inode, mtime, ctime and bytes; lookup requires all to match; closure and timestamp-margin rules decide what may be cached | Implemented, stricter than the plan (adds ctime, closure rule and a 2 s margin) |
| A render does one stat per day folder plus a walk of only changed days | `TestDiskUsage34CamerasYearOfDayFolders`: second render makes 12,410 stats and name reads and no day or staging reads; after one day per camera changes only 34 days are opened | Coverage committed; not run |
| Loose root `.mp4` files summed directly | Root pass sums regular `.mp4` entries; differential and counted-files tests | Coverage committed; not run |
| Cannot drift: nothing cached across a changed directory | Differential test compares cached, fresh and expected totals after ten kinds of change; same-tick change is never cached | Coverage committed; not run |
| Evict on camera deletion | `deleteManagedCameras` calls `evictCameraDiskUsage` after registry removal; every render also prunes unregistered directories; generations stop stale stores; `TestCameraDeletionEvictsDiskUsageCache` and eviction/prune tests | Implemented; not run |
| Bounded and non-recursive | 8,192 entries per directory, 400 day folders, staging at the existing 8,192-entry and depth-4 limits; exact-limit tests; exceeding a bound is incomplete | Coverage committed; not run |
| Request cost and failure | Per-render 10 s deadline and request context; detached two-minute inspections; one inspection per camera, four overall; incomplete or late figures show "Unavailable" | Implemented; timing-sensitive tests are marked in the inventory |
| Existing safety tests and flat fixtures | No existing assertion changed; the only existing caller (`camerasHandler`) changed and its template inputs are unchanged | Must still pass final CI |
| Documentation and status | `docs/DISK_USAGE.md`, `docs/CHECKPOINT6_TEST_INVENTORY.md` and this document; PR description updated separately after this commit | Documentation committed; PR update requires its own approved write |

## Behavior changes and limits

These follow the defaults recorded in the checkpoint 6 plan:

- Buffer and temp staging `.mp4` bytes stay in the total and are read live.
- An incomplete inspection shows "Unavailable" instead of a smaller number, and a
  camera directory that does not exist shows "0 B".
- Only regular `.mp4` files count; symlinked and special names and `.mp4` files in
  other directories no longer do.
- Cached days trust that published media is immutable; there is no periodic
  revalidation.

Known limits: cold-start time at 34 cameras x 365 days is unmeasured, so a first
render after a restart can show "Unavailable" for slower cameras until a refresh;
an inspection joined by a later render may have started slightly earlier; the cache
is in memory only; `calculateDiskUsage` remains as an unused-by-production wrapper;
no test renders the full camera list page through `camerasHandler`, and none of the
existing tests that call it were read; two tracker tests and the cache tests depend
on short real-time pauses and a coarse-timestamp filesystem could make them flaky.
No 1.79-million-file or production-scale qualification is claimed.

## Final acceptance gate

Checkpoint 5 was recorded complete on `fa6f7854574c1378b029c37774fa31711b43c50a`,
run [45676664390](https://github.com/mouldybread/SimpleNVR/actions/runs/45676664390),
all eight required checks successful. That result does not cover checkpoint 6.

After the final documentation or code commit, read PR #13's actual head SHA and use
that immutable value as the acceptance target. Do not use a review baseline, an older
green SHA or a synthetic merge SHA. Updating the PR description does not itself
create a new code head.

Only on explicit user request, run the full checkpoint verification. Require all of
the following to complete successfully against that same final head:

1. safety / Core - build, race, storage & lifecycle
2. safety / Chrome acceptance - isolated run 1
3. safety / Chrome acceptance - isolated run 2
4. safety / Ordered content - bounded synthetic resource measurements
5. safety / Required checks - all must pass
6. prepared_go / Full Go build, race suite and vet
7. prepared_browser / Prepared jobs and native playback - attempt 1
8. prepared_browser / Prepared jobs and native playback - attempt 2

Record the source SHA and run URL. Inspect relevant job logs or state plainly that
they could not be inspected. Pay particular attention to any camera list or camera
registration test that exercises `camerasHandler`, to the race detector on the
tracker tests, and to test duration of the 34-camera fixture. Queued, skipped,
canceled, failed or missing checks do not qualify. A green summary alone must not
be described as individual job-evidence inspection. Any later code or docs commit
requires fresh acceptance on its new head. See [CI procedure](CHECKPOINT_CI.md).

Until then checkpoint 6 stays acceptance-pending, issue #11 stays open, and PR #13
stays draft. No workflow, label, merge, release or issue closure is part of this
slice. Checkpoint 7 documentation and the final audit remain.
