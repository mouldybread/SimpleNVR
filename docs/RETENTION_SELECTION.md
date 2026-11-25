# Retention-aware day selection (checkpoint 5 slice A)

Retention uses a single UTC cutoff per pass. Valid stamped recordings expire only
when their stamped end is strictly before that cutoff; equality is retained.
The name's maximum 24-hour span allows start-day classification:

- Days before the day preceding the cutoff day are wholly old for correctly
  placed valid stamped recordings.
- The preceding day and cutoff day are inspected as boundary/lookback days.
- Days strictly after the cutoff day are conservatively skipped by retention.

Only the third group changes behavior. Old and boundary days are handled
identically: both are opened and every file in them goes through the per-file
predicate, identity preflight and immediate revalidation, because a folder date
alone is never proof about its children. The classifier names all three groups and
the tests assert them, but there is no cheaper path for wholly old days, so "old"
and "boundary" do not differ in the work done.

Classification never authorizes a whole-directory recursive deletion. All files
in inspected days still pass the original per-file predicate, identity preflight
and immediate revalidation. Files are individually unlinked; only empty day
folders can be removed by rmdir. No RemoveAll is used.

## Agreed option 2 exception

Skipping newer days preserves all their contents without inspection. An expired
valid name placed in a newer folder, a malformed name with an old parseable start,
or a malformed eligible name with old mtime therefore remains until that folder
ages into the inspected range. This delays exceptional cleanup; it does not delete
anything that the former predicate would retain. Once inspected, the existing
stamped-end/start-prefix/mtime rules apply unchanged. Direct-root file selection
is unchanged. A folder date is not proof about malformed or misplaced children.

The generic selector API and delete-all do not use the skip policy. They continue
to inspect newer folders and apply their original selection semantics.

## Bounds and safety

Root enumeration remains necessary. All real day directories, including skipped
ones, count toward the 400-day limit; root and opened-day processing retain the
8,192-entry limit. Skipped folder contents are not checked against the per-folder
limit until opened in a later pass. Directory symlinks are not followed.
Ownership admission, policy/registry/shutdown guards, no-follow descriptors,
identity checks, preflight, individual unlinks, directory syncs and partial-error
reporting are preserved. Trusted directory ancestors remain required.

Per-call observation separates root/day read attempts, all returned batch names,
processed names (including an overflow name), identified/opened/skipped day
folders, explicit stat/identity calls, unlink/rmdir attempts and successes, and
sync attempts. Batch read-ahead is charged even on early failure. Metadata counts
include guards as well as candidate inspection; successes are not a guarantee of
durability if a later sync fails. Read/stat/statat/sync operations are injectable
per pass; the existing unlink seam remains. No mutable global instrumentation is
introduced. Work objects must not be shared or reused across passes.

## Caller paths and scope

The production cleanup tick calls cleanupRetentionSnapshot, which admits the
media lease and invokes cleanupExpiredPublishedLeased. That function now chooses
the retention-specific day policy. Explicit deletion uses deleteAllPublishedLeased
and the generic shared engine without the policy. The older glob cleanup helper
(`cleanupOldRecordings`) was test-only and has been removed; its tests now run the
production cleanup tick. Stop/join and persistence ordering are unchanged.

Focused regressions cover UTC classification, midnight/equality/cross-day cutoff,
delayed malformed/misplaced cleanup, unchanged generic/delete-all behavior,
constant work when skipped contents grow, the skipped-day budget and read failure
observation. Checkpoint 5 was accepted on
`fa6f7854574c1378b029c37774fa31711b43c50a`, run 45676664390, with all eight
required checks successful (check-run verification only; job logs were not
inspected). See [CHECKPOINT5_READINESS.md](CHECKPOINT5_READINESS.md).
