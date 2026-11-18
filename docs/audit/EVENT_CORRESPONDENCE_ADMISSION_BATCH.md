# Full admission fault/recovery matrix — prepared candidate

Base: 72270f55f4668f61ce2637fd40f77dac5bec0994, green by user report.

This is an additive test-only batch, not production qualification or activation.

## Scope

The existing raw collector already tests child faults and live mutations. This
batch deliberately reuses those controlled children through corrAdmissionGate:
it tests the additional outer slot owner, refusal teardown and gate reuse.

- Thirteen child-failure modes with and without a selected prefix: 26 cases.
- Three sequential successful gate reuses after every failure: 78 invocations.
- Caller cancellation at each of six adapter hooks, with and without a prefix:
  12 cases, each followed by a successful fresh invocation.
- Attempt overlapping admission at all 12 cancellation boundaries; require busy
  refusal before binding or pipe allocation, even at late return boundaries.
- All returned reports must satisfy child/process/pipe/worker join assertions.
- Preserve the caller's held media lease, same duration receipt, staging tree
  and published bytes across each fault and its recovery sequence.

Total: 38 named leaf cases, 128 admitted invocations including recoveries, and
12 additional busy-refused overlap attempts. Loops remain serial because these
fixtures use process environment and shared media ownership. Deadline fault
cases use the existing 400ms controlled-child budget; success paths use no
extra sleeps. No workflow requirement is removed or marked continue-on-error.

## Limits and remaining acceptance

These are synthetic child/lifecycle tests, not additional native performance
measurements. They do not simulate true overlapping recorder jobs, prove
prebuffer content/time coverage, qualify codecs or establish a host budget.
Existing raw mutation tests, native full-adapter resource tests, comparison
profiles and browser checks remain necessary. Inspect the new green run's
bound-adapter.json before judging full-adapter resource cost acceptable.

Production code, media arguments, limits, cleanup, main and deployment are
unchanged. RuntimeQualified and CleanupAuthorized remain false. Candidate has
not been pushed or validated by CI; GitHub Actions is the test runner.
