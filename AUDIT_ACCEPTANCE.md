# Recording-safety audit acceptance gates

Keep fix/recording-safety-audit unmerged. Passing CI or successfully pushing a batch does not clear the audit.

## Current evidence

User-reported Safety verification is green through 4339ff3d8e99a7df2a1ce16fd3fe69896f15888f (4339ff3), including the private-tmpfs step. Actions logs were not independently inspected. Earlier reported-green milestones include recovery inspection (1cff006), publication (607bf98), HTTPS exports (117ac28), forced render cancellation (f0c2cb7), initial storage failures (2074785), queued checkpoint preservation (45a6083), active-recorder/full-server shutdown (9137668) and runtime storage/non-root failures (bdc1ef5).

Historical preparation-time nonexecution remains historical. Node/stub, native filesystem/media and Go CI evidence are distinct; focused docs retain their exact scope. The export-phase preparation ran native probe/render shims, not Go; subsequent green for 4339ff3 was user-reported. Local Chromium evidence remains scoped to six controlled export UI/DST checks, routed responses and automated saving, not full application/browser playback or end-user save dialogs. See EXPORT_PHASE_VALIDATION.md and tests/evidence/export-browser-bdc1ef5.json.

The private-media review inspected auth.go, handlers.go, cleanup.go, go2rtc_scan.go, recording_publication.go and shutdown.go at 4339ff3. Authentication already intercepted /recordings/ before the legacy playback.go handler and restricted it to registered-camera MP4s; arbitrary-file exposure through that legacy handler was not established. The active handler did open a final entry for data before checking regular-file type, and private-media requests were absent from shutdown job tracking.

The prepared fix uses descriptor-relative Linux O_PATH/no-follow root, camera and leaf inspection, rejects nonregular entries before data opening, reopens the verified inode through procfs and checks identity. GET/HEAD/ranges remain; nested staging, root configuration and invalid paths remain unavailable. Media requests are now canceled/joined by shutdownGate. Tests cover range/HEAD, malformed paths, FIFO/symlink/directory rejection, inode replacement and gate joining. Existing authentication/session/event-bearer tests remain separate.

This explicitly requires real root/camera directories (no symlink-root/camera compatibility fallback) and working Linux procfs; open/inspection failures return 404. It does not make regular-file I/O interruptible or protect against in-place writes/hard links to other regular data. Registry snapshots are not camera-deletion leases. No evidence or retention policy changes. Preparation verified source blob hashes and ran only a separate native descriptor prototype: FIFO/symlink rejection, original-inode reading after replacement and camera-symlink refusal. Go/gofmt are unavailable; record exact-commit CI for this fix.

This is a focused security-review/remediation batch, not whole-branch clearance. main was observed at c5be6bf0cfea03071c7df1c3b0f5064753c881dd and is not modified. Final diff review, destructive paths/locking, manifest writer-reader semantics, publication source/in-place races, credential/proxy/SSRF and duplicate-name handling remain open. Runtime-image SIGTERM, live RTSP/restart, lost mounts/blocked I/O, deployment filesystem/procfs/no-replace compatibility, quota policy and remaining real-browser playback acceptance remain open. Neither CI nor readable partials authorize evidence deletion, expiry or reassembly.

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
