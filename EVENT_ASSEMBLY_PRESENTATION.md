# Captured-versus-assembled video presentation guard

## Baseline and scope

Prepared after b54420751a3afc6130e53484cb5a22dca04ed2c6, user-reported green.
This is a cross-file necessary-condition check, not a restart of the shipped
individual duration/continuity checks and not a new coverage-report layer.

Before this change, capture and publication each had to reach the requested video
length. A 40-second capture and a 30-second published video could both satisfy a
30-second request. That does not establish preservation of the captured component.

## Added production condition

Reuse the capture and published continuous-presentation summaries already produced
by the existing two descriptor-bound inspections. Compare their frame counts and
exact presentation lengths, not wall-clock Run times or rounded/container durations.

With no selected prebuffer, concat only remuxes event.mp4. Require equal observed
video frame counts and equal continuous presentation lengths. Different timestamp
origins and rational timebases are allowed: only lengths are compared.

With selected prebuffer, require published count and length to be at least those
of capture. The presence of prebuffer is derived from the bound frozen selection,
not a request bit or mutable buffer lookup. Equality can pass this LOWER-BOUND check;
it does not prove that any/every prebuffer clip was included. Full ordered-prefix
correspondence remains unverified. No content or decoded-frame equality is claimed.

Integer subtraction and cross-products use math/big to preserve exact arithmetic
across int64 limits and different timebases. There is no epsilon, rounding allowance,
media timestamp rewrite or wall-clock accuracy assertion.

## Existing ownership and process bounds

The condition runs inside the existing eventPresentationEvidence.check after live
job/receipt binding and individual duration/continuity checks, before descriptor
metadata revalidation. Existing before/after terminal checks therefore also reuse it.

No media command, query entries, child count, descriptor, lease, timeout, output cap,
JSON decoder, checkpoint schema, checkpoint ordering or publication behavior changes.
There are still TWO combined probes under the SHARED FIVE-SECOND inspection budget.
No additional per-frame collection, hashing, media decode or mutable ledger is added.
Only scalar metadata already retained by the current implementation is compared.

Refusal flows through the existing evidence-error/all-retain path. Successful
publication is not rolled back, retained bytes are not deleted, and independent
metadata grace, same-lease terminal teardown, descriptor closure and slot return
remain required. The b544207 retention-only assessment-preservation fix is untouched.

## Tests and evidence limits

New arithmetic regressions cover capture longer than publication despite both
meeting the requested duration, frame-count loss, no-prefix unexplained extension,
allowed timestamp/timebase rebasing, a one-tick shortfall, fractional precision,
int64 overflow and malformed summaries.

Synthetic production tests exercise bound revalidation before/after I/O, restoration
of genuine observations, exactly two probes, preserved published checkpoint/output,
unconditional staging retention and lease/descriptor/slot teardown on refusal.
They are not camera, codec-version or decoded-equivalence acceptance. Existing Go
package discovery runs them without a workflow change. Tests are prepared, not
reported as executed; exact-head Actions core and BOTH isolated Chrome runs remain
required after a separately approved push.

This guard does NOT complete requested NVR-time coverage, prove source content,
verify every prefix, validate audio correspondence or authorize cleanup. Existing
coverage diagnostics must remain unknown; no unknown reason is cleared by this
necessary condition. Next work remains the coarse timeline/capture boundaries and
ordered-prefix/output correspondence, then separately reviewed positive eligibility,
guarded release/partial outcomes, ownership acceptance and whole-project A-D closure.
No merge, deployment, deletion or release-engine activation is authorized.
