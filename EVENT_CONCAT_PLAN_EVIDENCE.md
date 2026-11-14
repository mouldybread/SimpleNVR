# Descriptor-verified production concat-plan evidence — NEW IMPLEMENTATION

Base: 6257b0e4db5d7c62d276c6efae556449fce0ea73. Remote audit head verified.
Base green is user-reported, not independently retrieved Actions evidence.

## Bounded next step and the wall-clock blocker

The timing batch freezes the requested labels/selected pins and host Run bookends;
none is a trusted first/last-frame wall-clock anchor. That limitation is unchanged.
This batch addresses a separate required part of coverage composition: binding the
actual assembly INPUT PLAN to the frozen selection. It does not invent a timestamp
mapping or enable positive interval coverage/cleanup. Establishing a trustworthy
producer/media clock mapping still requires a separately reviewed source/policy.

The earlier assembly provenance required concat.txt to be inventoried, but did not
independently compare its bytes to the frozen input order. Now an exact canonical
plan must contain every frozen pre_NNN.mp4 entry once, in that order, followed by
exactly event.mp4. No omissions, aliases, duplicate inputs, extra directives or
matching-prefix-only acceptance is permitted. Filename labels are not media proof.

## Production observation and writer completion

The REAL creator callback writes as before. After successful real Remember, a bounded
read checks the inventoried concat descriptor while borrowing the EXISTING creator-
registration lease. No new acquisition, source-path rediscovery or descriptor is
introduced. Whole-inventory preflight and held-descriptor snapshots bracket ReadAt.
ReadAt requests the expected byte count plus one, requires exact bytes AND EOF,
and does not change the shared offset. Expected-plan generation is bounded by
maxRecoveryInputs and maxRecoveryManifestBytes. Access time is not a binding field.

A successful registration observation is NOT ready yet. Only the production concat
wrapper returning, including the writer Close result, seals it. A callback returning
nil, a descriptor merely being present, a close/registration error or malformed
actual bytes cannot provide a ready receipt. Observation refusal invalidates execution
qualification but does NOT replace the original writer result or undo publication.

Assembly join and qualification require that exact ready receipt. Production release
revalidation pins its identity and rechecks exact descriptor bytes before and after
existing evidence I/O, under the already transferred lease and caller's current
context. Normal local context cancellation is not stored as a permanent requirement.
No registry mutex is held across these reads. Metadata budgets stay independent;
there is no new timeout renewal, probe or media-gate acquisition. Kernel I/O remains
uninterruptible. The file-size limit bounds bytes read, not every kernel stall.

The receipt binds execution, frozen selection, inventory, concat descriptor and
capture descriptor identities, plus compared concat metadata and a digest of the
exact observed/expected bytes. It is not immutable-content protection against a
hostile external writer, an atomic guarantee of what FFmpeg opened, or proof that
all input frames actually appear in the output. Trusted directory ownership remains
required; media correspondence and requested/prebuffer interval coverage remain open.

## Unchanged safety boundaries

No media command arguments, child count, process lifetime, capture/assembly return,
persistence-failure latch, publication behavior, unconditional staging retention,
terminal descriptor closure, slot return or lease handoff/release is changed.
The existing two evidence probes and their aggregate budget remain unchanged.
No new lease, persistent pin, goroutine, global map, schema, workflow, HTTP behavior,
cleanup capability, deletion, deployment or merge is added. Main stays untouched.
The corrected private probe buffers and their regression file are not modified.

## Candidate validation

New Go regressions exercise the real owned writer and descriptor path, exact ordered
plans, wrong/reordered/duplicate/missing inputs, extra/truncated bytes, prefix-only
refusal, registration-versus-close sequencing, synthetic close-result failure,
missing/copied/wrong-owner/digest receipts, before/after-I/O substitution, cancellation,
foreign leases, unchanged descriptor offset and terminal lifetime/teardown.
Synthetic plan/process fixtures are NOT evidence of camera footage or media coverage.

Preparation checks base Git blob identities, bounded ReadAt structure, preservation
of existing media commands/acquisition counts, and patch application against verified
base files. Go/gofmt are unavailable locally: compilation, Go tests, vet, race, both
Chrome runs and whole-branch checks have NOT run. Earlier green is for the BASE only.
New-head Actions validation remains mandatory after separately approved push.

Next: establish trustworthy media clock provenance/provable bounds, then verify
selected-input/output media correspondence and requested/prebuffer coverage before
positive cleanup authorization. Partial-release/restoration receipts, remaining
A01-A12 integration, real-camera/deployment and whole-branch/browser acceptance remain
open. This observed input-plan receipt must not be used as a cleanup permit.
