# Active-recorder and full HTTP shutdown validation

## Controlled integration scope

active_recorder_shutdown_test.go adds two ordinary Go tests, automatically discovered by the existing Safety verification race suite. Python 3, FFmpeg, FFprobe and Linux procfs are required. No separate workflow wiring or timeout increase is added.

A test-only executable substitutes only the sentinel rtsp://simplenvr-fixture input with a real-time loop of generated local MP4 media, then execs the real FFmpeg in the same PID. Non-capture commands pass through. The assembly case adds input pacing to concat processing. This does not implement or test RTSP transport, camera authentication, disconnect/reconnect or Dahua behavior. No production code is changed.

The full-server test runs production continuous and event recorder workers, observes nonempty in-flight outputs and real FFmpeg executables, then shuts down with an authenticated HTTPS whole-copy export under actual socket backpressure. It passes the fixture's listening http.Server to shutdownApplication, not nil. A handler-cleanup latch must block persistence. The persistence callback requires observed capture/export children to be gone before writing a snapshot; the completed shutdown must close export resources and the HTTP listener, release event capacity and clear runtime handles while retaining restart intent. Explicit stop separately joins an actual continuous recorder and clears intent. Synthetic continuous partials must decode in full. Interrupted event capture must retain its checkpoint, nonempty capture and original pinned seed across buffer cleanup.

The separate assembly test cancels an observed, output-producing real FFmpeg concat process without SIGSTOP. It requires child reaping, partial publication, full synthetic partial decode, slot release and retention of both original inputs, concat list and partial publication checkpoint. Pacing changes input timing; this is not unmodified-speed or live-camera assembly acceptance.

## Persistence and evidence limits

Camera media stays in test TempDirs. Production saveAt deliberately rejects those recording paths. The test callback therefore serializes the actual stopped registry and uses atomicWrite on a TempDir snapshot; it checks saved intent by JSON decoding. It does not bypass the production validator, use /recordings, exercise saveAt backup validation, or prove real startup resume. Existing persistence and restart-control tests remain separate.

/proc PID/executable observations follow the existing Linux test approach, not descriptor-pinned production process control or a guarantee against PID reuse. No test deliberately stalls filesystem calls. Cancellation uses the production interrupt/force-kill helper; these tests do not inspect every capture command's WaitStatus and must not be described as proving no force-kill fallback occurred.

Full decode of generated partials does not establish complete requested footage, timestamp accuracy, gapless capture, storage durability or power-loss recovery. Event admission is direct internal TriggerEvent, not the HTTP storage-policy boundary. The live RTSP, production filesystem, runtime-image SIGTERM, inspection concurrency and browser gates remain open. No deletion, expiry or recovery automation is added.

## Preparation and required follow-up

User-reported Safety verification is green through 45a6083; Actions logs were not independently inspected. Go and gofmt are unavailable in the preparation environment. No Go compilation, vet, race or full-server test execution is claimed for this batch.

Local preparation executed the Python input-substitution and paced-concat shims against real FFmpeg, observed output before ordinary SIGINT and fully decoded both resulting synthetic MP4s. This validates only those two native fixture prototypes, not their Go orchestration or all recorder modes.

Record Safety verification for the exact resulting commit. Keep the audit branch unmerged. Actual deployment acceptance still requires controlled camera/network interruption and active-recorder container shutdown with independently preserved evidence, followed by the remaining whole-branch review.
