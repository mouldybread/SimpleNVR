# Native resource fixture presentation boundary

Base: 16edded98d893094a0552e2334ee604e44f4c8e0.

The uploaded CI diagnostics decode successfully but show shortened remuxed
presentation intervals. The 360p output starts at tick 215 and ends at 143360
in time base 1/10240; the 720p output starts at 323 and ends at 1689600 in
1/15360. Stream duration metadata still reports 14 and 110 seconds.

Headroom alone is insufficient: no-prebuffer assembly also requires exact
capture/output presentation-length and frame-count agreement.

This batch changes only tests and this document:
- Align the fixed 48 kHz synthetic audio input by 1024 samples before native
  AAC encoding, compensating its priming for this controlled success fixture.
  This is not an adaptive fallback or a production media-command change.
- Keep generated video profiles at 14 and 110 seconds; request 13 and 109
  seconds and record both lengths explicitly in each measurement row.
- Keep exact production duration, continuity and assembly checks unchanged.
- Add pure regression cases for exact length, equal-length timestamp rebasing,
  the observed CI short-final-frame shape and a one-tick shortfall. Sufficient
  stream metadata must not promote a short observed interval to sufficient.
- Add failure-only bounded native observations logging first/end PTS, time
  base, count, duration sufficiency and assembly compatibility. Newly opened
  diagnostic paths never replace descriptor-bound owner evidence or receipts.

Existing resource matrix remains two profiles, idle and paced segment writer,
with two repeats each (eight required rows). Existing comparison profiles and
workflow requirements remain unchanged. GitHub Actions must validate the
fixture correction; further native incompatibilities remain possible.

Runtime qualification and cleanup authorization remain false. Staging is
retained; main, merge and deployment are outside this batch.
