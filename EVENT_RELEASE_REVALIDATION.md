# Live event release revalidation (not cleanup authorization)

Preparation base: f8942942347d5da4d344c456dcb58ce6d51cc4e6, user-reported green.

After the published checkpoint, production finalization binds a read-only check to
its job, manager/camera pointer, camera ID/path, inventory, output receipt and held
lease. Terminal release checks again before and after retained-checkpoint writing.
Results remain private observations; publication/checkpoint outcomes and the legacy
status flag are unchanged. ALL terminal jobs still retain staging. No release-engine
call, unlink, schema, workflow, recovery automation or new HTTP behavior is added.

The check requires a live event-based registered camera and the same active manager,
recording intent/cancel handle, no shutdown or manager cancellation, an unchanged
publication-derived output, exact owned staging preflight, capture/concat entries,
and a descriptor-read checkpoint matching the in-memory published manifest bytes.
Registry read/write locks cover only field checks, never descriptor/stat/enumeration/
checkpoint I/O. Registry state and context are checked again afterward. Unknown,
changed or modified observations refuse rather than refreshing identities by name.

A failed/partial/canceled handoff observation is captured before the assembly context
is canceled on normal return. Terminal checks deliberately do NOT reuse that canceled
function-local context: they use terminal metadata's independent budget plus the LIVE
manager context. This distinguishes normal deferred cancellation from an actual stop.
Refusal never blocks the independent retention-checkpoint attempt, clears a persistence
failure latch, or prevents descriptor/capacity/lease teardown. Missing/injected/manual
ownership evidence refuses; fake successful callbacks are not proof of persistence.

This checks necessary live/evidence conditions, NOT requested-duration/full-capture
or successful assembly provenance. Even a successful check is not a cleanup permit.
Positive complete-capture/assembly authorization and explicit handling of partial
release receipts must still be integrated before activating the opt-in engine.
Orphan/camera-add coordination and final acceptance remain open. Hard-link ctime
changes conservatively refuse; hostile writers, power loss and mount/ancestor
replacement are not solved, and context does not interrupt kernel I/O.

Six regression functions, including table-driven cases, cover necessary-check success and owned
checkpoint replacement without cleanup, changed registry/lifecycle state, modified
checkpoint/input/output evidence and unknown entries, context/lease/handoff refusal,
registry reader/writer availability during injected I/O plus post-I/O revalidation,
and shutdown observed after terminal metadata persistence. Fixtures use synthetic
registry/process witnesses, not real-camera completion evidence. Existing Actions
full-package race/build/vet and both browser/verify jobs remain unchanged.
