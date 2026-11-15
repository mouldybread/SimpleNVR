# Native browser playback edges — grouped acceptance candidate

Base: 5ccf40b2d480cbceb1bb6eff466448e85280dff7, green by user report.
The user supplied its complete correspondence assessment on November 15, 2026.
All four matrices and both writer summaries passed that assessment; Actions logs
and the underlying JSON artifacts have not been independently inspected here.
Complete-validation medians/maxima (seconds), from the supplied summary:

| Profile | Load | Evidence median | Comparison median | Total median | Total maximum |
| --- | --- | --- | --- | --- | --- |
| 360p 14s | idle | 0.155949 | 0.047472 | 0.203429 | 0.205469 |
| 360p 14s | paced writer | 0.162920 | 0.048808 | 0.211739 | 0.211884 |
| 720p 110s | idle | 2.224511 | 0.134073 | 2.358596 | 2.363696 |
| 720p 110s | paced writer | 2.223153 | 0.133380 | 2.356545 | 2.356970 |

These cache-warm fixture observations support continuing development; they are
not resource-cost acceptance, intended-host fit or tail-latency qualification.
Evidence collection dominates the 720p cost. Observed writer byte progress across
the two-repeat validation observation window was reported only for 720p; both
profiles separately demonstrated post-measurement segment/byte progress and joined
termination. Post-measurement progress must not be counted as in-window progress.
Runtime qualification, cleanup authorization and resource-cost acceptance remain
false. The browser implementation is unchanged from the previously prepared set;
this update reconciles its base status and the supplied numerical evidence only.

## One group, five files

Retain the unchanged real runtime browser/export driver as the baseline. A new
orchestrator must first complete it, then run all four additional playback cases.
The existing shell harness invokes that orchestrator and runs eleven lightweight
acceptance-tool contract tests, retaining their log before generating fixtures.
Both original independently named Chrome jobs remain mandatory through the
existing workflow. No workflow job, baseline assertion or required check is removed.
Fixtures, application source, credentials handling and runtime image are unchanged.

## Four native cases

1. Delayed metadata and pause/resume: CDP holds exactly the first matching real
   media request before server delivery, with that page's cache disabled. Observe
   native HAVE_NOTHING, a pending metadata handler, no established session, and
   no metadata/playing events. Click the actual pause/resume control while held;
   intent must change without media advancing. Release the real request and
   require the correct source, advancing native clock, trusted metadata/playing
   events and a decoded-frame witness. Hold at least 400ms while paused.
2. Pending-load retarget: hold the first source, seek through the application's
   real function into the second clip, and establish native playback there.
   Release the obsolete request (or record that Chromium already cancelled it),
   then check the replacement source/generation/session remains intact. This
   qualifies pending-request invalidation, NOT delivery of a stale metadata event.
3. Paused at native end: start real playback, pause through the control, assign
   native currentTime to native duration and wait for the browser's ended property.
   It must not advance while paused. Explicit trusted resume must establish the
   uniquely contiguous second source, advancing clock, native events and frames.
   This is an end seek, not proof that a naturally paused clock reaches end.
4. Browser-enforced autoplay denial/retry: use a separate fresh Chrome process
   with document-user-activation-required and media-engagement bypass disabled.
   Authenticate once through the real form, then close that activated login page.
   A new page in the authenticated context must have no user activation; all
   setup/read/seek evaluation on that page uses CDP userGesture=false. Select the
   actual AAC fixture, unmuted at volume one. Require the application's stopped
   intent/retry message and a native play() NotAllowedError witness. A real click
   on Play must then start the audible-track media with native events and frames.
   No HTMLMediaElement.play replacement, mocked promise or fake media event is used.
   Unmuted media state is not a physical-speaker/audio-loopback measurement.

The stricter Chrome policy is confined to the additional test browser. The baseline
launch/flags and all production behavior remain unchanged. The extra login is a
separate fresh-browser test session, not an automatic credential retry. Existing
bounded successful-login GET navigation recovery is reused unchanged.

## Evidence and failure handling

The original runtime-browser-report.json/media/network artifacts remain unchanged.
Add runtime-browser-expanded-report.json, a playback-edges/playback-edge-report.json,
and separate network diagnostics. The aggregate only passes if baseline and all
four cases pass. Preserve partial case states, failures, native snapshots/events,
frame witnesses and request-release traces; never log cookies or passwords.
Request holds use Fetch.continueRequest, never fulfillRequest or substitute bytes.
Only the explicit invalid-interception-ID cancellation diagnostic is tolerated;
other command/handler errors fail. Pending holds are released/disabled and handler
work joined; test pages and browser contexts/processes close on success or failure.

The expanded orchestrator shares the existing 300-second driver envelope; the
existing readiness/outer shell bounds and runtime-container stop checks remain.
The new contract-test subprocess has its own 20-second limit. CI executes actual
Chrome/backend/media checks and the synthetic tooling regressions. Preparation
checks grammar, the modified shell's original Git blob, and patch round trips;
none of those checks establish native browser acceptance.

## Original-audit reconciliation and remaining gates

These cases supply executable coverage for specific remaining browser gates;
they close nothing until exact-candidate CI evidence is obtained. The older
RUNTIME_BROWSER_ACCEPTANCE.md remains historical/scoped baseline documentation;
this document records the additional candidate without rewriting past evidence.
Late mid-response truncation/backpressure, other browsers/OS save dialogs,
remaining timeline/DST interactions, live Dahua/RTSP, deployment filesystem/blocked
I/O, final whole-branch review and intended-host resource acceptance remain open.
No ingestion-time proof, comparator activation, cleanup/expiry/reassembly authority,
production source change, merge or deployment is included. main stays untouched.
