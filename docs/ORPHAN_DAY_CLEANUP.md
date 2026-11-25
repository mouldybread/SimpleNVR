# Camera deletion and orphan day folders (checkpoint 5 slice B)

Camera deletion preserves the legacy direct-root regular .mp4 rule. Inside a
calendar-day folder, every member must instead be a regular valid stamped .mp4
whose UTC start day matches that folder. Unexpected/malformed/misplaced entries,
staging, nested directories, symlinks and special files block the complete plan
before its first unlink. Existing stop/join/persist/lease ordering is unchanged.

Orphan cleanup skips registered and hidden camera directories and examines
exactly one day level, never generic recursion. Before any camera removal it
checks root .tmp evidence, opaque/nonempty temp and buffer, and every day member.
Only valid matching regular stamped day members are eligible. Any unexpected day
member preserves the associated camera. Evidence is rechecked between removals
and before empty-directory removal. Late evidence can leave partial results;
successful file-unlink count/bytes remain reported separately from failures,
interruption and limit flags. Directory removals never count as deleted media.

There are at most 400 day-shaped entries per orphan camera, 8,192 entries per
opened directory, 262,144 observed entries and 4,096 directory-open/read requests
per pass. Repeated evidence inspections consume those same budgets. The existing
flat-tree collector and call order are unchanged when no day entries exist.
A budget or read failure cannot supply a partial listing as deletion evidence.

Orphan path operations retain their existing trusted-ancestor assumption: final
directory opens refuse symlinks and day removal revalidates camera/day identity,
but pathname stat/remove is not an atomic compare-and-unlink and ancestors must
not be adversarially replaced. This is not a claim of full descriptor confinement
for the orphan path. No recursive removal, workflow or CI changes are included.

Focused tests cover camera membership refusal, orphan success/evidence/registered
and hidden preservation, the 400/401 boundary, per-directory/pass budgets,
cancellation, partial counts, read/unlink failure, late evidence and empty-only
rmdir. A staging regression creates 365 day folders with more than 8,192 published
files and asserts exact root/staging counts. Existing flat budget tests remain
unchanged. These tests have not been run by the assistant; the full checkpoint
will be validated on its final head after all slices are ready.
