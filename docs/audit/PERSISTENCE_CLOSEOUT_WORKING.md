# Persistence closeout working record

Source-review candidate: 516ad04760f55371ccc81e68c8917944eca58713.
Starting candidate 2ba5881421f58041b531b7f6960a24d208899deb was user-reported green.
Section 1 remains OPEN. This record supersedes the pending-work and evidence
requests in PERSISTENCE_WRITE_RECONCILIATION.md; that document is historical.

## Accepted recording-intent policy

On 2026-11-17 the operator selected option A: retain recording intent and retry
event-storage initialization. The existing tracked worker owns setup retries
and Done; retry uses the existing cancellable five-second delay. Failure does
not clear IsRecording or require an asynchronous stopped-intent save. The manager
remains nil until setup succeeds, so requested event capture returns 503 while
unavailable rather than silently changing to metadata-only logging.

Explicit Stop cancels/joins and clears intent. Process shutdown cancels/joins
without clearing restart intent. Continuous recording errors also retry without
clearing intent. Neither enabled intent nor a nonnil manager certifies healthy
acquisition, camera connectivity, complete coverage or successful footage capture.
Setup syscalls themselves can block; cancellation does not interrupt all I/O.

The camera template now says Recording enabled or Recording stopped and visibly
explains intent versus acquisition. Start/Stop selection still uses intent.
TestCameraTemplateDescribesRecordingIntentNotHealth renders both states; this is
an authored regression, not observed execution or live/browser qualification.
No local Go compilation or execution is claimed for this batch.

## Reviewed configuration mutation and consumer paths

- main.go and persistence.go: startup load validates before publishing registry
  state and aborts on failure; recovery precedes worker startup. A dedicated
  persistence mutex serializes snapshot/read/backup/replace. Registry locks are
  released before persistence I/O.
- lifecycle.go and recording_control_persistence.go: controls serialize through
  the save attempt. Failed replacement admission can leave changed mode/stopped
  intent, which the wrapper attempts to save before returning combined errors.
- recorder.go and event_storage_retry.go: asynchronous recording/setup failures
  retry without clearing persisted intent. events.go checks intent, event mode
  and manager availability; the camera template is an intent consumer.
- handlers.go and retention_snapshot.go: reorder/retention mutate then save;
  validated retention setters release policy/registry locks before handler save.
  Save failure does not restore memory or provide durable policy transactions.
- camera_registration.go and go2rtc_scan.go: production manual-add and scan use
  serialized registration with persistence. The separate applyScanStreams helper
  is nonpersisting; source-wide caller enumeration remains to be completed.
- events.go and cleanup.go: trigger, clear and periodic event expiry can change
  metadata before save. Trigger can queue capture before metadata save fails.
  HTTP failure is not proof capture never started; expiry save failures are logged.
- camera_deletion.go: stop/join and stopped-intent save precede media admission
  and unlink. Successful removal unregisters then attempts a final save under
  media ownership. Final failure cannot restore unlinked files and can leave
  disk registration stopped while memory is unregistered. This is persistence
  boundary review only, not section-2 camera-deletion closure.
- shutdown.go and shutdown_completion.go: workers and admitted mutations drain
  before final persistence. Expired drain does not schedule a late save. Once
  started, persistence may outlive the deadline; no cancellation/rollback claim.

These APIs are not transactions spanning memory, workers, media and disk. Failed
saves may leave memory/disk different; later saves snapshot current memory. A
post-rename failure may already have changed the visible primary. This inventory
is not yet an exhaustive source-wide all-writer/all-consumer attestation. Auth/TLS
and event recovery manifests are separate stores.

## Failure-stage reconciliation

Existing atomic-writer coverage has nine cases: create, partial-write error, short
write without error, file sync, file close, rename, directory open, directory sync
and success. Short writes return io.ErrShortWrite before rename. Directory-open
or directory-sync failure follows rename: new bytes may be visible despite error.

The save-level suite covers backup callback refusal, primary callback refusal
before rename and real post-rename primary directory-sync failure. The first two
are injected callback refusals, NOT direct save-level backup-replacement/primary-
rename failures. The lower atomic-writer rename case provides separate coverage.
Whether direct save-level replacement-failure cases are needed remains a named
coverage reconciliation item, not an established production defect. Do not
duplicate existing write/sync/close coverage or claim power-loss simulation.

The primary reader checks type through O_PATH, then reopens its held descriptor
through procfs. An internal reopen ENOENT can enter caller absence classification;
load/save Lstat the original primary and refuse if an entry remains. Existing
backup entries, including dangling symlinks, also prevent pristine initialization.
No evidence-overwrite defect was established by this review. Regular-file symlink
reads remain supported. Concurrent external filesystem races are not covered.

## Independently inspected execution evidence

The operator supplied /aq/nvr/job-logs.txt for run 44106894648, job 134121562771.
The inspected text identifies checkout 516ad04760f55371ccc81e68c8917944eca58713,
Go 1.26.11 linux/amd64, successful go build ./..., go vet ./..., core wrapper
completion and package pass. The core assessment reports 15 skipped events and
explicitly denies cleanup authority, runtime qualification and whole-audit
clearance. Individual run/pass records, not that assessment alone, establish
execution of persistence and setup regressions. The wrapper's expanded core
command identity was inspected in earlier run metadata, not printed in this log.

Observed passes include both persistence failure-boundary suites, nonregular
primary/regular-read suites, missing recovery-entry safeguards, pristine save,
exact backup rotation, concurrent saves, event setup retry/admission/cancellation,
recording-control persistence and shutdown-intent regressions.

The dedicated namespace step explicitly runs go test -race -count=1 with the
TestStorageFailureMountNamespace filter and passes detached-mount fallback,
runtime continuous/capture/assembly failure, unprivileged permission, byte/inode
exhaustion and read-only fixtures. Ordinary-run skips are not execution; these
later dedicated passes are. Deployment-filesystem compatibility remains skipped
without an explicitly supplied deployment scratch parent.

The runtime image built and active-container storage fixture verified two cycles
with active continuous/event buffering using file-fixture input; live RTSP was
explicitly not tested. This is evidence for the reviewed candidate, not execution
of the new template regression or final deployment-storage qualification.

## Remaining section-1 gates

1. Finish exhaustive source-wide configuration-writer and recording-status
   consumer references, including any additional callers of reviewed helpers.
2. Resolve the direct save-level replacement-failure coverage item above.
3. Obtain exact-content approval for this batch, then inspect applicable execution
   on its resulting SHA, including the new rendering regression and build/race/vet.
4. Reconcile any concrete findings into the same bounded section and issue a
   closure verdict with supported guarantees and explicit accepted restrictions.

A prepared batch, pushed commit or aggregate green result is not section closure.
No recovery/cleanup/retention/staging-expiry activation, deletion authority, live
deletion, merge or deployment is authorized. main remains outside write scope.
