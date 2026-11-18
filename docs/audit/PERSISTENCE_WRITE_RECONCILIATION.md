# Persistence closeout: writer and failure-boundary reconciliation

Reviewed baseline: 4260b0647a551d44de06410a60002567a83461c4.
Latest independently observed result: source retrieval only; no CI run inspected.
Previous candidate 2ba5881 was user-reported green. Section 1 remains OPEN.

## Writer/caller inventory reviewed across this conversation

- main.go: save/load wrappers target /recordings/cameras.json; invalid startup
  load aborts; recovery precedes recording startup.
- persistence.go: dedicated mutex serializes snapshot/read/backup/replace;
  registry lock is released before I/O. Load publishes decoded state only after
  validation. Existing invalid or recovery entries refuse overwrite.
- recording_control_persistence.go and lifecycle.go: explicit controls preserve
  changed-state save attempts and report combined failures; no disk rollback.
- recorder.go / event_storage_retry.go: committed setup retry preserves intent;
  manager unavailable until setup. New candidate's execution remains unverified.
- handlers.go: reorder and retention mutate then save and report save failures;
  start/stop/mode use persistence-aware controls; deletion uses managed engine.
- retention_snapshot.go: setters validate and mutate under policy/registry locks,
  release before handler save. Policy can take effect before persistence succeeds;
  this review does not activate cleanup or certify durable policy transactions.
- camera_registration.go / go2rtc_scan.go: production manual add and scan use
  serialized registration with save. applyScanStreams without persistence exists
  separately; no claim is made that every possible future caller saves.
- events.go: event clearing mutates then saves. Trigger may queue capture before
  metadata save; HTTP failure is not proof capture never started.
- camera_deletion.go: stopped-intent save before removal and final save after
  unregister; final failure can leave memory removed with stopped registration
  on disk. Camera deletion's complete closeout remains section 2.
- shutdown.go / shutdown_completion.go: drain then final save; started I/O may
  outlive the deadline; preserve recording intent rather than equating shutdown
  with explicit Stop.

This is an inventory of reviewed mutation paths, not a repository-wide automated
attestation that no other writer exists. Finish source-wide reference inventory
before issuing final closure. Auth/TLS and event manifests are separate stores.

## Existing tests credited, not duplicated

persistence_test.go: concurrent saves, valid JSON/permissions, corrupt-primary
preservation, backup refusal, path validation and invalid JSON.
Recovery-entry and save-recovery suites: missing/dangling primary/backup handling.
persistence_primary_type_test.go: nonregular reads and regular symlink policy.
recording_control_persistence_test.go: mode/admission/save and shutdown semantics.
storage_failure_integration_test.go: real isolated tmpfs byte/inode exhaustion
and read-only errors. These are skipped in normal runs and need their dedicated
namespace job results, not inference from a passing core test package.

## Prepared missing deterministic boundary coverage

Production atomicWrite delegates to per-call filesystem operations with the same
create/write/sync/close/rename/open-directory/sync sequence. saveAt delegates to a
per-call writer, retaining its dedicated lock. No mutable global hooks. A short
write without error now explicitly returns io.ErrShortWrite before rename.

Nine atomic-write cases: create, partial write error, short write without error,
file sync, file close, rename, directory open, directory sync, and success.
Three save cases: failed backup prevents primary write; failed primary preserves
rotated previous bytes; post-rename sync error reports failure with new valid
primary visible and previous primary in backup. All use disposable fixtures.
These are injected failures, not power-loss simulations or live storage evidence.

## Exact-candidate evidence needed

For 4260b0647a551d44de06410a60002567a83461c4 obtain the core artifact named
core-test-execution-4260b0647a551d44de06410a60002567a83461c4-<attempt>.
Inspect run.json and go-test.jsonl; verify matching head/command and successful
run/pass events, including persistence tests and TestEventStorageSetup* cases.
The existing core_ci_evidence.py assessor explicitly requires recovery contracts
and fuzz seeds, NOT persistence-suite execution. Its passed assessment alone is
insufficient. Review separate build/vet and isolated-storage step results.

If the prepared boundary changes are pushed, repeat the applicable checks on the
new SHA. A previous green candidate is baseline evidence, not new-code execution.
GitHub source tools currently do not expose Actions logs/artifact retrieval.
Supply the artifact and step logs by upload or an accessible artifact source.
No local Go/test execution claimed; no section closure, merge or deployment.
