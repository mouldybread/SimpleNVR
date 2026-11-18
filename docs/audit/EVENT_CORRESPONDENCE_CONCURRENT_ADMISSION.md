# Concurrent admission ownership matrix — prepared candidate

Base: ba1a2c162f3a29f3da54fd21b2d4cd0fdab4b3fd, green by user report.

The previous batch tests nested busy calls and sequential recovery. This batch
adds real goroutine contention around the same test-only admission gate.

## Matrix and assertions

- Selected prefix absent/present, six collector hooks, owner success/cancel:
  24 leaf cases.
- Hold the admitted invocation at the selected hook using channels, not sleeps.
- Release 16 contender goroutines from one start barrier while the owner is held.
- Require every contender to return the busy sentinel with zero children and
  zero pipes. Nil contender evidence verifies busy refusal precedes binding.
- Join contenders before allowing the owner to proceed or cancelling it.
- Assert owner child/worker/watcher joins and closed pipes on both outcomes.
- Require a subsequent successful inspection through the exact same gate.
- Preserve the caller's held lease, same receipt, staging tree and output bytes.
- On terminal fixture teardown, assert inventory closure, lease/slot release and
  unconditional staging retention.

Planned totals: 384 busy-refused contender attempts and 48 owner/retry calls.
Contenders never access live evidence; the sole admitted owner does. Fixture
setup and subtests remain serial because they share process environment and
media ownership. Cleanup unblocks and joins the owner before fixture teardown,
including when a test assertion fails. Existing five-second admission budget
is unchanged. The test's ten-second caller context cannot extend that budget.

## Execution and boundaries

The ordinary full Go/race suite discovers this additive test automatically;
no workflow change or weakened requirement is proposed. CI must validate it.
The synchronized hold tests ownership at distinct boundaries, not randomized
stress, real recorder overlap, filesystem latency or scheduler performance.

Production code, budgets, media commands, cleanup and deployment remain
unchanged; main remains unmerged. This does not qualify native prebuffer
content or ingestion-time coverage, nor accept the comparator's runtime cost.
The successful native resource run's bound-adapter.json still needs review.
No new resource acceptance is inferred from a green test status.
