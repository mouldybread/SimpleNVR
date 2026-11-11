# Recording-safety audit acceptance gates

Keep fix/recording-safety-audit unmerged. Passing CI or successfully pushing a batch does not clear the audit.

## Current evidence

User-reported Safety verification is green through 9137668e48e59ebee29bf9a44164e1cd0b160287 (9137668), including the mandatory isolated tmpfs storage-failure step. Results were reported by the user, not independently inspected Actions logs. Later reported-green batches after 26a7bc0 include recovery inspection (1cff006), no-replace publication (607bf98), HTTPS export lifecycle (117ac28), forced render cancellation (f0c2cb7), isolated storage failures (2074785) queued-event checkpoint retention (45a6083) and controlled active-recorder/full-server shutdown (9137668).

Historical preparation-time nonexecution remains historical: the 26a7bc0 preparation ran full-script Node syntax and failure/lifecycle stub suites in UTC and America/Chicago, not Go or browser integration. Subsequent user-reported CI results do not turn local preparation into local Go execution.

The queued-event preservation fix retains initialized recovery staging on cancellation or queue refusal even without prebuffer pins. Its preparation did not run local Go compilation, gofmt, vet or race tests; subsequent CI green for 45a6083 was reported by the user. These are separate evidence statements.

The active-recorder/full-HTTP-shutdown batch adds controlled real-FFmpeg tests with local-file input substitution, interrupted continuous/event acquisition, paced event assembly, explicit stop versus shutdown intent, actual authenticated HTTPS export backpressure and a non-nil listening server. See ACTIVE_RECORDER_VALIDATION.md for exact scope and persistence-callback limitations. Historical preparation exercised only Python/FFmpeg shim prototypes, not the Go tests; subsequent Safety verification green for 9137668 was reported by the user. No broader acceptance gate is closed.

Earlier HTTPS application-shutdown fixtures used server=nil; the new test passes the real fixture server. This is not runtime-image SIGTERM or live RTSP testing. Forced renderer tests used SIGSTOP; tmpfs tests are not deployment-filesystem or power-loss tests. Ordinary graceful trim-render cancellation, probe cancellation, container shutdown with active recorders, remaining runtime storage failures and real-system/browser validation remain outstanding. Re-record evidence for each exact committed SHA; never carry a prior green result forward.

The runtime-storage batch adds private-tmpfs exhaustion after continuous capture, event acquisition or paced assembly is already writing, plus a UID/GID 65534 helper for actual manifest/configuration/publication EACCES checks. See RUNTIME_STORAGE_FAILURES.md. Preparation executed only a native unprivileged descriptor-relative create/rename prototype, not Go or privileged mount tests. Record CI for the resulting exact SHA. Lost/unavailable mounts, blocked I/O, deployment-filesystem compatibility, service-account/container qualification and runtime quota policy remain unresolved.

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
