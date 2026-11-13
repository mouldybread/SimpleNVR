# Opt-in nonrecursive staging-input release

Base: 1b54ec6eaa5df8cf2c79530fb3e32e355689cb98 (user-reported green).

This batch adds an input-only destructive primitive with no production caller.
It does not change recorder.go, releaseOwnedJob, cleanup eligibility, publication
ordering, checkpoint schema or workflows. Existing recursive production success
cleanup is STILL PRESENT and is not made safe by this standalone prerequisite.

## Contract

releaseOwnedStagingInputsLeased borrows a held shared media lease and a serialized
creator-proven inventory. The caller must supply an independent authorization and
registry/context/shutdown guard; inventory membership is never eligibility. The
helper neither acquires/releases the lease nor locks NVR.mu. Guard callbacks must
not reacquire ownership, wait for workers or retain registry locks across I/O.

An explicitly recorded recovery.json is mandatory. Full bounded inventory
preflight precedes the first unlink. Only recorded event.mp4, concat.txt and
pre_000.mp4 through pre_127.mp4 are accepted as inputs; recorded unrelated names
also refuse. Every candidate rechecks root/parent, checkpoint, creator descriptor
and nonfollowing named identity before descriptor-relative unlink. Successful
unlinks are reported in deterministic order; removed input pins close immediately.
There is no pathname re-adoption, recursive traversal, RemoveAll or rmdir.

The checkpoint, its pin and root always remain, including after all inputs are
released. This deliberately postpones checkpoint-last removal/late-rmdir recovery
to a later slice. Directory Sync is attempted after any successful unlink even
when a later phase fails or the operation is canceled. Final guard/preflight runs
after Sync before InputsReleased can be true. Failure returns the actual Removed
subset and never claims rollback. Late unknown entries are retained, not swept.
InputsReleased means only this input phase finished, not complete job cleanup,
durable publication, successful capture or permission for any later deletion.

Stat then unlink is not atomic against hostile external writers. Trusted owned
directories remain required. Hard-link ctime changes conservatively refuse later
identity checks; no identities are refreshed by filename. Kernel I/O is not made
interruptible by the operation context. Inventory access/teardown is serialized.

## Regression coverage and next boundary

Seven table-driven tests cover successful input release and repeat invocation;
foreign file/directory/symlink/FIFO refusal before unlink; partial unlink failure;
input/root/parent replacement, modification and missing input between unlinks;
late entries during unlink and Sync; cancellation before/mid-operation and during
Sync; Sync/close failure; required checkpoint, guard revocation and invalid lease.
Checkpoint bytes/pin/root, retained bytes, removed-pin closure and borrowed-lease
ownership are asserted. File-system seams make interleavings synchronous, not
sleep-based. Existing Safety verification already includes go build ./...,
go test -race -count=1 -timeout=120s ./... and go vet ./...; new tests are picked up
without changing the core, both browser acceptance runs or aggregate verify job.

Next integration still requires positive cleanup eligibility/default retention,
a shared publication -> published-checkpoint -> release boundary, and checkpoint-
last/final empty-directory removal with explicit partial-outcome evidence. No
whole acceptance point or overall audit workstream is declared complete here.
