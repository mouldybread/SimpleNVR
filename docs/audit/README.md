# Recording-safety audit

Start with the [final closeout ledger](AUDIT_FINAL_CLOSEOUT.md). It distinguishes
source review, exact-commit test evidence, accepted restrictions, and deferred
live-camera and deployment-storage qualification.

- [Acceptance criteria and historical results](AUDIT_ACCEPTANCE.md)
- [Closeout matrix](AUDIT_CLOSEOUT_MATRIX.md)
- [Persistence verdict](PERSISTENCE_CLOSEOUT_VERDICT.md)
- [Camera deletion safety](CAMERA_DELETION_SAFETY.md)
- [Recording lifecycle](RECORDING_LIFECYCLE.md)
- [Recovery and retained evidence](EVENT_RECOVERY.md)

The remaining notes are historical or focused engineering records. Earlier
branch instructions, open items, and failures are retained for context; use the
final ledger to interpret them rather than treating them as current status.

## Statements superseded by the day-folder work

The recording layout changed after some of these notes were written. All 95 notes
in this directory were read against that work for checkpoint 9 of issue #11; the
ones below contain statements it superseded. Read them as history and use the
documents named here for current behavior. The dated records themselves are not
edited. Notes not listed here had nothing to supersede.

### Timeline, playback and exports

- [TIMELINE_API.md](TIMELINE_API.md): probed video durations, the 8192-entry scan
  limit, the 2048-result duration cache, and an unreadable file blocking selection.
  Timeline and export selection now use stamped file names without probing, with
  limits of 400 day folders, 8,192 entries per folder and 20,000 candidates per
  call, and the timeline skips and reports an unreadable recording; see
  [recording layout](../RECORDING_LAYOUT.md).
- [PLAYBACK_CONTINUATION.md](PLAYBACK_CONTINUATION.md): interval ends are called
  "metadata end". They now come from the stamped file name; see
  [recording layout](../RECORDING_LAYOUT.md).
- [EVENT_DURATION_EVIDENCE.md](EVENT_DURATION_EVIDENCE.md): refers to the timeline
  and export duration probe and its cache. Both were removed.
- [EXPORT_PHASE_VALIDATION.md](EXPORT_PHASE_VALIDATION.md): lists a duration
  discovery cancellation case and a duration cache test. Both were removed with the
  cache; the profile inspection and render cancellation cases remain.

### Retention, deletion and cleanup

- [RETENTION_DELETION_ENGINE.md](RETENTION_DELETION_ENGINE.md),
  [RETENTION_POLICY.md](RETENTION_POLICY.md),
  [PUBLISHED_DELETION_ENGINE.md](PUBLISHED_DELETION_ENGINE.md),
  [PUBLISHED_DELETION_ADMISSION.md](PUBLISHED_DELETION_ADMISSION.md),
  [REGISTRY_IO_BOUNDARIES.md](REGISTRY_IO_BOUNDARIES.md) and
  [BACKGROUND_CLEANUP.md](BACKGROUND_CLEANUP.md): selection described as top-level
  or root `.mp4` files only, expiry by start prefix, and in places that the
  directory is never removed. Retention and explicit deletion now work through UTC
  day folders, expire a valid stamped name by its end, and remove emptied day
  folders; see [retention selection](../RETENTION_SELECTION.md).
- [CAMERA_DELETION_SAFETY.md](CAMERA_DELETION_SAFETY.md): top-level `.mp4` files
  only, with at most 8192 direct entries. Camera deletion now accepts valid stamped
  day folders under a 400-day bound; see [orphan day cleanup](../ORPHAN_DAY_CLEANUP.md).
- [ORPHAN_CLEANUP_BOUNDARIES.md](ORPHAN_CLEANUP_BOUNDARIES.md) and
  [ORPHAN_CLEANUP_INSPECTION_LIMITS.md](ORPHAN_CLEANUP_INSPECTION_LIMITS.md): a Glob
  pass, unbounded enumeration, repeated whole-camera rechecks, and a top-level scope
  "not broadened". Orphan cleanup now descends one day level under shared read and
  entry budgets with batched rechecks; see [orphan day cleanup](../ORPHAN_DAY_CLEANUP.md).
- [MEDIA_OWNERSHIP_ADMISSION.md](MEDIA_OWNERSHIP_ADMISSION.md) and
  [AUDIT_FINAL_CLOSEOUT.md](AUDIT_FINAL_CLOSEOUT.md): name `cleanupOldRecordings*`
  and an 8192-entry, no-recursion retention engine. The glob helper was test-only
  and has been removed; see [retention selection](../RETENTION_SELECTION.md).

### Publication, recovery and lifecycle

- [RECORDING_LIFECYCLE.md](RECORDING_LIFECYCLE.md): root `.mp4` retention and
  delete-all, startup recovery publishing into the camera root, and orphan cleanup
  not descending. Recordings are now published into UTC start-day folders and
  retention, delete-all, recovery and orphan cleanup handle them; see
  [recording layout](../RECORDING_LAYOUT.md).
- [RECORDING_PUBLICATION.md](RECORDING_PUBLICATION.md): continuous capture and
  startup recovery reach the original public publisher. They now publish through
  the stamped day-folder publisher.
- [EVENT_RECOVERY_INSPECTION.md](EVENT_RECOVERY_INSPECTION.md): only root output
  names are observed. A stamped event output is now looked up in its UTC start-day
  folder.

### Disk usage

- The disk usage figure on the camera list is no longer a walk of the whole camera
  tree; see [disk usage](../DISK_USAGE.md).

For operation and setup, return to the [project README](../../README.md).
