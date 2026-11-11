# Export-phase cancellation and scoped Chromium evidence

## Backend cases awaiting CI

export_phase_cancellation_test.go adds six authenticated HTTPS cases: duration discovery, trim-profile inspection and an output-producing trim renderer, each canceled by the client or shutdownApplication with the actual listening fixture http.Server. A separate duration-cache test checks occupied admission, context.Canceled, slot release and child reaping. Existing Go discovery is sufficient; no redundant workflow wiring or timeout increase is added.

Probe fixtures replace only the child input descriptor with an empty pipe whose writer stays open across exec of real ffprobe. They produce no fictional metadata. Cancellation therefore exercises production command joining, handler/resource cleanup and profile admission with a real blocked probe, but not a blocked regular-file syscall, corrupt-recording recovery or every metadata phase.

The renderer fixture adds -re input pacing before exec of real FFmpeg; it never sends SIGSTOP. The test requires a real executable, occupied export slot, anonymous output with written bytes and no completed response before cancellation. Cleanup must finish within seven seconds, below the configured eight-second force-kill grace. This is scoped cooperative-cancellation evidence if the Go tests pass, not unmodified-speed or universal graceful-exit assurance. Earlier deliberately stopped-renderer force-kill tests remain separate.

All cases require observed child reaping, no video/download headers from canceled preparation, preserved source and appropriate pin/slot/output cleanup. A handler latch blocks persistence until release; server-shutdown callbacks reject persistence before observed child reaping. Native preparation exercised both blocked-probe shims and a paced real renderer; the probes were killed as exec.CommandContext does, while the renderer received ordinary SIGINT. Go/gofmt are unavailable, so this is not Go execution or CI evidence.

## Actual local browser execution

Python Playwright with installed headless Chromium 134.0.6998.35 ran tests/browser-export-acceptance.py. Export UI blob 58cc92fe4ec29e3d7247aa622c83a59288a8c430 and geometry blob 77e518605508caa99a8c60b42c7fb477063c460b were reconstructed from complete connector reads and Git-blob verified before execution. Neither production script is edited by this batch.

Six checks passed: reject nine cameras before a request; render hostile camera/server text without HTML execution; reject advertised length above 64 MiB; cancel a prepared file and revoke its real object URL; receive a real browser download, save it to an automated temporary file, compare SHA-256 and fully decode it with external FFmpeg, then release the object URL; and evaluate 23/25-hour day lengths and distinct repeated-hour labels in an America/Chicago browser context. The committed JSON evidence binds browser version, script blobs and fixture hash.

This harness uses routed responses and a small real DOM. Browser fetch, Blob URLs, buttons and download management are native, not stub implementations. The save assertion proves only that this automated browser-managed file was saved and matched this generated fixture. External FFmpeg decoding is not browser playback decoding. It does not validate the live HTTPS/backend integration, end-user OS save dialogs, all browsers, actual timeline canvas, real late media errors, autoplay rejection, ended resume or metadata delay. Those acceptance gates remain open.

The browser runner is an explicit acceptance command, not silently added to ordinary Go CI. It requires Python Playwright, installed Chromium and FFmpeg. In an isolated test environment with a generated MP4, run:

```sh
python3 tests/browser-export-acceptance.py --repo . --media /path/to/synthetic.mp4 --report /path/to/browser-evidence.json
```

It uses --no-sandbox solely for isolated headless fixture execution; do not treat that as a production browser recommendation. Use generated media, not private recordings. Re-run against the exact final scripts and retain the report. User-reported Safety verification is green through bdc1ef5; new backend tests still need exact-commit CI. Keep the branch unmerged.
