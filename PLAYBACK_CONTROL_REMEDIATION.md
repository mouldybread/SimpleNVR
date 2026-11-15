# Playback control and browser failure remediation

Base: 9aca3f4f5851dd493b3f2ac6f971ac27cde8fbdb, failed in both supplied
Chrome artifacts. The unchanged baseline passed all twelve recorded checks in
both runs, and eleven tooling tests passed. Both edge runs stopped at the first
metadata case: a 30-second wait for nonexistent #play-pause. The media request
was correctly held; metadata was pending, with no media/page/handler error.
The other three edge cases did not start. This was a harness assumption plus
missing user-facing implementation of documented explicit Play/retry behavior,
not evidence that all playback failed or that the remaining cases passed.

## Frontend implementation

Add one visible enabled native type=button playback control before Mute, with
aria-controls=video-grid and an accessible Play recordings action name. It calls
the existing guarded togglePlayPause, not a new media-start implementation.
Plain-text Play/Pause labels and accessible action names update together through
setPlaybackButton. They express intent, including pending metadata, not proof
that all selected players are decoding. Keep an enabled Play action before a
selection: the existing select-a-recording message remains, with no false start.

Use the shared label updater for seek start and conservative continuation stops,
in addition to existing pause/rejection/media-failure paths. Timeline invalidation
and grid teardown clear stale playing intent and update the control; an existing
subsequent reseek may still establish new playback through the unchanged seek
policy. No selected cameras or all-unavailable media must not leave Pause intent.
No multi-camera synchronization, new continuation rule, recovery or decoder
fallback is introduced. Existing session/intent guards and evidence policy remain.

The global shortcut handler ignores interactive/editable targets, modified/default-
prevented keys and repeat events, leaving focused buttons their native Space/Enter
activation instead of stealing it. Body-level Space and arrow shortcuts remain.
This also avoids hijacking speed-select arrows or Space on unrelated buttons.

## Native acceptance and reporting

Keep all four original browser edge cases and the baseline mandatory. Before the
edge cases, a separate activated page checks the actual rendered unique visible
control, accessible role/name, target, type, enabled state and keyboard order.
Enter before selecting footage must request selection without claiming playback.
After a real seek, focused Space pauses once, native clock remains paused, and
focused Enter resumes once. Compare playback-intent versions to detect double
toggles; require native frames. This page is not reused for autoplay denial, which
still requires a fresh unactivated page and a trusted real click for retry.

Fix aggregate stage status on exception/cancellation: only a running stage becomes
failed, completed baseline success remains passed, and unstarted work stays not-
started. The top-level result remains failed. Context/browser cleanup exceptions
also prevent an edge report from claiming success. No timeout/retry expansion,
assertion weakening, browser flag change or synthetic playback fallback is added.

## Regression group and CI wiring

- Node control regression exercises initial selection refusal, labels, pending
  metadata toggles, rejection/retry and stale rejection, media errors, ended/gap
  stops, paused-ended resume, invalidation/grid reset, unavailable media and keyboard
  target/default/repeat guards. New Go discovery runs it in UTC and America/Chicago
  alongside existing frontend tests.
- Python HTMLParser contract reads the actual template; rejects absent/duplicate,
  wrong-tag/submit/disabled/hidden/negative-tabindex, mislabeled/unwired controls
  and absent/ambiguous target. This is markup evidence, not a native browser substitute.
- Seven new Python test methods cover markup faults and aggregate baseline/edge
  failure, cancellation and success behavior. The existing shell discovery broadens
  only to test_browser_playback*.py, covering these plus the original eleven methods.
- Existing core/race/vet checks and both isolated Chrome jobs remain mandatory.
  GitHub Actions is the execution gate for this candidate.

Preparation reconstructed the complete playback.js using its inspected current
prefix and unchanged inspected historical tail, then verified Git blob
7d640b3fc9221a18fd4e3b6528e636173d845591 before editing. The complete template
and runner were likewise blob-verified; no truncated source was blindly replaced.
The shell base is verified by reversing the known prior three-line driver change.

## Scope

This is an explicit production FRONTEND change, plus test/report corrections—not
a test-only batch. Backend recording/export media commands, storage, manifests,
retention/cleanup authority, runtime budgets, main, merge and deployment remain
unchanged. 5ccf40b remains user-reported green; 9aca3f4 remains failed until a new
exact candidate passes. Later failures may expose additional independent issues;
fixing the missing button alone does not qualify the four browser cases. No
production resource acceptance or ingestion-time coverage is granted.
