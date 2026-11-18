# Streaming admission: test-only qualification candidate

Base: 88942a209463bc9c13590929ad5b4ddb5046272e, user-reported green. The paired resource artifact was reviewed and the user selected streaming over scratch for further qualification. This is not production activation, merge, deployment or cleanup authorization.

## Decision and evidence

The two-run CI averages were approximately 0.202s scratch versus 0.065s streaming for 14s 360p, and 0.382s versus 0.191s for 110s 720p. Both matched video and audio. Streaming eliminated owned normalized scratch writes and used two children rather than four starts. The 1080p oversize case refused in approximately 0.167s streaming versus 0.096s scratch; streaming did more work before refusal. It was not a completed 1080p comparison. Measurements were cache-warm, fixed-order and excluded full bound-adapter cost; these are not host-performance guarantees.

## Proposed admission policy, not a production budget change

Add a wrapper entirely in _test.go. A shared experimental gate permits one full streaming inspection at a time and rejects overlap immediately; there is no waiting queue while holding a media lease. Its slot is retained through adapter return, after collector child and worker joins. Every return releases the slot. The gate is explicitly supplied, not global: creating separate gates would permit separate work. These tests do not establish a process-wide or host-wide production concurrency limit.

One five-second outer context starts before admission and the initial binding check. It covers initial evidence binding, collection and synchronous revalidation. A shorter caller deadline is preserved and the child contexts are not renewed. Existing command WaitDelay remains separate teardown grace. Contexts cannot make arbitrary filesystem calls deadline-interruptible; this is not a hard five-second wall-clock guarantee.

The existing collector's 20-second experimental maximum is unchanged; the wrapper shortens its effective deadline. Stream/input byte limits and conservative admission are unchanged. This candidate adds no production code, FFmpeg launch site, recording flags, receipt, persistent pin, cleanup eligibility, retry queue, workflow or dependency change. Existing production combined evidence inspection retains its separate two-child/five-second contract. Streaming would be extra work, not automatically covered by that production process ceiling.

## Prepared tests

Use the existing genuine job/evidence/lease fixture with synthetic media bytes and controlled re-executed children. Success tests cover with/without prebuffer and shorter-parent versus wrapper deadlines. Nested admission attempts at the after-launch and after-join boundaries must refuse before binding I/O or child/pipe allocation. Child context identity and all six revalidation boundaries are checked.

Pre-cancelled contexts, absent evidence, second-child launch failure, content mismatch, cancellation after both starts, and actual wrapper deadline expiry must refuse, join all owned work, preserve output/staging and leave the caller lease held. Each failure is followed by successful readmission using the same genuine witnesses. Missing/invalid gates must not launch work. These tests exercise full adapter binding/lifecycle behavior, not native encoded-media equivalence or live recorder load. Existing native, race, browser and paired resource gates remain required.

## Still unqualified

Exact-new-head compilation, race tests and CI are pending. RuntimeQualified and CleanupAuthorized remain false. This batch does not claim full-adapter CPU/I/O measurements, cold-cache behavior, peak concurrent memory/CPU, recorder-host performance, hostile descendant containment, every codec/topology or NVR-ingestion-time coverage. It does not justify changing the 32 MiB ceiling to admit the rejected 1080p fixture.

After CI acceptance, remaining work includes full bound-adapter native-media measurements under controlled recorder-like load, an explicit shared runtime admission/ownership design, and recorder-host acceptance with agreed recording-impact thresholds. Any future runtime resource-policy change requires separate review; the comparator remains outside production and staging remains retained.
