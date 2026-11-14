# Ordered event execution provenance — NEW IMPLEMENTATION

Base: 23ceada9bb1d7ca2fb9703bd89075ce1a7aa26d1.
This is newly implemented work, not a recovered post-checkpoint original.

## Observation and ordering

Production pinning attaches one bounded, owner-serialized execution observation to
its new job. It binds the job, manager, camera, inventory and manifest pointers,
job directory, event identity and requested duration. Manual/zero observations
refuse. No global mutable execution map, process, lease, descriptor or schema is
added. The observation dies with terminal inventory teardown.

The one-way sequence is ready -> capturing -> captured -> assembling -> joined ->
assembled. Any invalid sequence, reentry, failure or mismatch refuses permanently.
Capture enters in recordEventDuration. Qualification occurs only after the real
capture helper has returned (including Run/Wait, final descriptor registration and
creator close), successful owned captured persistence and a live completion-context
snapshot. Assembly enters in concatenateEventRecording for that same capture path.
It must finish its real Run successfully, adopt concat evidence, successfully use
the actual durable observed publisher and persist the owned published checkpoint.

Receipts are a fixed-size SHA-256 digest of bytes successfully written by the real
owned checkpoint writer, not a hash of media or a substitute for descriptor checks.
Capturing, captured, assembling, publishing and published snapshots must correspond
to those actual writes. Injected nil callbacks alone cannot produce receipts.
Publication is witnessed only after the actual observed publisher returns success,
including deferred close results, not merely after its observer has run. A generic
publication-success callback cannot provide that witness.

Completion contexts are checked before normal local deferred cancellation. They
are not retained as permanently live cancellation requirements. Live manager
cancellation and the existing registry/output/checkpoint/inventory checks continue
to be required during revalidation. The revalidation object pins the observation's
identity and rechecks it after I/O. A fresh observation cannot replace a used one.

## Compatibility and unchanged safety boundaries

Provenance refusal is observational: direct/manual assembly may still publish its
existing readable output or partial, but cannot qualify as production execution.
A published-checkpoint error still does not roll back durable publication. Terminal
staging retention remains unconditional, including legacy keepStaging=false jobs.
Independent metadata-finalization budgets, sticky persistence failure, descriptor
ownership, terminal lease handoff and slot return are preserved. No media process,
lease acquisition, deletion, cleanup-engine call or HTTP/schema behavior is added.

## Tests and Actions

The former creator-only revalidation fixture now executes production capture and
assembly entry points with explicitly synthetic executables, not manufactured
success bits. Existing registry, replacement, checkpoint, slow-I/O reader/writer,
terminal teardown and retained-checkpoint assertions remain. New regressions cover
ordered success, normal deferred cancellation, process/cancel/registration failures,
selective nil checkpoint callbacks, actual checkpoint failure, direct assembly,
injected publication, bound identities, replay and out-of-order/default refusal.
These witnesses do not produce or certify real camera footage.

The existing safety-ci.yml discovers all package tests with build/race/vet and
retains core/storage/lifecycle checks, both mandatory isolated Chrome runs and the
aggregate required check. No coverage-wiring gap was found; workflows are unchanged.

## Not a cleanup capability

Successful execution does NOT establish requested duration, full prebuffer/event
coverage, a tolerance policy, genuine RTSP behavior or deployment compatibility.
Those remain mandatory separate requirements before positive cleanup authorization.
Production engine activation, explicit partial-receipt handling, the remaining
ownership matrix and whole-branch audit/acceptance remain open. Do not merge from
this prerequisite alone. Main remains untouched and merge needs separate approval.
