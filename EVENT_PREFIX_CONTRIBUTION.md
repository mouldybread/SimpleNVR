# Minimum selected-prefix contribution guard

Prepared against b4a2dce063ed933b75588ba0e9a7b370ec3ff8ce, user-reported green.
Only target: mouldybread/SimpleNVR branch fix/recording-safety-audit.
Main remains c5be6bf0cfea03071c7df1c3b0f5064753c881dd.

## The concrete gap this closes

The e792abe check allowed a prefixed final with exactly the captured video's
presentation length and frame count. The b4a2dce coherence check could establish
only notification-chain coherence, not that any prefix reached the final output.
Together those checks could therefore still accept a capture-only final for a job
with selected prebuffer. The old coherent synthetic prefix fixture also used equal
capture/output summaries, accidentally representing that omission case.

This batch tightens the EXISTING assembly evidence path; it introduces no new
reporting layer or observation collector. For selected prebuffer require:

* a strictly longer observed continuous presentation length than capture;
* at least one additional observed video frame per frozen selected input.

For example, two selected inputs require at least two extra frames, not merely a
longer file or one extra frame. The number comes from the existing live bound frozen
selection, not the current ring, a client field or the stored coverage diagnostic.
No-prefix jobs retain exact video-count/length remux equality, allowing rebasing.

## Implementation and arithmetic

Tighten eventAssemblyPresentationCompatible's prefix branch from non-strict to
strict length/count inequalities, retaining its exact rational math/big comparison.
Add eventAssemblySelectedPrefixCompatible to apply the bounded selected-input
count. Reject negative/over-limit counts before checking summaries; subtract only
already-positive, already-capped frame counts. No floating time tolerance is added.
Replace ONE call expression in eventPresentationEvidence.check. The live binding,
individual duration/continuity checks, b4a2dce coherence predicate, descriptor checks
and existing before/after same-lease revalidation remain in their established order.
The entire strict JSON decoder remains byte-for-byte unchanged.

The required frame budget is a necessary VIDEO qualification condition: selected
inputs with no usable video contribution cannot establish video prefix coverage.
Refusal does not undo publication, remove staging, or pretend an audio-only input
proved video coverage. Successful but ambiguous media remains available for recovery.

## Tests

Strengthen the old prefix-equality assertion from acceptance to refusal. Add pure
cases for input-count bounds, exact no-prefix behavior, insufficient extra frames,
strict span increase, rational rebasing, sub-float-precision changes and frame caps.

Extend the existing synthetic production prefix matrix, rather than building a new
fixture layer. Its coherent output now explicitly represents two five-second prefix
frames followed by the existing 30-second capture frame. Capture-only, duration-only,
frames-only and insufficient-extra-frame variants must refuse assembly qualification.
Missing/gapped clocks still reach and exercise the existing coarse-timeline refusal.

Exercise the frame budget before/after revalidation I/O and restoration of genuine
observations. Retain existing publication, unknown coverage, retention-only checkpoint,
closed inventory, same-lease/slot teardown and exactly-two-probe assertions. No negative
clock or retention assertion is removed. Synthetic media/metadata is explicitly NOT
real-camera, codec-version, decoded-frame or ordered-content acceptance.

## Unchanged resource and authorization boundaries

No new probe, query entries, media command/flag, decode, per-frame collection, hash,
connection, descriptor, lease, goroutine, mutable ledger or persistence schema.
There are still TWO combined probes under ONE shared FIVE-SECOND inspection budget.
Publication/checkpoint sequencing, independent metadata grace and interrupt/grace/
kill/join behavior are unchanged. Event staging retention is unconditional.

Passing these aggregate bounds does NOT identify which prefix supplied each frame.
One duplicated/overlong input could still satisfy them while another is omitted.
This is NOT full prefix correspondence, input-arrival mapping, audio preservation,
stable-clock proof or requested NVR-time coverage. All coverage unknowns stay; no
complete bit, positive eligibility or cleanup activation is introduced.

## Validation and next acceptance

All four edited baseline blobs are verified against the remote SHA. Patch application
and exact candidate/payload bytes are checked separately. Go/race/core and BOTH isolated
Chrome runs have NOT been executed for this candidate. Baseline green is user-reported;
the available Actions page read failed. Exact new-head acceptance is required after a
separately approved push. Thread permission is not exact-content authorization.

Remaining work still includes bounded ingestion/capture timing and clock transitions,
full ordered-prefix/output correspondence, separately reviewed positive eligibility
and guarded same-lease/partial-outcome release, then ownership and whole-project A-D
acceptance. No merge, deployment, deletion or cleanup action is included.
