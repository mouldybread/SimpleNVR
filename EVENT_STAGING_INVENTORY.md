# Read-only owned staging inventory, bounded subtask 5

Baseline: 668ed00a9e313baf6c319af61f96626ad84515b5. Additive prerequisite only:
no recorder, checkpoint writer, publication, releaseJob, retention or deletion
caller is changed. Existing recursive releaseJob cleanup remains unresolved.

The future owner must open an inventory while its new job directory is owned,
then register explicit basenames with descriptors originating in its own
creation/pinning operations. Opening files discovered at cleanup time is NOT
ownership. Remember verifies the descriptor against a nonfollowing relative
lookup, pins a CLOEXEC duplicate, and refuses duplicate names or mismatches.
It never infers ownership from extensions, known-looking names, age or size.

Parent/root directory identities are pinned. Each inspection rechecks named
directory identities, type, mode and ownership. File identity checks include
inode/device, mode/ownership, size and mtime/ctime. Open descriptors prevent
inode reuse while retained. Metadata changes conservatively invalidate evidence;
this is not a content hash or protection against hostile same-inode writers.

Preflight uses a fresh directory description, requires exact membership and
checks both held file descriptors and nonfollowing named entries. Unknown,
nested, symlink, special, missing, modified or replaced entries cause refusal.
All filesystem operations are read-only apart from descriptor lifetime changes.
There is NO unlink, directory removal, recursive fallback or checkpoint rewrite.
Successful preflight alone grants NO cleanup authority or publication proof.

The limit is maxRecoveryInputs+3 pinned file descriptors (prebuffer budget plus
capture, concat metadata and checkpoint), plus parent/root and a transient scan
descriptor. The owning caller must serialize methods and Close; never copy or
close while borrowed. Methods borrow an active shared media lease, never acquire
or release it. Descriptors may outlive an admission interval; the gate must NOT
be held across capture. Cancellation is checked between inspection steps; kernel
filesystem calls are not made interruptible. Close is idempotent and joins errors.

Six new test functions cover descriptor provenance/lifetime, repeat preflight,
unknown entry kinds, replacements/modification/missing entries, root/parent
replacement, names/duplicates/limits, leases, cancellation and close. Existing
tests are untouched. Go/gofmt/Docker are unavailable: tests are authored, not
executed. Patch/JSON/source checks are not build/race/vet/browser/container proof.

Still required: creator-side inventory adoption; owner-writer checkpoint
replacement tracking; complete-capture/publication/persistence eligibility;
shared ownership through published checkpoint and release decision; nonrecursive
unlink with per-entry revalidation, cancellation/partial outcomes, checkpoint-last
preservation/restoration and empty-directory removal; default-retain releaseJob
integration and slot-return tests; registry/orphan integration and final matrix.
No production staging-safety fix or merge readiness is claimed by this batch.
