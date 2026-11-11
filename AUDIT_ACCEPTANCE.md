# Recording-safety audit acceptance gates

Keep fix/recording-safety-audit unmerged. Passing CI or successfully pushing a batch does not clear the audit.

## Current evidence

User-reported Safety verification is green through 375c5803a612d227f01c0a5071762fc506c0454f (375c580), including the private-tmpfs step. Actions logs were not independently inspected. Earlier reported-green milestones include recovery inspection (1cff006), publication (607bf98), HTTPS exports (117ac28), forced render cancellation (f0c2cb7), initial storage failures (2074785), queued checkpoint preservation (45a6083), active-recorder/full-server shutdown (9137668), runtime storage/non-root failures (bdc1ef5), export-phase cancellation (4339ff3) and private-media descriptor/shutdown handling (c9358ba).

Historical preparation-time nonexecution remains historical. Node/stub, native filesystem/media and Go CI evidence are distinct; focused docs retain their exact scope. The export-phase preparation ran native probe/render shims, not Go; subsequent green for 4339ff3 was user-reported. Local Chromium evidence remains scoped to six controlled export UI/DST checks, routed responses and automated saving, not full application/browser playback or end-user save dialogs. See EXPORT_PHASE_VALIDATION.md and tests/evidence/export-browser-bdc1ef5.json.

The private-media review inspected auth.go, handlers.go, cleanup.go, go2rtc_scan.go, recording_publication.go and shutdown.go at 4339ff3. Authentication already intercepted /recordings/ before the legacy playback.go handler and restricted it to registered-camera MP4s; arbitrary-file exposure through that legacy handler was not established. The active handler did open a final entry for data before checking regular-file type, and private-media requests were absent from shutdown job tracking.

The prepared fix uses descriptor-relative Linux O_PATH/no-follow root, camera and leaf inspection, rejects nonregular entries before data opening, reopens the verified inode through procfs and checks identity. GET/HEAD/ranges remain; nested staging, root configuration and invalid paths remain unavailable. Media requests are now canceled/joined by shutdownGate. Tests cover range/HEAD, malformed paths, FIFO/symlink/directory rejection, inode replacement and gate joining. Existing authentication/session/event-bearer tests remain separate.

This explicitly requires real root/camera directories (no symlink-root/camera compatibility fallback) and working Linux procfs; open/inspection failures return 404. It does not make regular-file I/O interruptible or protect against in-place writes/hard links to other regular data. Registry snapshots are not camera-deletion leases. No evidence or retention policy changes. Preparation verified source blob hashes and ran only a separate native descriptor prototype: FIFO/symlink rejection, original-inode reading after replacement and camera-symlink refusal. Go/gofmt were unavailable; subsequent exact-commit CI green for c9358ba was user-reported, not local Go execution.

This is a focused security-review/remediation batch, not whole-branch clearance. main was observed at c5be6bf0cfea03071c7df1c3b0f5064753c881dd and is not modified. Final diff review, destructive paths/locking, manifest writer-reader semantics, publication source/in-place races, credential/proxy/SSRF and duplicate-name handling remain open. Runtime-image SIGTERM, live RTSP/restart, lost mounts/blocked I/O, deployment filesystem/procfs/no-replace compatibility, quota policy and remaining real-browser playback acceptance remain open. Neither CI nor readable partials authorize evidence deletion, expiry or reassembly.

## Live-proxy review batch (user-reported green at 375c580)

Preparation base is c9358ba5cda8679cbcb643f07f2989ed24fd3cbf. Its validation was still pending during draft preparation; the user subsequently reported it green. The live-proxy batch was subsequently pushed as 375c580 and user-reported green. Its preparation-time nonexecution is historical, not a claim that CI was independently inspected. No recordings, retention, recovery or deletion policy changes were made.

The live proxy previously selected the first duplicate name encountered, fell back to go2rtc for malformed stored URLs, and relayed arbitrary upstream statuses/content types/bodies. The draft rejects duplicate names with 409 and invalid/non-RTSP sources with 503 before outbound access, derives only the fixed HTTP port-1984 stream endpoint from the stored host, and never forwards RTSP credentials or caller-selected target URLs. It requires 200/206 video/mp4 responses, substitutes fixed errors for unexpected responses, and preserves only validated unsatisfied-range metadata for sanitized 416 errors. Existing request/response header allowlists, no-redirect policy and cancellation remain.

Tests add selection/URL/header/session-token cases plus real loopback HTTP media-type/status/redirect cases. The old stalled-body fixture now declares video/mp4 so its cancellation path is still exercised. See LIVE_PROXY_BOUNDARIES.md for compatibility restrictions and remaining SSRF/trust limits. Preparation used source review only: Go/gofmt and a Go parser were unavailable locally; no local Go execution, new browser validation, live go2rtc validation or whole-branch clearance was claimed. Subsequent exact-commit green for 375c580 was user-reported; live go2rtc compatibility and whole-branch clearance remain open.

## Prepared registry-I/O lock-boundary batch (not yet validated)

Base: 375c5803a612d227f01c0a5071762fc506c0454f, user-reported green. Recorder start, restart-resume and both recording-mode admission checks now release NVR.mu around storage inspection, then revalidate state/shutdown before launching. Mode changes interrupted during replacement admission restore restart intent without launching a late worker. recordingControlMu continues to serialize recording controls. Bulk published-recording deletion snapshots directories before filesystem work, serializes with recording control, reports read/unlink failures, counts only successful-unlink bytes, and skips stable symlinks/nonregular entries. Root staging and nested event/buffer inputs are not traversed; published partials remain within explicit delete-all scope.

The new tests use per-call admission/deletion injection to pause phases and assert registry read/write availability and recorder joining; they do not reproduce an indefinitely blocked kernel Statfs. Additional fixtures exercise staging/symlink preservation, partial accounting, missing-directory errors, cancellation and shutdown/method refusal. Go/gofmt remain unavailable locally; these new tests have not run. See REGISTRY_IO_BOUNDARIES.md. No mount-loss, live RTSP/container or whole-branch gate is closed. Pathname replacement races, orphan-cleanup review, global control/policy locks held across some filesystem operations, directory enumeration bounds and recovery writer/reader review remain open.

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
