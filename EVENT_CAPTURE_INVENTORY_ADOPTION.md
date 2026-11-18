# Capture/concat creator-descriptor adoption

Base: 701fb04a6cefd64f685a872a76dd8158c23c969d, fix/recording-safety-audit.

## Bounded production change

Production event capture creates event.mp4 exclusively, owner-only, relative to
its already pinned inventory root under a short media lease. FFmpeg receives the
creator as ExtraFiles[0] (child fd 3) and writes /proc/self/fd/3. This is a seekable
file, not pipe:3. Faststart can reopen that descriptor path without opening a
replacement event.mp4. Linux procfs access in the child is required; failure has
no pathname-output fallback. The existing command helper still owns cancellation,
interrupt/grace/kill and join. No media lease spans Run/Wait or RTSP capture.

concat.txt uses the same exclusive creator helper. The creator survives all list
writes. After each writer/process returns, a fresh bounded metadata lease records
final identity through Remember, including failed/canceled writers' remaining
bytes where admission succeeds. Original creators close on every return; only
inventory duplicates survive until existing terminal job teardown. Registration
failure refuses finalization, retains evidence, and never re-adopts a replacement.
An operation adds at most one creator and one transient Remember duplicate above
the existing bounded inventory. Helpers receiving a lease never reacquire it.

The post-write metadata budget is independently rooted in Background, matching the
existing checkpoint policy rather than treating capture cancellation as metadata
cancellation. It does not make kernel I/O interruptible. Job inventory access is
serialized by the existing job owner; concurrent teardown is not supported.

## Deliberately unchanged boundaries

The published output candidate, concat subprocess input path, publication calls,
partial-output handling, durable-publication sequence and published checkpoint
ordering remain unchanged. Concat/input reads are still pathname-based; this is
not hostile-writer read isolation or end-to-end publication ownership. Manually
constructed jobs without an inventory retain legacy compatibility behavior.

No cleanup authority is added. releaseOwnedJob, keepStaging defaults and recursive
success cleanup are unchanged. Positive eligibility, nonrecursive release and
publication-through-release coordination remain future slices. Inventory evidence
is neither proof of complete capture nor permission to unlink. Failed list writes
may register a partial list as recovery evidence, never as success proof.

## Evidence

Ten regression tests plus one synthetic child helper are authored for final
registration, failure/cancellation, replacement refusal, exclusive creation,
canceled admission, creator/pin closure, production capture/concat adoption,
checkpoint failure and gate availability while a barrier-blocked child runs.
These Go tests have NOT been executed during preparation: Go/gofmt are absent.
No race/vet, application, runtime-container, live-RTSP or browser run is claimed.

A newly executed standalone Python host probe explicitly remaps an inherited
creator descriptor to child fd 3 and execs FFmpeg. MP4 stream-copy + faststart,
FFprobe and decode pass with and without named-entry replacement; replacement
bytes remain unchanged. Its script and JSON are saved with the review artifacts.
That probe does not execute this Go implementation or establish runtime-image
compatibility. The older uploaded probe remains separately labeled historical.
