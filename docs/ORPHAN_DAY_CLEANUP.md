# Camera deletion and orphan day folders (checkpoint 5 slice B)

Camera deletion preserves the legacy direct-root regular .mp4 rule. Inside a
calendar-day folder, every member must instead be a regular valid stamped .mp4
whose UTC start day matches that folder. Unexpected/malformed/misplaced entries,
staging, nested directories, symlinks and special files block the complete plan
before its first unlink. Existing stop/join/persist/lease ordering is unchanged.

Orphan cleanup skips registered and hidden camera directories and examines
exactly one day level, never generic recursion. Only valid matching regular
stamped day members are eligible. Any unexpected day member preserves the
associated camera.

## Evidence model

Evidence is inspected completely, then rechecked at bounded granularity. A whole
camera inspection per removed file is not used, because its cost grows with the
number of retained files and would exhaust the pass budget on a year of days.

1. Before any day is processed, one full inspection covers root .tmp entries,
   opaque or nonempty temp/buffer, and every member of every day folder.
2. For each day, the whole listing is validated and every member's identity is
   remembered before the first removal. Immediately before each unlink the member
   is stat'ed again and must be the same regular file, and the camera root and day
   directory must still be the same directories.
3. The day is re-read every 64 removals. Any name that was not in the validated
   listing preserves the rest of the camera. A final re-read must find the day
   empty before the empty-only directory removal.
4. Every 16 days the camera root is re-read for .tmp entries. If a temp or buffer
   entry has appeared, a full inspection decides.
5. A final full inspection precedes removal of the camera directory itself.

The tradeoff is detection granularity. Evidence that arrives inside a day is seen
at the next 64-file batch boundary, so up to 63 already validated stamped files in
that batch can still be removed. The evidence itself is never selected or deleted,
and a nonempty day folder is never removed. Evidence that arrives in the camera
root is seen within 16 days. Timestamps are deliberately not used to detect new
entries, because coarse filesystem timestamps can match a change made immediately
after our own removal.

## Budgets and partial results

There are at most 400 day-shaped entries per orphan camera, 8,192 entries per
opened directory, 262,144 observed entries and 4,096 directory-open/read requests
per pass. Every inspection and recheck consumes those same budgets. A camera with
365 days and three files per day completes well inside them. The existing
flat-tree collector and call order are unchanged when no day folders exist. A
budget or read failure cannot supply a partial listing as deletion evidence.

Successful file-unlink count/bytes are reported separately from failures,
interruption and limit flags. Directory removals never count as deleted media.
Budget exhaustion, cancellation or late evidence can leave a partially cleaned
camera.

A camera that has both legacy direct-root .mp4 files and day folders still runs
the full inspection once per root file, which is the existing flat behavior and
grows with the retained day contents. Legacy root files are expected to be few.

Orphan path operations retain their existing trusted-ancestor assumption: final
directory opens refuse symlinks and day removal revalidates camera/day identity,
but pathname stat/remove is not an atomic compare-and-unlink and ancestors must
not be adversarially replaced. This is not a claim of full descriptor confinement
for the orphan path. No recursive removal, workflow or CI changes are included.

Focused tests cover camera membership refusal, orphan success/evidence/registered
and hidden preservation, the 400/401 boundary, per-directory/pass budgets,
cancellation, partial counts, read/unlink failure, late evidence, empty-only rmdir,
the 64-file batch recheck, the 16-day root recheck and a 365-day orphan within the
pass budget. A staging regression creates 365 day folders with more than 8,192
published files and asserts exact root/staging counts. Existing flat budget tests
remain unchanged. These tests have not been run by the assistant; the full
checkpoint will be validated on its final head after all work is ready.

## Acceptance status

Plan v2 checkpoint 5 (retention and deletion) was accepted on
`fa6f7854574c1378b029c37774fa31711b43c50a`, run 45676664390, all eight required
checks successful. The checkpoint 5 tests and behavior above were included on
that head. This is check-run verification; individual job logs were not inspected,
and the assistant did not run the tests locally. The later documentation-only
checkpoint 7 did not change these tests or behavior. Checkpoint 8 changes were
accepted on `2e1b33bd4f6f23ca54339a423c0109517089cf1b`, run 45696730551, all eight
required checks successful; it also does not change orphan cleanup. Later dead-code
and frontend-test commits were not covered by that run. See the
[checkpoint 9 closeout record](CHECKPOINT9_CLOSEOUT.md) for current final-head status.
