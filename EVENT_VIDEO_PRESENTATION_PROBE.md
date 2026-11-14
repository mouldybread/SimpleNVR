# Opt-in held-descriptor presentation probe — NEW IMPLEMENTATION

Base: ec6bb13959dc5c77c38f90790ac0285dc1118b87.
Base green is user-reported. No independent Actions result was retrieved.

## Scope and process boundary

This batch adds an OPT-IN subprocess component and tests, not production wiring.
It has no production caller and does NOT add runtime probes to finalization or
revalidation. Existing up-to-two duration probes, their aggregate deadline,
publication, persistence, terminal handoff and unconditional retention are unchanged.
No new lease acquisition, deletion, cleanup permit or merge is enabled.

IMPORTANT REVIEW BOUNDARY: when explicitly called, the new component runs ONE
additional read-only FFprobe decoding process on a borrowed descriptor. It is not
implicitly authorized for production use by this batch. Before integration, decide
whether/how to combine it with or replace existing observations without exceeding
the approved production process count and shared aggregate budget. No job-bound
presentation receipt or positive cleanup authorization is introduced here.

## Held descriptor, bounded observation, conservative refusal

The caller must serialize descriptor/lease lifetime and provide the currently held
recordingMediaOwnership lease. Nil/foreign/released leases, nil/closed descriptors,
empty/nonregular/unlinked files and canceled parents refuse before spawning.
The child receives the held descriptor as fd 3 and uses /proc/self/fd/3, MOV input,
file/pipe protocols, video stream zero and explicit frame presentation fields.
There is no pathname rediscovery, shell, fallback timestamp or inferred duration.

The component has a five-second maximum context that can only shorten an existing
caller deadline, plus two-second child pipe WaitDelay. A future multiple-file
caller must supply ONE aggregate context; per-file renewals are forbidden.
Kernel I/O is not made interruptible. Stdout is capped at four MiB and diagnostics
at 16 KiB, refusing overflow rather than accepting a prefix. Nonzero exit, context
failure, ANY error-level stderr, invalid JSON or continuity failure returns zero.

Device, inode, mode, size, link count, mtime and ctime must agree before the child,
after its completion and after parsing. The caller's descriptor/lease are never
closed or released. This is inspection-time identity, not immutable-content proof
against hostile writers, job/inventory provenance, future-lifetime validity or a
cryptographic content receipt. Access time may change through read-only I/O.

## Actual-format finding and narrow envelope adapter

Local media tools exist even though Go/gofmt do not. FFprobe 8.0.1-3ubuntu2 emitted
explicit pts/duration fields on generated MPEG-4 MP4, plus empty programs and
stream_groups arrays. The canonical strict parser correctly rejects these extras.
A separate adapter permits ONLY those two known optional EMPTY arrays. Unknown,
nonempty, null and duplicate envelopes still refuse. The canonical parser and its
existing tests remain unchanged. No pkt_duration or best-effort alias is added;
versions without the exact frame-duration contract remain unsupported and refuse.
The checked-in actual-output fixture records this one tested version, not universal
FFprobe compatibility, all codecs, RTSP behavior or complete camera coverage.

## Validation and remaining integration

The exact candidate query ran successfully on generated MPEG-4 without B-frames,
with B-frames, and through a held descriptor after pathname replacement. Each
returned five adjoining frames spanning exactly one second, no error diagnostics,
and unchanged compared descriptor metadata. Replacement bytes stayed untouched.
These subprocess observations were executed through Python, NOT the Go component.
A fixture regression, adapter/buffer checks, synthetic process/diagnostic/overflow/
mutation failures, admission, cancellation and borrowed-lifetime tests are included.
Go compilation/tests, gofmt, vet, race and both Chrome runs have NOT run locally.

Next bind complete presentation observations to exact job/request/execution,
inventory and published-output identities before/after I/O under a reviewed shared
budget/process policy. Compose per-input/assembled order, prebuffer coverage and
trustworthy wall-clock provenance. JSON EOF/process success is not proof that the
source had every expected frame or that camera footage covers the request.
Positive cleanup eligibility, release/partial receipts and whole-branch acceptance
remain open. Preserve independent metadata budgets, sticky persistence failures,
descriptor ownership, terminal lease handoff and slot return. Main stays untouched.
