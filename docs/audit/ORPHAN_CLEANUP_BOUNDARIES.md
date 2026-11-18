# Manual orphan-cleanup boundaries

## Prepared against 19187ae

The user reported commit 19187ae12e7b8aebdb3a58481684bc4867ec1388 green on November 11, 2026. Actions logs were not independently inspected. This batch reviews the explicit POST /api/recordings/cleanup-orphaned path, not all destructive operations. Source review included cleanup.go/tests, handlers.go, main.go, recorder.go, persistence.go and shutdown.go. main remains unchanged and the audit branch must stay unmerged.

## Findings and changes

The old manual pass did not check request cancellation or the NVR shutdown flag. It inspected evidence only once before a Glob deletion pass, whose directory-read errors could be hidden. Its recovery guard recognized root .tmp entries and temp staging, but not nonempty buffer contents. That allowed associated published media to be removed in a buffer-only orphan directory, although buffer contents themselves were not recursively deleted. Hidden private/application directories were also eligible for scanning as if they were orphan camera directories.

The extracted orphan_cleanup.go retains the existing explicit top-level regular .mp4 deletion scope, including published partials, but adds these conservative boundaries:

- Reject a stable symlink/non-directory cleanup root. Hidden root directories, registered directories and root non-directories are skipped.
- Preserve a whole candidate directory on any root .tmp entry, nonempty temp or buffer, unexpected staging type, or inspection error. Empty staging directories are retained but do not prevent explicit deletion of top-level published media.
- Enumerate with ReadDir, report errors, recheck evidence before each unlink and before empty-directory handling, and count bytes/files only after successful removal. Concurrently absent files are not counted as failures.
- Check cancellation/shutdown between operations. Already completed removals remain completed. The JSON adds interrupted; success is false on interruption or other counted failures. Partial results remain HTTP 200; root inspection failures return fixed HTTP 500. Late shutdown admission returns 503 and non-POST returns 405.
- Remove an orphan directory only after observing it empty, never recursively. Failed attempted empty-directory removal is reported; a deliberately retained nonempty directory is not a failure.

recordingControlMu remains held across the request's filesystem operations and response encoding. NVR.mu is held only for the registered-directory snapshot, never for filesystem work. The existing shutdown gate joins mutation handlers; this batch does not change gate behavior. The HTTP wrapper additionally checks NVR.shuttingDown during an already admitted pass.

## Tests and evidence limits

Six new test functions cover buffer contents/types and hidden directories, newly appearing empty .tmp evidence between unlinks, injected root/directory/inspection/unlink/empty-directory failures, cancellation before and during deletion, stable symlink roots and nonregular entries, and partial shutdown JSON/registry availability/method refusal. Existing cleanup preservation tests remain applicable. Fault seams are per-call, never mutable global hooks. Injection proves control-flow/error accounting, not real permission enforcement or interruption of kernel I/O.

Preparation verifies the original cleanup.go Git blob, separates the unchanged periodic cleanup suffix, builds exact payload/diff/hash artifacts, and checks patch round trips. No Go/gofmt/compiler/parser is available locally; no build, vet, race or Go test execution is claimed. Exact-candidate CI remains required. User-reported green for the base is not validation of this draft.

## Still open

Pathnames can be replaced after inspection, including parent/root/staging entries. Evidence rechecks are observations, not atomic ownership leases. No hostile-filesystem race guarantee is claimed. Directory enumeration and repeated evidence checks remain unbounded and can be expensive; filesystem calls, global control-lock acquisition and response writes remain potentially blocking. Camera addition is not serialized with manual cleanup, so its directory creation/registration window still requires review. Recursive camera deletion, recovery writer/reader semantics, startup recovery, persistence ordering, actual lost mounts, live RTSP/container operation and full browser acceptance remain separate gates. This batch authorizes no staging deletion, expiry, reassembly, automatic recovery, merge or push by itself.
