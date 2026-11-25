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

The recording layout changed after some of these notes were written. Read them as
history and use the documents named here for current behavior.

- [TIMELINE_API.md](TIMELINE_API.md): probed video durations, the 8192-entry scan
  limit and the 2048-result duration cache describe the earlier flat layout.
  Timeline and export selection now use stamped file names without probing; see
  [recording layout](../RECORDING_LAYOUT.md).
- [RETENTION_DELETION_ENGINE.md](RETENTION_DELETION_ENGINE.md): selection limited
  to top-level files. Retention now also inspects UTC day folders and skips those
  after the cutoff day; see [retention selection](../RETENTION_SELECTION.md).
- [ORPHAN_CLEANUP_BOUNDARIES.md](ORPHAN_CLEANUP_BOUNDARIES.md): flat directories and
  unbounded enumeration. Orphan cleanup now descends one day level under shared read
  and entry budgets; see [orphan day cleanup](../ORPHAN_DAY_CLEANUP.md).
- The disk usage figure on the camera list is no longer a walk of the whole camera
  tree; see [disk usage](../DISK_USAGE.md).

Other notes about deletion, orphan cleanup, storage status, retention policy and
playback continuation were not reviewed against these changes and may contain
similar statements.

For operation and setup, return to the [project README](../../README.md).
