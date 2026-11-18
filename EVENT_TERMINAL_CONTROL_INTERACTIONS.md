# Published-event terminal ownership versus controls and retention

Preparation base: 6dafb614b95c4077d977fb14be4c8b80e62bbd29.
Branch: fix/recording-safety-audit. Main remains c5be6bf0cfea03071c7df1c3b0f5064753c881dd.
This is an integration-regression candidate, not production remediation or whole-audit clearance.

## Why this batch

Existing event_terminal_lease_test.go verifies publication-to-terminal lease transfer,
independent terminal metadata budgets, checkpoint failure and media reentry refusal.
Existing camera_deletion_ownership_test.go verifies stop/join-before-admission with
controlled workers and injected callbacks. Those are implemented, not missing.
The new tests exercise the intersection with an actual admitted and published event
job, not merely another unrelated holder of the global media gate.

No new production defect was established in the reviewed interaction paths. Do not
change production behavior merely to create a remediation commit.

## Candidate coverage: 22 table-driven cases

- Fourteen control cases: seven actions times successful/failed terminal checkpoint.
  Actions are API stop, disable, replacement start, replacement admission failure,
  camera deletion refused by retained staging, stop-persistence failure preventing
  camera deletion, and process shutdown preserving restart intent.
- Two failed-shutdown cases: caller cancellation and an already-expired deadline
  while terminal retention is blocked. Failure must return without persistence;
  allowing the worker to finish afterward must not resurrect a save coordinator.
- Six managed-retention cases: unchanged state is the positive real-deletion control;
  changed policy, replaced camera, changed directory, shutdown and canceled admission
  must refuse stale deletion after waiting for the published event's terminal owner.

Every fixture uses TriggerEvent, actual owned input pinning, handleEvent, real
creator-bound capture/concat handling, durable publication and owned checkpoints.
The FFmpeg/FFprobe child scripts produce synthetic bytes/results. They do NOT
establish valid media, uninterrupted RTSP capture, ingestion-time completeness,
real recorder latency or deployment durability.

Controls run against a local NVR registry and a controlled top-level worker whose
WaitGroup owns actual releaseJob. Persistence callbacks observe ordering or inject
failure; they do not exercise the actual camera-registry JSON writer. Blocking is injected only around the terminal
checkpoint writer, after real publication transferred its lease. Tests check that
saves/replacements/removals cannot cross this boundary, NVR.mu remains available,
descriptors and event capacity close/return, and checkpoint failure latches without
replacing the previous persisted checkpoint. That last assertion concerns the
injected pre-delegation failure, not all post-rename durability failures.

Filesystem snapshots compare all non-checkpoint names, modes, lengths and hashes.
A healthy terminal checkpoint advances once without changing processing state;
a failed write preserves its prior bytes. Camera deletion uses the real remover
and must refuse nonempty retained staging before deleting any file. Retention's
positive case invokes the real leased expiry/deletion engine on one explicitly
created disposable old MP4; event staging and current published output survive.
That positive test is NOT permission to run cleanup against user recordings.

The retention wrapper records actual entry; runRetentionDeletionLeased's boolean
alone is not accepted as proof of successful filesystem deletion. The test also
checks the old fixture is actually gone and the remaining evidence is unchanged.

## Current-source review performed

Reviewed recorder.go, event_job_inventory.go, event_publication_checkpoint.go,
event_release_revalidation.go, event_staging_input_release.go,
published_deletion_engine.go, camera_deletion.go, lifecycle.go,
recording_control_persistence.go, retention_snapshot.go, media_mutation_ownership.go,
shutdown.go and shutdown_completion.go, alongside their relevant existing tests.

- Managed camera deletion stops and joins before media admission and persists the
  stop before the first destructive callback; no registry lock spans that join.
- Publication transfers the same held lease through terminal retention, even after
  the finalization context ends; terminal retention receives its own grace budget.
- Terminal release retains staging unconditionally and closes ownership/descriptors
  and returns capacity. The opt-in staging release primitive has no production
  caller in the inspected event release path.
- Managed retention acquires media ownership before policy/registry checks, then
  revalidates its snapshot. File preflight is bounded; stat/unlink still assumes
  trusted directory ownership and is not an atomic compare-and-unlink primitive.
- Shutdown keeps restart intent, and failed joining does not permit final save.
  Persistence already entered and kernel-blocked I/O remain separate limitations.

These are scoped observations, not a claim that all call sites/security boundaries
or filesystem races have been reviewed. Authentication/proxy/frontend interactions,
remaining destructive paths, live Dahua/RTSP and intended-storage qualification stay
open in AUDIT_CLOSEOUT_MATRIX.md. The 15 skipped events from the reviewed core
assessment still need raw-transcript reconciliation; none is silently cleared here.

## Qualification and unchanged authority

The three new Go integration tests are discovered by the existing full race suite;
no workflow, timeout, skip gate, media argument or dependency change is proposed.
Actions on the exact new candidate is the execution gate. Earlier green CI and the
reviewed 6dafb61 summaries are not results for these new tests.

No production source change; no comparator activation, automatic recovery, staging
expiry, cleanup/retention activation, deletion authority, merge or deployment.
