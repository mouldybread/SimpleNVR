# Enforce frozen coarse-timeline coherence

Base: e792abebd59cd9f33d4096c2f50ba84efc6ddefb, user-reported green.
Main remains c5be6bf0cfea03071c7df1c3b0f5064753c881dd.

## Concrete next step

Existing segment notifications were copied into the pinned input selection and
reported diagnostically. This change CONSUMES those existing frozen observations
in the production bound presentation-evidence check. An incoherent chain can no
longer pass that necessary qualification check just because duration and the new
capture-versus-assembly length/frame-count comparison pass.

The new code is a pure bounded refusal predicate, not a new reporting schema,
telemetry collector, timing receipt, media process or cleanup permit.

## Required coherence

Require successful capture bookends and nondecreasing selection -> pin completion
-> Run entry -> Run return in BOTH Go's available monotonic ordering and UTC wall
ordering. A no-prebuffer job does not need nonexistent segment observations.

For selected prebuffer, require observed nonempty session IDs, nonzero sequences,
finite positive-length declared CSV media ranges, no prior uncertainty/backwards
flag, and ordered start/predecessor/receipt times no later than selection. Within
the selected chain require the same session and session-start instant, consecutive
nonwrapping sequences, exact predecessor-receipt correspondence and adjacent stored
CSV boundaries. A selection may begin in the middle of a session; it need not
include earlier unselected segments. Missing observations cannot be backfilled from
filenames, current ring state, Run duration or checkpoint diagnostics.

Equal receipt instants and timezone formatting changes are allowed. No epsilon,
slack, invented clock-jump threshold, media timestamp rewrite or repair is used.
Stored float CSV declarations are checked as stored, not promoted into exact real
packet boundaries or an ingestion-time mapping.

## Integration and unchanged safety boundaries

The two-line production hook follows e792abe's assembled-versus-captured check in
eventPresentationEvidence.check. Its existing live execution/selection binding
runs first, and descriptor checks follow. Existing terminal before/after checks
reuse this predicate without new reads or probes. Do NOT gate capture/assembly
entry on this new predicate: absence must refuse qualification, not erase a
successful recording/publication or invalidate recoverable evidence.

No existing source outside that hook changes. In particular, media arguments,
stdout handling, lease lifetimes, persistent descriptors, process count, query
entries, output caps, the TWO combined probes under ONE shared FIVE-SECOND budget,
checkpoint schemas, independent metadata grace, publication, slot return and
interrupt/grace/kill/join behavior remain unchanged. No hashing or per-frame
collection is introduced. Event staging is retained unconditionally.

## Tests and actual validation status

New pure regressions cover coherent/empty/interior selection, no mutation, timezone
identity, missing/uncertain observations, backwards ordering, session/sequence/start
changes, forged predecessors, CSV gaps/overlaps, NaN/infinity and resource bounds.

Synthetic production cases cover coherent, missing and gapped prefix observations,
continued capture/publication, consumption of frozen values after ring removal,
refusal before/after revalidation I/O, exactly two probes without terminal re-probe,
unknown/incomplete persisted coverage, preservation of that assessment during the
retention-only checkpoint, closed inventory, returned slots and released lease.
Synthetic metadata is NOT a real camera, media-equivalence or ingestion-clock test.
Existing tests/assertions/workflows are not weakened or modified.

Prepared artifacts have baseline-blob and patch-byte verification. The Go/race/core
and BOTH isolated Chrome suites are NOT reported as run for this candidate. Exact
new-head acceptance is required after a separately approved push. Baseline Actions
could not be retrieved through the available read path; green is user-reported.

## Deliberate limits and next unresolved acceptance

Passing means only that the OBSERVATION CHAIN has no detected incoherence. Segment
completion receipts are not packet arrivals; Run bookends are not first/last-frame
arrival bounds. Forward clock changes or stalls can preserve ordering and remain
unqualified: no stable-clock or maximum-delay guarantee is inferred. This does not
bound notification delay, stitch true capture-media bounds onto prebuffer, establish
requested-interval coverage, prove every prefix appears in output or validate audio.
All existing coverage unknowns remain, and no complete/positive authorization is set.

The remaining implementation/acceptance work is bounded ingestion/capture timing,
clock-transition uncertainty, ordered-prefix/output correspondence, then separately
reviewed positive eligibility and guarded same-lease/partial-failure release. Preserve
ownership acceptance and whole-project A-D audit closure after coverage work. No
cleanup activation, deletion, merge or deployment is included in this batch.
