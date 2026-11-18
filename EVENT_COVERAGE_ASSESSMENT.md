# Event-level coverage assessment after 321d69b

## Purpose and exact scope

321d69bfd873bdeedfd5560531a508640b7d8179 is user-reported green, not independently
verified Actions evidence. That commit collects coarse segment notification timing.
This change consumes its frozen production observations in an event-level diagnostic
that survives a successful existing checkpoint write and appears in the existing
read-only recovery inspection. No new media connection/output/process/probe/hash,
lease, descriptor, worker, endpoint or video-timestamp rewrite is added.

## What the assessment means

The optional coverage_assessment field records the requested interval, frozen
selection availability, counts of observed/missing segment notifications, declared
CSV gaps/overlaps, session/sequence changes and clock warnings. Available first/last
notification samples are explicitly completion-notification receipt times.
Existing capture Run bookends are copied with a run-state label, not media bounds.

The interval calculation unions a LOCAL COPY of the frozen prebuffer declarations,
clips it to the requested half-open interval [start,end), and lists intervals not
covered by those declarations. Sorting this copy never reorders assembly inputs.
These are declared_prebuffer_gaps_in_request, NOT proven missing event footage:
event capture may contribute, but its media bounds are still unobserved. No guessed
clock offset, buffering tolerance, capture-duration fill or forward-clock health
claim is used. Invalid declarations never become covered ranges.

Status is always unknown and complete is always false in this implementation.
Three unresolved conditions remain explicit: unbounded segment-notification delay,
unobserved capture media bounds, and unverified assembled-output correspondence.
A gap-free declaration union or successful child process cannot clear them.
This report is NOT positive eligibility or a cleanup capability.

## Production persistence and ownership

The event owner snapshots the assessment immediately before json.Marshal in an
EXISTING recoveryCheckpoint call. No additional checkpoint/write/Sync is scheduled.
The existing writer, 65,536-byte cap, owned replacement ordering, persistence-failure
stickiness, checkpoint byte receipts, metadata grace and lease handoff remain intact.
The report is recomputed from the frozen request/selection and existing Run snapshots,
never by scanning the mutable buffer or reopening media paths. Missing/changed job,
manifest, inventory or selection bindings downgrade the report instead of blessing
substitution. Legacy/no-selection jobs omit the field and keep their old JSON shape.
Memory state after failed persistence is not a durable coverage report.

The optional manifest field and stored_coverage_assessment inspection field are
explicit schema additions. Existing version-1 manifests without the optional field
remain readable. An OLDER binary's strict inspector may reject newer manifests;
backward readability by older binaries is NOT claimed. Do not deploy or roll back
without separately reviewing that compatibility boundary.

## Strict read-only inspection

The inspector recognizes the optional field, rejects unknown/case-aliased/missing/
null nested keys, checks bounded counts and consistent request bounds, recomputes
assessed declaration gaps from the manifest, and refuses complete=true or a success
status. Existing duplicate-key, UTF-8, byte, token and depth limits remain unchanged.
The existing read-only inspection surfaces a STORED diagnostic, not independently
verified source/media/UTC facts. inspection_complete still means the inspection
finished, NOT that recording coverage is complete. No new UI or HTTP route is added.

## Bounds and acceptance

At most 128 frozen input declarations and 129 declaration gaps are processed. The
assessment stores aggregate notification information, not a per-packet ledger or
a duplicate per-segment media log. Input union work is O(n log n), n<=128, with no
media I/O. The maximum-gap regression exercises existing byte/token/depth limits.

New tests cover clipped/overlapping/out-of-order declarations, unknown capture
bounds despite long successful Run bookends, real production checkpoint/inspection
wiring, frozen selection, refused bindings, strict schema refusal, legacy manifests,
maximum input/gap counts, sticky failure, staging retention and slot teardown.
Tests are prepared for existing Go package discovery, not reported as executed.
Application acceptance requires exact-head Actions core/race/vet/storage/lifecycle
and BOTH isolated Chrome runs. No workflow change is included.

## Finite remaining path

Next close the actual coarse timeline policy/boundary and assembled-output coverage
conditions rather than adding packet tracking or pretending this report proves them.
Keep unknowns and all-retain behavior whenever available evidence is insufficient.
Positive cleanup eligibility, same-lease engine activation and explicit partial
outcomes require separate review. Then finish ownership acceptance and whole-project
A-D residuals/final candidate checks. No merge, deployment or deletion is authorized.
