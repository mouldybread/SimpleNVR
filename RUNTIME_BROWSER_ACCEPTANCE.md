# Live runtime-image export and browser acceptance

## Candidate and evidence status

Prepared against 6767a887cb56652c519cdb5c05d973103cc3827e, user-reported green on November 11, 2026. Actions logs were not independently inspected. This is a group-B validation batch, not completion of groups A-D. It changes no application recording, playback, authentication, export or deletion policy. Existing phase-cancellation Go tests and active-container/storage CI remain enabled.

The earlier tests/browser-export-acceptance.py intentionally routes fixture responses into a controlled DOM. It remains useful scoped evidence but is not a real backend/browser integration test. The new harness is separate and never replaces that earlier record with a broader claim. No routed network responses, replacement DOM, media-event dispatch stubs, play-promise overrides or fake rendered MP4s are used here.

## New mandatory CI path

The existing CI-built runtime image generates two contiguous eight-second 256x144 10 fps yuv420p H264 files and a separate H264/AAC file. One no-recording camera serves those synthetic files; no live camera is contacted. The real application runs with a read-only root, explicit recordings/certs/tmp mounts, reduced capabilities and a loopback-only published HTTPS port. A temporary administrator password is generated for the fixture and is not included in the report or command-line arguments.

A read-only test wrapper prepends -re to the real runtime FFmpeg arguments so render cancellation can observe an active child. It preserves all other arguments and execs the runtime binary. This is test-directed pacing, not a default production render-speed change. Fixture encoding uses the real binary directly. Browser tooling is isolated in a CI virtual environment with Playwright 1.56.0 pinned; the actual Chromium version is recorded when the test executes. The overall job budget rises to 25 minutes to include browser/dependency installation; the acceptance driver itself has a 300-second deadline plus bounded container stop/cleanup operations.

The browser driver loads the actual login form, HTML, scripts, camera template and timeline through HTTPS. It fills the date picker. Short seek/range values and pause/resume actions are directed through the application's existing state/functions because the current export modal has no short-range text inputs. Those calls do not replace production functions and do not qualify as validation of every mouse/keyboard affordance. The download modal, export start, cancel, handoff and URL-release buttons are clicked normally.

## Checks that must pass

- Establish a real session through the login form, load the camera template and retrieve real timeline metadata.
- Play private H264 media in native Chromium, observe advancing time and a requestVideoFrameCallback witness with decoded-frame count and dimensions.
- Pause/resume through application controls and verify native paused/clock behavior. Let a native ended event advance to the uniquely contiguous second clip.
- Direct the active application video element to a missing private URL, observe a real HTTP 404/native media error, and retry through the actual application seek path. This is an intentionally injected source switch after successful playback, not a mid-transfer physical/network failure.
- Refuse export/private-media requests from a separate unauthenticated HTTP context.
- Export whole clips through the actual authenticated backend, verify the whole-clip policy header and externally decode/probe the saved 16-second result.
- Reject real coverage gaps with 409/gap and audio-containing trim inputs with 422/unsupported, without a video response.
- Cancel through the UI while a paced runtime FFmpeg process exists, observe canceled UI completion, no prepared URL and disappearance of runtime FFmpeg children.
- Run a subsequent trim through the UI, verify frame-window policy text, receive it, request a browser-managed download, save it through Playwright, compare its SHA256 to the received Blob bytes, externally decode/probe the 15-second result, and verify object-URL release.

Download save_as is a real headless-browser-managed filesystem save observed by the harness. The application must still say disk saving is not verified: it cannot observe the harness's filesystem witness and must not infer a successful OS save from clicking an anchor. Valid/decoded synthetic output is not proof of complete camera footage or a calibrated wall-clock boundary.

## Evidence artifacts and execution

CI uploads the commit/image/browser report, saved synthetic whole-copy and trim media, and runtime-container log under a commit-specific artifact name for 14 days. A failed driver writes a failure report when its browser context was established, and cleanup captures container logs. The generated fixture state and credentials are deleted only from their unique test-owned temporary directory. No deployment recordings or staging inputs are touched. Browser/tool installation failures are job failures, not silently skipped acceptance.

For an explicit local run with Docker, the image, Python Playwright/Chromium and host FFmpeg installed:

    bash tests/runtime-browser-acceptance.sh IMAGE EVIDENCE_DIRECTORY

The driver generates and cleans its own application state. It does not run against an arbitrary existing production server. It requires sudo for synthetic container-owned state cleanup. Do not point its fixture storage at deployment evidence.

Preparation verified the original modified workflow Git blob, shell/Python grammar and exact payload/patch round trips. A separate host-native FFmpeg prototype generated the eight-second video-only profile, exercised the paced copy/trim command shapes, and externally decoded outputs of 16.0 and 15.0 seconds. That prototype did not execute Go, Docker, Chromium, the real application or inherited-descriptor paths. Its report is a preparation artifact, not runtime-image/browser acceptance evidence. Go/gofmt and Docker are unavailable locally; the new integration harness has not executed locally. Passing syntax and native prototypes do not imply passing CI.

Source review included the existing browser/export UI harness, actual HTML, auth session/method boundaries, export planning/input/render paths, core/modal scripts and the current workflow. The connector truncated playback.js; it was not reconstructed or modified. The harness will load the complete committed script from the actual runtime image.

## Remaining B gates

This candidate supplies executable coverage but closes nothing until exact-candidate CI evidence is obtained. Genuine autoplay-denial/retry, delayed metadata, paused-ended edge cases, late transfer truncation/errors, wider browser/OS save-dialog behavior and remaining timeline/DST interactions still need explicitly scoped acceptance. Existing probe/profile/post-render cancellation tests remain separate Go evidence rather than being recounted as new work. Runtime export shutdown/backpressure and full deployment browser acceptance remain subject to the final evidence review. A's live RTSP/deployment-filesystem gates and C/D whole-branch/final-candidate review remain open. main is untouched and fix/recording-safety-audit remains unmerged.
