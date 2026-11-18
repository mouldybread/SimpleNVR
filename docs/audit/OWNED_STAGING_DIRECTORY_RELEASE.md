# Opt-in checkpoint-last staging directory release

Preparation base: d8711672e09ca8684acbf3ef5b1dec34898fbe1f, user-reported green.

## Scope

Three additions provide the final phase missing from the c24bbe9 input primitive.
releaseOwnedStagingLeased composes that unchanged input engine with checkpoint-last
unlink and descriptor-relative empty-root removal. There is NO production caller:
event release still retains every job. No eligibility capability, publication rule,
checkpoint schema, workflow, expiry or operator-recovery automation is changed.

The caller must independently establish positive successful-capture/publication/
persisted-checkpoint eligibility, revalidate registry/cancellation/output and pass
one held shared media lease plus a pure guard. Inventory membership and an input-
phase result are not that authorization. Inventory access and teardown are serialized.

## Sequence and result

The checkpoint is read only through its already recorded descriptor, with slot
identity checks before/after ReadAt and a nonempty 64-KiB bound before any input
unlink. No filename discovery/re-adoption occurs. This treats payload bytes as
opaque evidence; it does not itself certify manifest semantics or complete capture.

The input primitive performs full-inventory preflight before any unlink and reports
the actual removed subset. Any input-phase failure leaves checkpoint removal
unattempted. After input success, the wrapper requires exactly the recorded
checkpoint, fresh full preflight, bounded reread and another ownership/guard check.
It unlinks recovery.json relative to the pinned root, syncs that root even after
late cancellation, checks that the directory is empty and removes ONLY that root
via Unlinkat(AT_REMOVEDIR) relative to its pinned parent. Parent Sync is attempted
even if cancellation follows rmdir. No recursive sweep or fallback exists.

Inputs, CheckpointUnlinked and DirectoryRemoved describe actual completed phases.
Complete is true only when the directory was removed without a reported error.
A post-rmdir Sync/cancellation/close error therefore reports DirectoryRemoved=true,
Complete=false; it never pretends the namespace was restored or deletion rolled back.
Neither a result nor successful preflight authorizes any further deletion.

## Recovery on final-phase refusal

If checkpoint unlink succeeded but root removal did not, restoration is attempted
under an independent Background-rooted recordingFinalizeGrace context while
borrowing the SAME lease. The existing creator-owned writer creates a new checkpoint
with no-replace semantics and fresh provenance. Its slot and bytes are verified
before CheckpointRestored is set. Late files, nested entries and foreign checkpoint
names are never swept, overwritten or adopted. The old checkpoint descriptor stays
live through restoration and then closes; the restored inventory pin stays until
normal inventory teardown. No new descriptor is opened from a discovered entry.

If root/parent identity changed, a foreign recovery.json arrived, or storage refused
restoration, durable checkpoint preservation cannot be guaranteed. The result returns
the original bounded CheckpointEvidence bytes along with the error; these are only
an in-memory fallback, NOT durable recovery evidence. Reported restoration errors
remain errors even if an owned restored slot can subsequently be verified. Media
inputs already unlinked are never recreated. A future production caller must handle
these partial/ambiguous receipts explicitly before this engine can be enabled.

Trusted directory ownership remains required: stat/unlink is not an atomic compare-
and-unlink against hostile external writers. Context budgets do not interrupt
kernel I/O. No power-loss, ancestor/mount or transactional-deletion guarantee is
added. The helper never acquires/releases the caller's lease or locks NVR.mu.

## Coverage and remaining integration

Nine table-driven regression functions cover full ordered removal; foreign file,
directory, symlink and FIFO refusal before inputs; partial input failure; checkpoint
unlink/rmdir/Sync errors; cancellation and guard revocation; late arrivals before
scan and at rmdir; restoration success/failure/foreign checkpoint collision; root/
parent replacement; post-rmdir ambiguity; bounded checkpoint bytes and refused
admission. Fixtures are synthetic owned directories. File-operation seams make
interleavings synchronous, not sleep-based. Existing full-package Actions race
coverage discovers them automatically; no checks are weakened or bypassed.

This advances the engine/partial-cleanup components, not production cleanup or
whole-stage acceptance. Positive eligibility, registry/shutdown/output checks and
production activation under the existing finalization lease remain the next work.
