# Recording-safety audit acceptance gates

Keep fix/recording-safety-audit unmerged. Passing CI or successfully pushing a batch does not clear the audit.

## Current evidence

User-reported Safety verification is green through 2074785f3fb924f8a8b191903ce314cc2cdc4999 (2074785), including the mandatory isolated tmpfs storage-failure step. Results were reported by the user, not independently inspected Actions logs. Later reported-green batches after 26a7bc0 include recovery inspection (1cff006), no-replace publication (607bf98), HTTPS export lifecycle (117ac28), forced render cancellation (f0c2cb7) and isolated storage failures (2074785).

Historical preparation-time nonexecution remains historical: the 26a7bc0 preparation ran full-script Node syntax and failure/lifecycle stub suites in UTC and America/Chicago, not Go or browser integration. Subsequent user-reported CI results do not turn local preparation into local Go execution.

The queued-event preservation fix retains initialized recovery staging on cancellation or queue refusal even without prebuffer pins. Regression coverage checks retained checkpoints, released capacity and pinned-input preservation; this preparation has not run Go compilation, gofmt, vet or race tests because Go tooling is unavailable locally. Record CI evidence for its resulting exact commit before treating it as verified. It does not validate active-recorder RTSP interruption, full-server shutdown, production filesystems or real browsers. Broader gates remain unchecked.

Scoped export and storage tests do not close broader acceptance: HTTPS application-shutdown fixtures used server=nil; forced renderer tests used SIGSTOP; tmpfs tests are not deployment-filesystem or power-loss tests. Ordinary graceful render cancellation, probe cancellation, full HTTP-server/runtime-image shutdown with active recorders, remaining runtime storage failures and real-system/browser validation remain outstanding. Re-record evidence for each exact committed SHA; never carry a prior green result forward.

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
