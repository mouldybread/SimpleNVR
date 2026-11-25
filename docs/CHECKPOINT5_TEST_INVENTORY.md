# Checkpoint 5 regression inventory

Slice C baseline: f733b3933516a413aa73d6836c1e85aeaec68f51. Slice C added tests
only; it changed or deleted no production code or existing assertion. A later review
fix changed orphan day evidence rechecks and two orphan test cases; they are listed
under "Changed tests". Coverage below describes committed source, not execution.
CI remains deferred until the checkpoint's final head. No 1.79-million-file or
timing benchmark is claimed.

## New evidence

- retention_scale_evidence_test.go: 34 camera roots with 365 day folders each;
  real expired-file deletion in both passes; doubling skipped contents leaves
  every work counter unchanged. Root day enumeration remains visible: each pass
  observes 365 day names and three inspected member names per camera. The grown
  fixture leaves 24,752 physical files. Exact 400/401 days and 8,192/8,193 valid
  day members are checked through published deletion, camera deletion and orphan
  evidence inspection. Camera admission uses a sentinel first-unlink error, not
  thousands of fake successful deletions; overflow must refuse before any unlink.
- deletion_preservation_evidence_test.go: inspected-day prefix/mtime fallback,
  short-name preservation, strict midnight equality and 24h/cross-day partial
  recordings; parent/root/day replacement and changed-entry refusal during
  complete preflight; day-level cancellation, policy/shutdown guard changes,
  metadata/unlink/sync failures with exact partial count/bytes and sync attempts;
  camera day replacement after first unlink; orphan camera/day replacement before
  removal with replacement bytes preserved.
- retention_publication_serialization_test.go: a publisher requesting shared
  ownership while retention holds it cannot enter publication until retention
  finishes and releases. Retention removes an old empty day; publication recreates
  it under ownership and publishes without loss. Channels control ordering;
  timeouts are deadlock watchdogs only.
- orphan_day_recheck_test.go (review fix): a 365-day, 1,095-file orphan camera
  completes without limit and within loose read/entry bounds that per-file whole
  camera inspection would exceed; late evidence stops removal at the 64-file batch
  boundary and is preserved; root .tmp evidence stops removal at the 16-day recheck.

## Changed tests

- deletion_day_membership_test.go, TestOrphanDayPartialOutcomes, case new-evidence.
  Before: late .tmp evidence stopped the second removal because every file was
  preceded by a whole-camera inspection. After: the small day finishes its two
  validated files and the final empty-only check keeps the day because the late
  evidence is still there. Reason: the per-file whole-camera inspection was removed
  because it made orphan cleanup cost grow with every removed file. The evidence is
  still preserved; batch-boundary behavior is covered by the new tests.
- Same test, case limit. Before: maxReads=7 stopped the pass after one removal
  because each removal re-read the camera. After: maxReads=5 stops at the final
  empty-day read, so both files are removed, the day is retained, LimitReached is
  set and one failure is counted. Reason: the former value depended on the removed
  per-file inspections. The wanted counts for the other cases are unchanged.
- All other cases in that file and the flat orphan tests are unchanged.

## Preserved earlier evidence

- retention_day_selection_test.go (slice A): cutoff at midnight/noon, UTC offsets,
  delayed malformed/misplaced cleanup in skipped days, unchanged generic/delete-all,
  constant skipped-content work, skipped-day budget and read-error accounting.
- deletion_day_membership_test.go and event_staging_day_count_test.go (slice B):
  malformed/wrong-day/overlong camera members blocked before unlink; orphan root/day
  .tmp, temp/buffer, nested content, symlink/FIFO and malformed membership preserve
  evidence; registered/hidden directories; cancellation/read/unlink/late-rmdir
  partial results; shared read/entry limits; exact staging root counts with 8,760
  published files in 365 day folders.
- retention_day_folders_test.go: end-cutoff rules, lease refusal, guard failures,
  late arrivals preserving nonempty folders, camera evidence/bounds and managed
  deletion ownership. Kept intact, not replaced by the new evidence.
- published_deletion_engine_test.go: original flat scope, complete preflight,
  root/parent replacement, cancellation/unlink failures, late arrivals and bounds.
- retention_snapshot_test.go: production admission rejects stale identity/path/
  policy snapshots and canceled work; policy writers remain excluded while registry
  access stays available; fresh snapshots delete expired published media.
- orphan_cleanup_test.go and orphan_cleanup_budget_test.go: original flat-tree
  exact budget/read expectations, private/staging preservation, incomplete listing
  rejection, cancellation and HTTP partial/failure reporting. Unchanged.
- Existing camera deletion persistence, ownership, parent-identity, partial-sync
  and completion tests remain unchanged. They complement, rather than substitute
  for, day-specific coverage. Browser authenticated 206 preflight is untouched.

## Limits and review notes

Orphan cleanup retains the documented trusted-ancestor/pathname-race limitation;
replacement tests demonstrate the explicit rechecks, not atomic descriptor
confinement. Evidence rechecks are batched, so late evidence is seen at a 64-file
batch or 16-day boundary. A camera with legacy root files and day folders still
inspects fully once per root file. Retention's option-2 exception delays
malformed/misplaced-file cleanup in skipped newer days. Root enumeration is still
charged; retained file contents, not root day-name work, are skipped. Known test
gaps: no test asserts that the production retention tick applies the skip policy
through its time-based entry point, and the root-level 8,192/8,193 retention
boundary has no new test. Final acceptance must reconcile this inventory and
inspect relevant CI evidence on the exact final SHA.
