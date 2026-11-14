# Production combined duration/presentation evidence — REBUILT CANDIDATE

Base: d4755650fd03f0c7e983f9a606223d65c7598089. Remote head verified.
Base green is user-reported, not independently retrieved Actions evidence.
This supersedes the BLOCKED 7c8271e-based production integration candidate.
Its old payload 93623951774c99de737fee9d3bd1a68695db1def5fa2415307f9449d385eeb44
must NOT be reused. This candidate preserves the prerequisite buffer fix.

## Production change and unchanged process ceiling

After durable publication, the published-checkpoint attempt and qualifying execution,
production finalization calls the combined collector. Each EXISTING capture and
published-output inspection runs ONE query obtaining duration_ts/time_base AND
presentation frames from the SAME selected video stream. There is no extra frame
query, separate duration query, fallback query or release-time retry. The ceiling
stays up to TWO read-only evidence-inspection children. Existing publication/codec
probes are unchanged and outside that evidence-inspection count.

One five-second outer context rooted in the live manager covers both probes,
before/after checks and receipt attachment. Inner deadlines can only shorten it.
WaitDelay still separately bounds child pipe teardown. No additional lease,
persistent pin, goroutine or metadata context is introduced or revived. The SAME
transferred lease and owned descriptors survive to terminal release.

Each file must independently pass BOTH an exact video-stream duration lower bound
and an explicit contiguous video-frame interval-length lower bound. Gaps, overlaps,
missing/invalid evidence, diagnostics, nonzero exit, changed descriptor metadata,
limits and cancellation refuse. Long output/prebuffer cannot compensate for short
capture. A duration-only observation cannot satisfy production revalidation.

The fixed-size presentation receipt binds the exact duration receipt and its
job/execution/inventory/request/start/output/manager/borrow bindings. It records
capture/output observations and compared descriptor metadata. Revalidation pins
its identity and checks it before and after evidence I/O, without spawning media
processes. Missing, substituted, short, stale or wrong-owner evidence refuses.
No canceled probe/assembly-local context is retained as a live requirement.

## Buffer-fix preservation and strict adapter

The ENTIRE named private buffer section from d475565 is preserved byte-for-byte,
including its limiting Write and explicit read-only accessors. No bytes.Buffer
embedding, ReadFrom or WriteString promotion is reintroduced. The already-green
copy-fast-path and actual stdout/stderr pipe regression file is left UNCHANGED
and remains discovered by normal package tests. This is not a claim of local Go
execution. Stdout still has the four-MiB cap, diagnostics the 16-KiB cap, with
sticky refusal. Parser rejection after collection is not relied upon as a cap.

The root adapter accepts only the reviewed known EMPTY optional arrays. Some
H.264/H.265 queries emit side_data_list: [{}] despite selecting no side-data fields.
The combined-query adapter permits ONLY up to 16 EMPTY objects in that known
optional array; nonempty/null data, unknown and duplicate keys refuse. The canonical
frame parser stays unchanged. No timestamp/duration alias or inference is added.

## Preserved publication and terminal behavior

Observation refusal does not undo publication, set/clear recoveryFailed, change
sticky checkpoint failure, prevent independent retained-checkpoint attempts, or
prevent descriptor closure, slot return and terminal lease release. Unconditional
staging retention stays in force. Assembly/publication returns and incomplete
output behavior are unchanged. No cleanup permit, engine call, deletion, schema
or workflow change is added. Main remains untouched; no merge is authorized.

## Compatibility, costs and validation limits

The query now decodes frames instead of inspecting metadata alone, doing MORE WORK
within the same deadline/process ceiling. Long/high-resolution footage may time
out or exceed the output cap; it then refuses and retains staging. Versions lacking
explicit frame duration remain unsupported: they may make every job ineligible
without preventing publication/retention. One FFprobe version is not deployment
compatibility or genuine camera acceptance. Kernel I/O is not made interruptible.

Existing production fixtures now emit explicit combined stream/frame JSON through
an executable, not assigned success flags. Tests retain process-count, no-retry,
expired metadata-budget, checkpoint-failure, held-lease and terminal teardown
assertions. New cases cover gaps in either file, missing/short frames, diagnostics,
receipt substitution before/after I/O, metadata drift, duration-only substitution,
strict same-stream duration metadata and the narrow side-data envelope.

Local Python subprocess/reference checks reran the EXACT combined query on MPEG-4
with/without B-frames, H.264, H.265, and a held descriptor after name replacement.
All returned one-second metadata and five adjoining frame intervals, no diagnostics,
and unchanged compared descriptor metadata. Replacement bytes stayed untouched.
Actual combined outputs for three codecs are included as Go parser-test fixtures.
Updated synthetic executable cases also ran via Python. Go compilation/tests,
gofmt, vet, race, full-branch and both Chrome runs have NOT run locally because
Go tooling is unavailable. These checks do not validate the Go implementation.
New-head Actions core/storage/lifecycle, both isolated Chrome acceptance runs and
aggregate required checks remain mandatory after separately approved push.

## Remaining before ANY positive cleanup authorization

Contiguous explicit intervals and stream duration do NOT prove that the camera
supplied every expected frame, decoded/source completeness, capture start mapping,
selected-prebuffer coverage, per-input/assembled correspondence or the requested
wall-clock interval. Those provenance/composition requirements, partial-release
receipts, remaining A01-A12 integration and whole-branch deployment/browser audit
remain open. This receipt must not be treated as a cleanup capability.
