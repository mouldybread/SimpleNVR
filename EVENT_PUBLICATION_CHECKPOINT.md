# Event publication and published-checkpoint ownership

Preparation base: a8b1e06c09ff1c8c9ff00affa1183950c9228642, user-reported green.

Production event assembly now calls publishEventRecording after FFmpeg Run has
joined. It acquires one bounded shared media lease, calls the existing durable
borrowed publication function, persists the published checkpoint and calculates
the existing partial/cancellation/persistence status before releasing ownership.
File Sync, no-replace rename, directory Sync and post-rename ambiguity semantics
remain in the unchanged publication engine. No lease spans capture or concat Run.
Publication validation still uses FFprobe under the publication lease as before.

The job has a synchronous scoped recoveryBorrow. The production checkpoint writer
borrows that operation's lease and deadline, including when a test writer wraps
the original method. It does not reacquire/release ownership or start a new budget.
An explicitly supplied canceled checkpoint context still refuses. The borrow is
cleared before release on every return. Outside this scope, checkpoints retain
their existing independent metadata budget and fresh ownership acquisition.
Injected recovery writers and manual legacy jobs keep compatibility behavior;
this does not convert an injected success into descriptor-proven disk persistence.
Job/inventory use and teardown remain serialized, not concurrently callable.

A successful publication with failed checkpoint still returns the published path
and retains staging; it is not rolled back. A publication error takes the existing
caller failure-checkpoint/preservation path after this lease ends. The finalization
budget now covers both publication and the default published checkpoint, rather
than extending it with a second independent metadata budget. Capture cancellation
is still distinct from finalization cancellation; it prevents a success status,
not independent metadata completion for an already published partial.

Terminal release still unconditionally retains every job, checkpoints retention
under a fresh lease, closes the inventory and returns capacity. This slice does
NOT grant positive cleanup eligibility, connect the input-release engine, remove
checkpoint/root, or span terminal release with the publication lease. Camera
registry/shutdown revalidation and orphan/add-camera coordination remain pending.
The existing keepStaging calculation is compatibility state, not deletion authority.
No complete-capture, hostile-writer, power-loss or whole-stage clearance is claimed.

Seven focused regression functions cover same-lease checkpoint borrowing and a
cancelable competing owner, the production durable-publication call path (synthetic
probe), checkpoint failure after publication, publication ambiguity, partial/late
capture cancellation, canceled ownership admission and expired borrowed budgets.
Existing assembly, failure-preservation and creator/no-gate-across-child tests stay
in place. GitHub Actions Safety verification discovers the new tests automatically;
core, both browser attempts and aggregate verification are unchanged.
