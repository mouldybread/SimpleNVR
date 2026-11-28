# Browser export UI

The playback toolbar's Export range button calls openExportPanel from export-ui.js, loaded after playback.js. It opens an inline export panel in the playback section and shows a floating start/end range bar over the timeline. The old Download modal, its separate camera checklist and the copy-approval prompt have been removed. The legacy direct-link startDownload function is absent from playback.js without a compatibility alias; source guards reject legacy definitions, calls and references, and a JavaScript load test asserts the old globals are absent while the export entry points exist. These guards are not a general call-graph proof.

## Cameras and range

An export always covers every camera currently selected for playback. Users leave a camera out by deselecting it in the playback camera selector; the panel states which cameras will be exported. One to eight unique cameras are accepted. The range opens as ten minutes either side of the playhead and is adjusted with the Earlier/Later buttons and step selector, or with the two range handles on the timeline. Handles are role=slider buttons: pointer dragging maps through the timeline viewport and clamps to it, and Arrow, PageUp/PageDown, Home and End keys move a boundary. Start always stays at least one second before end. The end is exclusive. Mode is chosen with a Copy / Accurate toggle (aria-pressed). The mode toggle, range controls, handles and submit button are disabled while a submission is running.

## Requests

Submitting sends one POST /api/exports/create per selected camera, in selection order, with only camera_id, start_time, end_time and mode. There is no proposal, consent or accepted_start/accepted_end step. Copy may be extended outward by the server to safe cuts; the saved range must be a valid interval covering the requested range or the job is refused, and an extended range is labelled as such. A create whose response is lost or invalid is reported as uncertain and is never retried automatically.

All control requests are small JSON requests made through one helper with credentials same-origin, cache no-store, redirect error, a 45-second abort timeout and a streamed response limit (8 KiB for control responses, 64 KiB for the queue list). The page never fetches or buffers video. A prepared file is saved through a normal same-origin link to /api/exports/download?id=<job-id>; the link is offered only when the server's download_url equals exactly that path for the same job. Saving to disk is never claimed as verified.

## Progress and queue

Jobs created by the current request are monitored with at most one status request per job every 1.5 seconds, stopping on ready, failed, canceled or expired, on cancel, or after two hours. Each row has Cancel, and ready rows offer Download prepared video and Release prepared file / continue.

The session queue lists GET /api/exports/list once when the panel opens, on Refresh, and after create, cancel, release and Clear all actions; there is no background queue polling. Each entry shows the camera name and mode, its state, queue position and estimated size where present, actual size when ready, the requested range and the saved range, a Download link when the URL is exact, and Cancel job or Release. The camera, mode and requested range are taken from the server's job fields (camera_name, mode, requested_start, requested_end), so a reloaded page shows them for jobs it did not create; for jobs this page created the page's own record is the fallback, and a job with no usable details is titled with the start of its id. An expired job stays listed as expired, without actions, until the session clears it. Clear all sends POST /api/exports/clear with {} and the server removes only jobs owned by the current session.

## Messages

Server reason codes (output_limit, estimate_exceeded, deadline_exceeded, empty_output, render_failed, canceled, cleared, expired, service_stopping) are shown as fixed plain-language text; unknown reasons are shown generically. Server message fields and error bodies are never displayed. HTTP failures map to fixed messages (authentication, unavailable, queue full, recording gap, unsafe copy, missing job). All names and text enter the page through textContent only.

## Tests and limits

Node tests (tests/frontend/export-ui.cjs, tests/frontend/export-mode-ui.cjs, tests/frontend/export-range-controls.cjs, tests/frontend/export-queue-details.cjs, tests/frontend/download-entrypoints.cjs) exercise the real module with a stub DOM, the real timeline geometry helper and stubbed fetch/streams. They are not browser tests. tests/acceptance/runtime-browser-acceptance.py drives the panel in Chrome against a container in CI, including the queue list and its download link, Clear all, the range handles with a real mouse and keyboard, and the server's capacity limit. Browser download restrictions, filesystem failures and media completeness of saved files remain unverified. Real-browser/authenticated export integration, storage-failure validation and final whole-branch review remain required under AUDIT_ACCEPTANCE.md.
