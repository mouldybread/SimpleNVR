# Automated correspondence evidence assessment

Base: 4d7270149109d0a94687ab28da74387fcf703690, green by user report.

The standard-library-only analyzer reads bound-adapter.json and
native-prebuffer.json, requires the explicit full expected commit SHA, rejects
duplicate JSON keys and nonfinite metrics, limits input size and records source
SHA256 values. These are diagnostic reports, not authenticated owner receipts.

It verifies the unique eight-row full-adapter and eighteen-row native matrices,
expected refusal layers, successful positive comparisons, lifecycle counts,
generated/requested lengths and paced-background liveness and teardown.

It summarizes full-adapter median/max elapsed time and observed child/collector
CPU per profile and idle/paced condition. Two cache-warm samples do not support
tail-latency claims. Background lifetime CPU is separate; zero kernel block
operations do not prove zero logical reads. No host threshold is invented.

Usage:
python3 tests/acceptance/correspondence_assessment.py EXTRACTED_ARTIFACT_DIRECTORY \
  --expected-head FULL_COMMIT_SHA --output assessment.json --summary assessment.md

The workflow runs regression tests and assessment after resource collection,
even on failure, then publishes the Markdown summary before the existing
unconditional artifact upload. Missing or inconsistent evidence causes a failing
assessment exit status. No existing required check is removed or weakened.

Regression fixtures cover complete reports, missing/duplicate rows, refusals,
unjoined work, invalid metrics, background failures, wrong refusal layers and
mismatch errors, nil refusal, changed fixture lengths, wrong heads, duplicate
JSON keys and unauthorized flags. CI must validate this new batch.

Production code, media commands, budgets, cleanup, main and deployment remain
unchanged. Runtime qualification, cleanup authorization and resource-cost
acceptance remain false after successful assessment. Actual successful-run
numerical review is still pending; this batch does not claim to retrieve it.
