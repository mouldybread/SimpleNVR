# Recording-safety audit acceptance gates

Keep fix/recording-safety-audit unmerged. Passing CI or successfully pushing a batch does not clear the audit.

## Current status reconciliation — November 16, 2026

Latest user-reported green: 6d64ed803a2a314b1489f3f15bf3c48309bb6c45.
The user also reported a7f9f6618c992fd201421fc492032fc19d6eb37e green.
These are user-reported CI results, not independently downloaded Actions results
or numerical resource acceptance. The chronological sections below remain
historical snapshots; old pending wording does not prove current absence.
No original acceptance checkbox is automatically cleared by this reconciliation.

Integration baseline for this final recovery-contract/evidence write:
623bbe16564aed009735e1a0bd8aa820490eaa61, preceded by the helper commit
46386d532b82e64aab81ca8451fb663747d9b46d. Their runtime integration and the
remaining six recovery-contract/evidence files form the prepared 13-file batch.
No exact-final-head CI result is known at preparation. Main remains
c5be6bf0cfea03071c7df1c3b0f5064753c881dd and is not changed.

The original preparation base a9adc67f7e09a90ef1e42474b3b25c6403ff86ae failed
according to the user after all 22 original tooling regressions passed. The
user-supplied report for that exact head was read: whole-copy slow-reader success/
decode passed; the next case failed to recognize sustained backpressure.
Disconnect/reuse and both SIGTERM gates were not cleared. 6d64ed8 predates mandatory
execution of that campaign, so its green does not supply runtime-container export
acceptance. The original failure remains historical evidence, not a new result for
the corrected candidate.

The combined batch corrects the test observer's leader-thread/process-wide write-
counter assumptions, adds safe console diagnostics and regressions, and includes
all five recovery-manifest contract draft files. Its 48 Python tooling methods
passed in preparation; corrected runtime-container and new Go/race qualification
remain pending Actions. No production source defect or successful corrected
runtime campaign is claimed. See RUNTIME_EXPORT_THREAD_OBSERVER_FIX.md and
RECOVERY_MANIFEST_CONTRACT_ACCEPTANCE.md. The original runtime-export scope is
recorded in RUNTIME_EXPORT_BACKPRESSURE_ACCEPTANCE.md; the thread/counter correction
note supersedes its initial observer assumptions, not its acceptance gates.

Already implemented scoped evidence includes authenticated Go HTTPS slow-reader,
backpressure/disconnect/shutdown and pre-header probe/render cancellation; active
continuous/event-buffer runtime-container capture, SIGTERM and restart intent;
native Chrome control, metadata/autoplay/end/failure/explicit-recovery acceptance;
and correspondence interruption/readiness/teardown/retry plus all-four-report
assessment. a7f9f66 adds native late-transfer TLS failure/recovery with its explicit
separate-source injection boundary. These implementations are not recounted as
absent, independently reviewed artifact results, or deployment qualification.

The required workflow retains core/race/vet/storage/lifecycle, both isolated Chrome
attempts and correspondence resources. Comparator experiments remain test-only.
Actual same-head numerical resource review, intended-host fit, positive ingestion-
time coverage and resource-cost acceptance remain unqualified. Historical reports
need their own compatible same-commit assessor/schema. The committed coverage-
retention fix remains intact; no retention/date/environment workaround is proposed.

Remaining original-audit gates: exact-candidate validation and artifact review;
remaining bounded synthetic/browser/timeline cases after current-test reconciliation;
whole-branch writer/reader, destructive-path, lock/resource/security/publication/
persistence review; live Dahua/RTSP/reconnect/interrupted acquisition; deployment
filesystem/mount/procfs/no-replace/blocked-I/O/quota/durability and intended-host
pipeline cost/recorder-latency qualification; and final-candidate reconciliation.
Existing implementations are neither absent nor complete deployment acceptance.
Automatic recovery, staging expiry, cleanup activation, evidence deletion, merge
and deployment remain unauthorized.

## Historical evidence through 19187ae

User-reported Safety verification is green through 19187ae12e7b8aebdb3a58481684bc4867ec1388 (19187ae), including the private-tmpfs step. Actions logs were not independently inspected. Earlier reported-green milestones include recovery inspection (1cff006), publication (607bf98), HTTPS exports (117ac28), forced render cancellation (f0c2cb7), initial storage failures (2074785), queued checkpoint preservation (45a6083), active-recorder/full-server shutdown (9137668), runtime storage/non-root failures (bdc1ef5), export-phase cancellation (4339ff3) and private-media descriptor/shutdown handling (c9358ba).

Historical preparation-time nonexecution remains historical. Node/stub, native filesystem/media and Go CI evidence are distinct; focused docs retain their exact scope. The export-phase preparation ran native probe/render shims, not Go; subsequent green for 4339ff3 was user-reported. Local Chromium evidence remains scoped to six controlled export UI/DST checks, routed responses and automated saving, not full application/browser playback or end-user save dialogs. See EXPORT_PHASE_VALIDATION.md and tests/evidence/export-browser-bdc1ef5.json.

The private-media review inspected auth.go, handlers.go, cleanup.go, go2rtc_scan.go, recording_publication.go and shutdown.go at 4339ff3. Authentication already intercepted /recordings/ before the legacy playback.go handler and restricted it to registered-camera MP4s; arbitrary-file exposure through that legacy handler was not established. The active handler did open a final entry for data before checking regular-file type, and private-media requests were absent from shutdown job tracking.

The prepared fix uses descriptor-relative Linux O_PATH/no-follow root, camera and leaf inspection, rejects nonregular entries before data opening, reopens the verified inode through procfs and checks identity. GET/HEAD/ranges remain; nested staging, root configuration and invalid paths remain unavailable. Media requests are now canceled/joined by shutdownGate. Tests cover range/HEAD, malformed paths, FIFO/symlink/directory rejection, inode replacement and gate joining. Existing authentication/session/event-bearer tests remain separate.

This explicitly requires real root/camera directories (no symlink-root/camera compatibility fallback) and working Linux procfs; open/inspection failures return 404. It does not make regular-file I/O interruptible or protect against in-place writes/hard links to other regular data. Registry snapshots are not camera-deletion leases. No evidence or retention policy changes. Preparation verified source blob hashes and ran only a separate native descriptor prototype: FIFO/symlink rejection, original-inode reading after replacement and camera-symlink refusal. Go/gofmt were unavailable; subsequent exact-commit CI green for c9358ba was user-reported, not local Go execution.

This is a focused security-review/remediation batch, not whole-branch clearance. main was observed at c5be6bf0cfea03071c7df1c3b0f5064753c881dd and is not modified. Final diff review, destructive paths/locking, manifest writer-reader semantics, publication source/in-place races, credential/proxy/SSRF and duplicate-name handling remain open. Runtime-image SIGTERM, live RTSP/restart, lost mounts/blocked I/O, deployment filesystem/procfs/no-replace compatibility, quota policy and remaining real-browser playback acceptance remain open. Neither CI nor readable partials authorize evidence deletion, expiry or reassembly.

## Live-proxy review batch (user-reported green at 375c580)

Preparation base is c9358ba5cda8679cbcb643f07f2989ed24fd3cbf. Its validation was still pending during draft preparation; the user subsequently reported it green. The live-proxy batch was subsequently pushed as 375c580 and user-reported green. Its preparation-time nonexecution is historical, not a claim that CI was independently inspected. No recordings, retention, recovery or deletion policy changes were made.

The live proxy previously selected the first duplicate name encountered, fell back to go2rtc for malformed stored URLs, and relayed arbitrary upstream statuses/content types/bodies. The draft rejects duplicate names with 409 and invalid/non-RTSP sources with 503 before outbound access, derives only the fixed HTTP port-1984 stream endpoint from the stored host, and never forwards RTSP credentials or caller-selected target URLs. It requires 200/206 video/mp4 responses, substitutes fixed errors for unexpected responses, and preserves only validated unsatisfied-range metadata for sanitized 416 errors. Existing request/response header allowlists, no-redirect policy and cancellation remain.

Tests add selection/URL/header/session-token cases plus real loopback HTTP media-type/status/redirect cases. The old stalled-body fixture now declares video/mp4 so its cancellation path is still exercised. See LIVE_PROXY_BOUNDARIES.md for compatibility restrictions and remaining SSRF/trust limits. Preparation used source review only: Go/gofmt and a Go parser were unavailable locally; no local Go execution, new browser validation, live go2rtc validation or whole-branch clearance was claimed. Subsequent exact-commit green for 375c580 was user-reported; live go2rtc compatibility and whole-branch clearance remain open.

## Registry-I/O lock-boundary batch (user-reported green at 19187ae)

Base: 375c5803a612d227f01c0a5071762fc506c0454f, user-reported green. Recorder start, restart-resume and both recording-mode admission checks now release NVR.mu around storage inspection, then revalidate state/shutdown before launching. Mode changes interrupted during replacement admission restore restart intent without launching a late worker. recordingControlMu continues to serialize recording controls. Bulk published-recording deletion snapshots directories before filesystem work, serializes with recording control, reports read/unlink failures, counts only successful-unlink bytes, and skips stable symlinks/nonregular entries. Root staging and nested event/buffer inputs are not traversed; published partials remain within explicit delete-all scope.

The new tests use per-call admission/deletion injection to pause phases and assert registry read/write availability and recorder joining; they do not reproduce an indefinitely blocked kernel Statfs. Additional fixtures exercise staging/symlink preservation, partial accounting, missing-directory errors, cancellation and shutdown/method refusal. Go/gofmt were unavailable during preparation; no local Go execution was claimed. The batch was pushed as 19187ae and subsequently user-reported green on November 11, 2026; Actions logs were not independently inspected. See REGISTRY_IO_BOUNDARIES.md. No mount-loss, live RTSP/container or whole-branch gate is closed. Pathname replacement races, orphan-cleanup review, global control/policy locks held across some filesystem operations, directory enumeration bounds and recovery writer/reader review remain open.

## Prepared orphan-cleanup batch (not yet validated)

Base: 19187ae12e7b8aebdb3a58481684bc4867ec1388, user-reported green. The manual orphan-cleanup pass gains cooperative request-cancellation/shutdown checks, explicit directory-read errors, successful-unlink-only accounting and partial-result interrupted metadata. Hidden private/application directories are skipped. Nonempty or unexpected temp/buffer entries preserve the whole candidate directory; root .tmp preservation remains. Recovery evidence is rechecked before each unlink and empty-directory handling. A stable symlink cleanup root is rejected. No recursive removal is introduced.

Six new test functions cover preservation, newly appearing staging, injected errors, cancellation, stable symlink/nonregular boundaries and partial shutdown HTTP responses with registry availability. Original modified source files are blob-verified and patch round trips are checked; no Go test/build/vet/race execution is claimed. See ORPHAN_CLEANUP_BOUNDARIES.md. Pathname replacement races, camera-add creation/registration serialization, unbounded enumeration, blocked syscalls and long-held recordingControlMu remain open. This is a scoped destructive-path remediation draft, not whole-branch clearance or authorization to delete staging.

## Required before proposing a merge

- [ ] Exact candidate commit passes go vet ./... and go test -race -count=1 -timeout=120s ./..., with required media/frontend dependencies present rather than silently skipped.
- [ ] Recovery manifest writer/strict read-only reader integration is reviewed together; incomplete, unsupported and changed observations preserve uncertainty and never authorize deletion or reassembly.
- [ ] Runtime failure behavior is tested for exhausted bytes/inodes, permission failures, unavailable mounts and failed capture/assembly/publication/persistence. Admission observations are not accepted as runtime reservations or a full disk-full policy.
- [ ] Live Dahua/RTSP disconnect/reconnect and interrupted continuous/event capture/assembly are validated with retained evidence inspected.
- [ ] Active-recorder SIGTERM and in-flight export/inspection shutdown are tested; cancellation and joining precede final persistence, and timeout reports failure.
- [ ] Authenticated real-FFmpeg export rendering/streaming is tested with slow clients, disconnects, cancellation and shutdown.
- [ ] Real-browser playback covers late media errors, autoplay denial, explicit retry, paused-ended resume and metadata delay. Monitored-download tests distinguish browser handoff from actual disk saving.
- [ ] Final whole-branch review covers destructive paths, lock order/long-held locks, resource bounds, authentication, frontend text sinks, proxy/SSRF boundaries, media publication and persistence ordering.
- [ ] Older lifecycle/timeline/export summaries are reconciled with scoped source behavior and focused documentation; no completed work is counted twice and no validation gap is described as solved.

## Explicit scope decisions, not automatic feature additions

Audio-aware trimming/broader media profiles, synchronized multi-camera advancement, next-day fetching, gapless transitions, a storage/recovery UI and legacy event-ID migration can be deferred if original acceptance criteria permit. Automatic recovery, staging expiry and evidence deletion are not implicitly authorized by this checklist. Any future recovery write needs separate validated semantics and explicit approval.

## Final closeout pointer — November 18, 2026

See AUDIT_FINAL_CLOSEOUT.md for the final section ledger and verdict. Historical sections
and original checkboxes above are preserved unchanged. Live-camera/storage qualification is
deferred by the owner to after merge and is not claimed.
