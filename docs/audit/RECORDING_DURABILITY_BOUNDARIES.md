# Recording control persistence and publication durability

Prepared against 03cd49ff273eef607d3bf9e47f84df9b93c93335. This bounded batch
addresses two observed code paths, not the whole recovery/race audit.

## Failed replacement is a changed state, not an unchanged refusal

Previously a mode-change handler saved only after transition success. Replacement
admission can fail after the old recorder joins and the new mode is assigned.
The in-memory camera is then stopped in the new mode while the durable registry
can still request recording in the old mode. A restart could resurrect that stale
intent if no later successful save intervened.

Start/stop/mode HTTP controls now use transitionRecordingAndSave. The common
transition remains serialized by recordingControlMu through persistence. Saves
run without NVR.mu held. On transition error, a changed camera pointer, mode or
recording intent triggers a save before the error is returned; unchanged refusals
are not saved. Successful requests still save, including idempotent successes.
Missing persistence is rejected before mutation. Existing internal/startup methods
retain their non-persisting contract for lifecycle composition and tests.

A failed save returns errRecordingPersistence with HTTP 500, ahead of admission
status, while retaining the underlying errors for errors.Is. No worker restart,
mode rollback or claim of durable success is synthesized. Existing shutdown
restoration of restart intent is preserved; a changed mode is still saved. This
is not a transaction across process execution and configuration persistence.
Asynchronous worker failure, all other metadata writers, save failure recovery,
and the crash window before a save completes are not comprehensively solved here.

## Rename is not the durability boundary

Previously publishRecordingWithOps returned after no-replace rename without file
or parent-directory Sync. A successful event assembly could checkpoint published
and release staging without those output durability steps.

The production publisher now opens a non-following/nonblocking read descriptor,
checks the nonempty regular-file identity, validates media, Syncs the pinned media
file and rechecks both descriptor and named source before no-replace rename.
Parent directories are opened before validation and identity-checked before
rename. After rename, both source and destination directory descriptors are
Synced (one descriptor for the same lexical parent). Final-file identity is
checked before success. File/directory Close failures also fail publication.

File-Sync failure prevents rename. Once rename succeeds, cancellation does not
abandon directory Sync: cancellation is reported afterward so event input
preservation remains conservative. Directory-Sync failure is an ambiguous
publication outcome: the final may already exist and the temporary may be gone.
There is no rollback, removal of the final, retry or overwrite fallback. The
existing event error branch retains staging and records publication failure; this
batch does not change it or automatically delete/reassemble retained inputs.
Collision errors retain os.IsExist compatibility when descriptor closes succeed.

Sync is a requested filesystem durability boundary, not proof of power-loss
behavior, durable remote storage, complete media capture, or universal hardware
flush correctness. Media validation still proves only the existing limited
readability checks. Published partials remain partial.

## Remaining ownership/race limits

Path-based validation and rename still have compare/use gaps. Pinned Sync and
identity checks are not atomic compare-and-rename, mount identity verification or
protection from hostile external writers/ancestors. Retention and explicit
published-recording deletion may still race publication. Successful staging
release still uses its existing owned-job cleanup path; unexpected-entry and
external-replacement hardening there remains open. Startup recovery behavior,
checkpoint schema, automatic expiry, deployment rules and browser tools are not
changed. recordingControlMu and filesystem waits remain potentially blocking.

## Authored regression coverage and evidence

Six publication groups cover Sync ordering, one/two parents, file-Sync failure,
post-rename directory failure without rollback/retry, cancellation around rename,
post-Sync source changes, parent replacement and final identity replacement.
Existing collision, source-change and real-media tests remain unchanged.

Six control groups cover failed second admission with actual save/load and no
restart, unchanged preflight refusal, combined persistence/admission error status,
control-lock retention with registry accessibility during save, missing callbacks
and invalid/shutdown admission, and shutdown-restored intent with a changed mode.

Preparation verifies original Git blob SHA1s, exact reversible patches and payload
serialization. Authored tests have not been run locally: no Go build/test/race,
vet, gofmt, Docker or browser candidate execution is claimed. Candidate CI and
runtime evidence, broader ownership review, and final exact-head branch/docs
reconciliation remain required. No main change or merge is authorized here.
