# Playback failure handling and explicit ended resume

## Supported behavior

Each metadata-loaded video receives a playback session bound to its seek generation, video element identity and grid index. Its error listener remains installed after loadedmetadata, unlike the loading-only error listener. Session and continuation cleanup use the existing segment cleanup registry, so seek cancellation, timeline invalidation and grid rebuilds detach them.

A media error before metadata or during playback stops all selected players, clears pending playback work, removes session/continuation listeners, marks the affected video unavailable, and reports a fixed error message requiring a new timeline selection. It does not skip to another file or continue across a decode failure. Other players are paused rather than claimed synchronized or successfully playing. A media error is not classified as a specific codec, network or storage diagnosis.

Both initial autoplay and explicit Play use the same guarded play call. Synchronous exceptions and rejected play promises pause all players, clear the shared playing intent, update the button and report a fixed message without exposing exception text. Existing valid sessions remain, allowing an explicit Play retry at their current positions; there is no automatic retry loop, source reload or alternate-media fallback. A later retry may still fail under browser policy.

Each play request captures the current playback-intent version and its session's play token. Rejections after pause, a newer Play request, seek/invalidation or player replacement are ignored. Ignoring a stale rejection does not restart playback or mutate newer state. Promise fulfillment has no state-changing callback. Undefined legacy play return values are supported.

When Play is requested after a paused ended event, exactly one selected camera with a valid ended session must re-enter the existing ended continuation handler. The unique next interval must start exactly at the current interval's metadata end, cover that instant and have a distinct nonempty path. Gaps, overlap ambiguity, duplicate candidates, same-file continuation and the selected day's end stop playback. Ended multi-camera playback is stopped with a select-another-time message rather than silently rewound or advanced. Pausing while the next clip's metadata is pending continues to suppress autoplay.

Play without a valid loaded session or pending load asks for a recording time rather than claiming playback started. Pending metadata loads are not prematurely played by the toggle control. No multi-camera clock synchronization, automatic next-day lookup, gap skipping or gapless transition guarantee is added. Metadata-derived playhead time remains uncalibrated camera wall-clock time.

## Regression and verification scope

The new Node regression loads the repository's playback.js under a stub DOM/media/timer harness. Cases cover media errors after metadata, stale error handlers, rejected promises and synchronous throws, explicit retries, stale rejections after pause/new Play/seek/invalidation/replacement, paused-ended continuation and unsafe boundaries, pause during pending continuation, loading-aware toggles and prevention of retry after media invalidation. A Go entry point runs it in UTC and America/Chicago. The lifecycle baseline now expects an active error listener after metadata and verifies its invalidation cleanup.

The full base playback.js was reconstructed from inspected file/history content and verified against its Git blob SHA 3b7f442e5c00475dcf1fccd5e15e573826faaa5d before editing. Node syntax checking of the complete modified script passed. The new failure regression and updated lifecycle regression passed against that complete modified script in UTC and America/Chicago with strict unhandled-rejection handling. Those are local stub-harness results, not repository CI, Go compilation/vet/race, real-browser, live-footage or download validation. Earlier draft excerpt-only results must not be confused with these narrowly scoped complete-script runs.

Keep the audit branch unmerged. Real-browser tests must check autoplay denial, a genuinely corrupt/decode-failing clip, late asynchronous errors, pause/resume at media end and real metadata delay. Stub tests cannot certify browser decoder behavior or footage completeness. Exports, backend recording, evidence preservation and retention policies are unchanged by this batch.
