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
- `2dae116e728bd76e9f69428c2b75e79b2ba3f102` (slice D): documentation
  reconciliation.
- `69da10c68c772a14843a24c4b712b1d00a10ccb5` (checkpoint 5 follow-up): waiter counter
  on the shared media ownership gate and a stronger publication serialization test.
  This is a production change outside the disk usage code and postdates the
  checkpoint 5 acceptance run, so this checkpoint's run is its first verification.
- The commit containing this file: checkpoint 5 documentation reconciliation.

## Requirement reconciliation

| Plan requirement | Implementation and committed evidence | Assessment before CI |
| --- | --- | --- |
| Size each closed day folder once and cache it, validated by directory identity and mtime | `disk_usage.go`: cache entry holds device, inode, mtime, ctime and bytes; lookup requires all to match; closure and timestamp-margin rules decide what may be cached | Implemented, stricter than the plan (adds ctime, closure rule and a 2 s margin) |
| A render does one stat per day folder plus a walk of only changed days | `TestDiskUsage34CamerasYearOfDayFolders`: second render makes 12,410 stats and name reads and no day or staging reads; after one day per camera changes only 34 days are opened | Coverage committed; not run |
| Loose root `.mp4` files summed directly | Root pass sums regular `.mp4` entries; differential and counted-files tests | Coverage committed; not run |
| Cannot drift: nothing cached across a changed directory | Differential test compares cached, fresh and expected totals after ten kinds of change; same-tick change is never cached | Coverage committed; not run |
| Evict on camera deletion | `deleteManagedCameras` calls `evictCameraDiskUsage` after registry removal; every render also prunes unregistered directories; generations stop stale stores; `TestCameraDeletionEvictsDiskUsageCache` and eviction/prune tests | Implemented; not run |
| Bounded and non-recursive | 8,192 entries per directory, 400 day folders, staging at the existing 8,192-entry and depth-4 limits; exact-limit tests; exceeding a bound shows "Unavailable" | Implemented; not run |
| Request cost | One inspection per camera, four at once, 10 s render deadline, detached two-minute inspections that keep warming the cache | Implemented; not run |
| Existing safety tests and flat fixtures | No existing assertion changed; the only existing caller (`camerasHandler`) changed and its template inputs are unchanged | Must still pass final CI |
| Documentation | `docs/DISK_USAGE.md`, `docs/CHECKPOINT6_TEST_INVENTORY.md`, `docs/CHECKPOINT6_READINESS.md` | Committed |

## Behavior changes and limits

These follow the defaults recorded in the checkpoint 6 plan:

- Buffer and temp staging `.mp4` bytes stay in the total and are read live.
- An incomplete inspection shows "Unavailable" instead of a smaller number; a
  camera directory that does not exist shows "0 B".
- Only regular `.mp4` files count; symlinked and special names and `.mp4` files
  in other directories no longer do.
- Cached days trust that published media is immutable; there is no periodic
  revalidation.

Known limits: cold-start time at 34 cameras x 365 days is unmeasured, so a first
render after a restart can show "Unavailable" for slower cameras until a refresh;
an inspection joined by a later render may have started slightly earlier; the cache
is in memory only; `calculateDiskUsage` remains as an unused-by-production wrapper;
no test renders the full camera list through `camerasHandler`, and none of the
existing tests that call it were read; two tracker tests and the cache tests depend
on short real-time pauses and a coarse-timestamp filesystem could make them flaky.
No 1.79-million-file or production-scale qualification is claimed.

## Acceptance record

Checkpoint 5 was recorded complete on `fa6f7854574c1378b029c37774fa31711b43c50a`,
run [45676664390](https://github.com/mouldybread/SimpleNVR/actions/runs/45676664390),
all eight required checks successful. That result does not cover checkpoint 6 or
later commits.

Checkpoint 6 was recorded complete in issue #11 on
`d5f9665cc964d47daf4769dd4e774bad9d55f406`, run
[45686302269](https://github.com/mouldybread/SimpleNVR/actions/runs/45686302269),
all eight required checks successful. This is check-run verification only:
individual job logs were not inspected, so the tracker race output, the duration of
the 34-camera fixture and which tests exercised `camerasHandler` were not read.
The run first verified `69da10c`. The documentation-only checkpoint 7 did not change
checkpoint 6 code or tests. Issue #11 and PR #13 remain open/draft pending the final
closeout decision; see [checkpoint 9 closeout](CHECKPOINT9_CLOSEOUT.md).
