# Persistence closeout working record

Baseline: 2ba5881421f58041b531b7f6960a24d208899deb, user-reported green.
Prepared only; section 1 remains OPEN. No CI execution is claimed here.

## Prepared remediation

Event-storage initialization remains inside the tracked event recorder and retries
with the existing cancellable five-second recording retry delay. Initialization
failure no longer clears IsRecording. The buffer manager remains nil until setup
succeeds; existing event admission therefore returns 503 rather than converting a
requested capture to metadata-only logging. Stop cancels/joins and clears intent;
shutdown cancels/joins without clearing restart intent. Setup syscalls themselves
remain potentially blocking. No claim of healthy acquisition follows from intent.

The patch replaces the event recorder entry body and adds a per-call setup seam,
four authored lifecycle/admission cases, and this working record. Tests use fake
setup and worker execution; the save/load check uses a disposable config path.
No media process or actual recording directory is touched by these new tests.

## Caller review completed so far

Startup load fails closed. Explicit recording controls serialize transition and
save, preserving admission and persistence errors without rollback claims.
Camera registration serializes commit/save and reports persistence failure.
Reorder, retention handler completion, and event clearing mutate then save; errors
do not restore the previous memory state. Event capture may be queued before its
metadata save fails. Shutdown drains workers/mutations and attempts final save;
a started save cannot be canceled and can outlive a deadline. These are not
transactional APIs. Retention setter internals and complete mutation inventory
still require final tracing; this is not an exhaustive all-writer attestation.

## Remaining section-1 work

1. Complete writer/status inventory including retention setters and imported
   camera registration; confirm no unreviewed production configuration writers.
2. Compile/run the prepared setup policy tests and integrate exact approved files.
3. Reconcile tests for atomic-write create/write/sync/close/rename/directory-sync,
   backup failure, and post-rename visibility. Add only genuinely missing tests.
4. Check primary-reader procfs reopen error classification against the existing
   Lstat safeguards; no evidence-overwrite defect has yet been established.
5. Inspect exact-candidate core execution artifact and build/vet results, then
   issue section closure or named blockers. Current connector exposes source
   tools but not Actions artifact/run-log retrieval; workflow source is not run
   evidence. Candidate changes require fresh applicable execution evidence.

No move to section 2, cleanup activation, live deletion, merge or deployment.
