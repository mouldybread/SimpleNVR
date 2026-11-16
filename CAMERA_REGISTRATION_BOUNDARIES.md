# Serialized manual and scanned camera registration

Preparation base: 974e2fd09419db8f5faa6afbbbfbd7442dfc2bb3 on fix/recording-safety-audit.
974e2fd is green by user report, not an independently reviewed Actions result.
Main remains c5be6bf0cfea03071c7df1c3b0f5064753c881dd. Keep the audit branch unmerged.

## Existing gap addressed

ORPHAN_CLEANUP_BOUNDARIES.md explicitly left camera directory creation/registration
serialization open. At the preparation base, addCameraHandler used MkdirAll before
registry insertion without recordingControlMu, whereas serveOrphanCleanup held
that mutex across its registry snapshot and filesystem pass. A newly created empty
camera directory could therefore be observed as unregistered and removed by a
concurrent explicit orphan-cleanup request. No actual recording loss is claimed.

Scan application already held recordingControlMu, but also held NVR.mu across
Mkdir and failed-batch directory removal. Slow storage could block registry access.
Both registration handlers persisted after their registration helper returned,
without maintaining registration serialization through the save.

The candidate addresses these current-source findings, not a previously fixed
retention failure. It does not run or enable orphan cleanup.

## Shared registration boundary

camera_registration.go snapshots existing registry identities/source URLs under a
brief read lock, then creates directories under recordingControlMu with NVR.mu
released. The commit takes NVR.mu briefly, rechecks cancellation/shutdown and the
registry snapshot, and registers the complete pending set. Persistence follows
without NVR.mu but while recordingControlMu remains held. No network or recorder
worker lifetime is moved under a registry lock.

Both manual add and the production scan callback use this boundary through save.
The old applyScanStreams registry-only compatibility method remains for existing
callers/tests; the production scan handler explicitly uses applyScanStreamsAndSave.
No second finishCameraUpdate save runs afterward. Rendering happens after the
registration operation releases its control lock.

- Manual additions retain duplicate-URL behavior; scans deduplicate existing URLs
  and duplicate URLs in the returned batch.
- New cameras remain stopped, continuous-mode, seven-day-retention registrations;
  this batch does not start recorders or change retention policy.
- IDs are basename-checked and cannot replace existing registry IDs or pending IDs.
  Mkdir is exclusive: existing directories/symlinks are refused, not adopted by
  MkdirAll. The configured recordings root must already exist.
- Before registry commit, cancellation, shutdown, creation failure or stale registry
  observation refuses the batch without inserting a partial set of new cameras.
- Newly created directories survive pre-commit failure for inspection. In particular,
  the old scan failure rollback no longer performs pathname-based removal. This can
  leave empty unregistered directories; it is a conservative preservation tradeoff,
  not new evidence that they may be deleted.
- After commit, persistence is attempted even on late request cancellation. A failed
  save returns a distinct committed-state error and does not pretend to roll back
  registry entries or remove created directories. Persistence errors take HTTP 500
  precedence even if the wrapped cause is context cancellation.
- Manual HTTP requests reject wrong methods and malformed forms before registration.
  Existing outer authentication/body-size/CSRF policy remains unchanged. HTTP errors
  use fixed messages rather than exposing private filesystem/persistence details.

## Regression candidate: ten functions, 40 cases

- Two deterministic creation/save versus real orphan-handler cases (manual and scan
  deduplication policies), with registry-lock availability checked at both blocked
  phases. The cleanup handler cannot enumerate until registration/save completes,
  then observes the new camera and leaves its directory intact.
- Nine interrupted/failing batch cases: second creation failure, cancellation,
  shutdown, registry replacement/addition, camera ID/path/source changes and
  persistence failure. Created directories survive;
  pre-commit failures insert no pending cameras; save failure retains committed state.
- Five identity/directory cases: registered ID collision, existing directory,
  existing symlink, duplicate pending ID and invalid generated ID.
- Two duplicate-URL policy cases and two existing-URL/no-op scan cases.
- Eight manual HTTP cases, including save failure wrapping context.Canceled.
- One loopback scan plus real registration/save-failure response case: committed
  state is retained and failure is HTTP 500, not misreported as upstream HTTP 502.
- Three post-commit cancellation/shutdown/save-failure cases.
- Seven invalid-input cases proving no create/save call.
- One canceled control-lock waiter case: acquisition itself is not interruptible,
  but cancellation is rechecked after acquisition before filesystem work.

Fault seams are per call, with disposable roots and real directory creation. Test
persistence callbacks observe ordering or inject failures; they do not qualify the
actual /recordings camera JSON writer or power-loss durability. Existing persistence
and authenticated route tests remain separate and required. No global hooks, root
mount manipulation, live cameras or existing recordings are used.

## Review and remaining limits

Source review also inspected auth.go/auth_test.go, live_proxy.go/live_proxy_test.go,
playback.go, frontend_security_test.go, main.go, go2rtc_scan.go/tests,
handlers.go, orphan_cleanup.go and persistence.go at the base. Authentication,
credential stripping, fixed-target proxy selection, response filtering and frontend
rendering behavior are unchanged; this is not whole-security or frontend clearance.

recordingControlMu can still be held across slow creation/persistence; its acquisition
and kernel filesystem calls remain potentially blocking. The fix specifically avoids
holding NVR.mu across those operations and serializes cooperating application callers.
It does not protect against hostile external ancestor/path replacement, lost mounts,
hard links or unrelated processes. Snapshot checks are observations, not filesystem
leases. Registration snapshots/enumeration scale with the existing camera registry.

This candidate supersedes only the creation/registration window and scan registry-I/O
scope described above. Other limitations in ORPHAN_CLEANUP_BOUNDARIES.md remain.
The 15 skipped core events, whole-branch acceptance reconciliation, live Dahua/RTSP
and intended-storage qualification remain open. No new audit checkbox is cleared.

Exact-candidate Safety verification is the execution gate. Existing full race/vet,
storage, export, both Chrome attempts and correspondence gates/timeouts are unchanged.
No comparator activation, automatic recovery, staging expiry, evidence deletion,
cleanup/retention activation, merge or deployment is authorized.
