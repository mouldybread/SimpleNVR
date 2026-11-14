# Strict presentation JSON decoding — NEW IMPLEMENTATION

Base: af816b4bb8bbb9ba8314289fead0714240eef41e.
Base green is user-reported, not an independently retrieved Actions result.

## Scope and wire contract

This is an opt-in in-memory JSON decoder feeding the existing exact continuity
validator. It has NO production caller and is NOT a descriptor-bound media reader.
No process, lease acquisition, descriptor, goroutine, checkpoint, schema migration,
cleanup authorization or deletion is added. Production behavior remains unchanged.

The complete JSON document must contain exactly streams and frames. Streams must
contain exactly ONE video stream with index, codec_type and time_base. Each frame
must contain exactly media_type, stream_index, pts and duration; media_type is
video and stream_index must equal the declared nonnegative stream index. Every
object key is case-sensitive, required and unique, including escaped duplicates.
Unknown fields refuse. Field order may vary; frame order is never rewritten.

Integer fields must be unquoted int64 JSON integers, not decimals, exponent forms,
strings, null, booleans or invented default zeroes. Zero PTS is valid when present.
Positive rational time-base components are exact integers. Durations must be
explicit and positive. There is no best_effort_timestamp, pkt_duration, format
length or other alias/fallback. Future process wiring must explicitly request and
validate this contract; compatibility with actual FFprobe versions is NOT proven.

A four-MiB input ceiling rejects oversized observations before parsing; it never
accepts a truncated prefix. The existing 1,048,576-frame ceiling also applies.
The bounded byte slice is already in memory; this is not an interruptible I/O API.
Context checks occur at entry, object fields, frame boundaries, continuity checks
and completion. Standard-library JSON operations inside those bounded regions
are not themselves interruptible. Input bytes are neither mutated nor retained;
only scalar stream identity and continuity arithmetic escape on success. Any
refusal returns a zero result, never a useful partial span.

## Completeness is NOT inferred from JSON EOF

The parser requires the entire document and JSON EOF, rejecting malformed tails,
extra documents, missing fields and syntactically truncated arrays/objects. It
cannot detect a producer that silently emits a valid shorter frame array. A test
explicitly preserves this distinction: a valid one-frame observation parses but
cannot reach the fixture's two-second lower bound. No result is an authenticated
receipt, proof of all decoded frames or proof of source/process completion.

The existing continuity validator rejects gaps, overlaps, duplicates, reordered
intervals and overflowing ends. Signed/nonzero origins remain visible. Passing
length and continuity still does not prove any requested wall-clock interval.

## Validation and remaining work

New regressions cover strict keys/types, duplicate and escaped keys, mixed/wrong
streams, missing durations/PTS, invalid rationals, integer overflow, every truncated
prefix, trailing data, cancellation at every observed checkpoint, exact/over byte
ceilings, synthetic shorter valid JSON, and fuzz seed invariants. No media or
browser acceptance is claimed. Go/gofmt are unavailable in the preparation runtime;
new Go tests, vet, race, formatting and full-branch checks have NOT run.

Next: a separately reviewed descriptor-bound complete presentation reader with
explicit process/EOF provenance, source and stream identity, aggregate bounds,
actual-format fixtures, and before/after inventory/lease/job/manager checks. Adding
or changing media processes requires explicit separate review; this batch does not
authorize it. Then compose per-input and assembled ordering, selected-prebuffer
coverage and trustworthy requested wall-clock interval provenance before ANY
positive cleanup eligibility. Preserve independent metadata budgets, publication
and persistence-failure semantics, unconditional staging retention, descriptor
ownership, terminal lease handoff and slot return. Both Chrome acceptance runs,
core/storage/lifecycle and aggregate required checks remain mandatory. No workflow
changes, main writes or merge are included.
