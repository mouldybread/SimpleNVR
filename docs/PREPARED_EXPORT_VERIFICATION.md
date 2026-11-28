# Prepared export verification

## Recorded CI result

[Pull request #8](https://github.com/mouldybread/SimpleNVR/pull/8) was checked after the recording-status fix from PR #7 was brought into the export branch. On its head commit `b8a6178136f620d88f82099ddc7a3a91cdd5cff7`, GitHub reported all 11 check runs completed successfully:

- Full Go build, race-enabled suite and `go vet` (push and pull-request runs).
- Prepared jobs/native playback Chrome attempts 1 and 2 (push and pull-request runs).
- Safety verification: core build/race/storage/lifecycle, both isolated Chrome runs, bounded synthetic resource measurements, and the required-checks gate.

This records those specific check results, not a new test run on later commits. The count includes duplicate push/pull-request executions, not 11 independent test suites. See the [PR check results](https://github.com/mouldybread/SimpleNVR/pull/8) and its linked Actions runs.

## Automated scope

The prepared-export workflows trigger on pushes to `fix/large-media-exports`, pull requests and manual dispatch. The Go workflow uses the complete race-enabled suite, not an export-only subset. The browser workflow makes two independent Chrome attempts against a runtime container built from the exact checkout.

The live HTTPS browser harness checks:

- Anonymous requests to prepared-job routes and private media are refused.
- Whole-copy compressed video packet hashes, external decode and duration, plus HEAD and Range behaviour.
- Missing-coverage refusal without a download.
- Accurate export with one AAC track and the expected audio/video durations.
- Partial Copy accepted on the first request, with the saved range reported and within the 30-second extension limit.
- Active accurate-render cancellation and FFmpeg child reaping.
- Browser-saved SHA-256 matching, external decode/duration, and HTTP 404 for new requests after server release.
- Export queue panel: Refresh lists the new job, the item's Download link points at the exact download URL and saves a decodable file whose SHA-256 matches the API download, and Clear all empties the queue, makes the download return 404 and leaves `GET /api/exports/list` empty.
- Range bar over the real timeline: keyboard moves on the end handle, a real mouse drag of the start handle follows the timeline viewport, an overshoot clamps start to one second before end, and closing the panel hides the range bar.
- Server capacity: eight paced Accurate jobs are accepted and listed with consecutive queue positions, the ninth create is refused with HTTP 429 and `Retry-After: 5`, Clear all releases all eight and stops their FFmpeg children, and a new create is then accepted.

Native playback, real login/navigation diagnostics and the expanded playback-edge suite are retained. The container mounts private export scratch at `/exports`, outside `/recordings`, with small test quotas. Non-reordered fixtures exercise supported subsection copy; Go tests cover reordered-video refusal.

The Go suite also checks that the default limits admit eight exports at once (one preparing, seven queued), which is the largest export the UI can submit, and refuse a ninth.

## Local preparation checks

Before the browser migration was committed, Python/shell/YAML syntax checks and parsing of 30 embedded JavaScript expressions passed. A separate media-helper check preserved 160 compressed video packet hashes and externally decoded a 16-second fixture. Six protocol regressions also passed. Those local checks were not live-backend/browser execution; the CI result above supplies the recorded Go/container/Chrome execution evidence.

## Limits

Passing synthetic acceptance is not production-camera, full-resolution, multi-GiB-transfer, colour-fidelity or heavy-load qualification. The metadata-only large-file UI regression is not an actual multi-GiB transfer.

The live browser fixture has a single camera, so queuing several cameras from the UI in one request and the 8-camera client limit are covered by the Node UI tests, not by live Chrome. The server's capacity limit is exercised live through the API, with several jobs for the same camera.

The older routed-response harness `tests/browser-export-acceptance.py` has been removed. It targeted the retired direct-download flow and was not run by any workflow. The live-runtime entrypoint is `tests/acceptance/runtime-browser-acceptance.py`, invoked through the expanded playback runner.
