# Ordered-correspondence resource qualification candidate

Prepared against 48d4250d574cb4cecc6444192fe60c0b57245e53, user-reported green.
This batch is not pushed. No production source, recording flag, ownership rule, cleanup eligibility, or inspection limit changes.

## Purpose

Measure the existing prototype's normalization/comparison cost for longer generated H264/AAC clips. These are CI-runner measurements, not representative-camera/deployment acceptance. The full live-job adapter is NOT invoked by this measurement harness; its binding/preflight and same-lease costs are excluded. Existing safety/core/race and two isolated Chrome jobs remain unchanged.

## Profiles and repetitions

- 640x360, 10 fps, nominal 1 Mbps video: two 2-second prefixes and 10-second capture, total 14 seconds.
- 1280x720, 15 fps, nominal 2 Mbps: two 10-second prefixes and 90-second capture, total 110 seconds.
- 1920x1080, 25 fps, nominal 4 Mbps: two 10-second prefixes and 70-second capture, total 90 seconds; intended to exceed the UNCHANGED 32 MiB normalized-stream ceiling.

Sources use stock libx264 CBR/HRD and AAC, generated in private test directories. The codec/filter support must exist; the dedicated job fails rather than skips if tools are missing. Fixture generation has a 120-second context per profile plus bounded join. Each output requests FFmpeg's -fs limit and is checked before collection; -fs is not described as an exact hard filesystem quota. No cold-cache claim: encoding/muxing already populate cache, and both repetitions reuse those files. Two measurements per profile, six required rows. There is no performance percentile claim from two samples.

## Reuse, limits, outcomes

The harness calls the green corrPrototypeNormalize and corrPrototypeEqual helpers, not a separate normalizer implementation. Four sequential children for a successful video/audio comparison, same 20-second experimental aggregate collection context and existing byte limits. No concurrent measurements or race instrumentation in the resource job; the ordinary core job retains race testing. Neither this experimental budget nor its measurements replace production's existing two-probe/five-second policy.

The two smaller profiles must complete with matching streams and four children. A slower runner can therefore fail this experimental test; that failure must be inspected, not reclassified as success or fixed by silently extending limits. The large profile must refuse on the first extraction near the stream writer's existing byte cap, with no deadline error; missing codecs, early admission failure and unrelated early errors do not count as the expected refusal. Reports retain the actual error, counts and elapsed time. A refusal does not mean content was verified.

## Measurements and their meaning

- Fixture-generation elapsed seconds, separated from comparison elapsed seconds.
- Selected-source file bytes and logical two-pass input-file-size budget (the latter includes published output). Logical size is NOT actual bytes read by FFmpeg.
- Actual normalized bytes accepted by the scratch writer and bytes read by the Go byte comparator.
- Actual normalizer child starts, borrowed media descriptor count and scratch descriptor peak; existing report counts still exclude os/exec pipes and inventory transient descriptors.
- Linux Getrusage CPU deltas and input/output block-operation counters for waited children and the Go collector separately. Parent counters matter because Go writes scratch files from the child stdout pipe. These are kernel accounting observations, NOT direct physical-disk byte or latency measurements; cache/writeback/filesystem behavior can yield zero block counters.
- Child maximum RSS as reported cumulatively since the test process began. It includes fixture-generation children and earlier profiles; it is explicitly NOT a per-comparison memory peak or delta.

No dropping of host caches, privileged disk tracing, camera access, custom FFmpeg, per-packet hashing, deployment or recording cleanup occurs. Full device-level I/O, real storage contention and production-host performance still require a separately controlled deployment experiment.

## CI integration

A separate correspondence_resources job compiles the exact checkout test binary without race instrumentation and runs only TestCorrResource* with the resource environment flag set. Ordinary test runs skip the long matrix but still validate profile definitions. The required aggregate verify job now requires core, both browser attempts and resource measurements. No continue-on-error is introduced.

The new job records exact GitHub head, Go/FFmpeg versions, runner kernel and filesystem context, JSON rows and verbose logs in a head/run-attempt-named artifact retained for 14 days. Upload is attempted on failures too. A hard timeout can prevent deferred JSON generation; the job must then remain failed, with available logs, rather than claiming six measurements. Successful matrix completion requires six rows. Existing dependency-readonly checks remain.

## Acceptance and next decision

Prepared code and workflow, not measured results. No timings or throughput are asserted for this candidate. After exact-content approval/push, inspect the exact new-head resource artifact alongside core/race and both Chrome results. Green would mean these synthetic cases met their stated expectations, not that live deployment or production budget fit is established. Use those observations to decide whether to reduce passes, propose separately bounded runtime costs, or retain offline-only use. NVR-ingestion-time mapping and cleanup authorization remain separate unfinished gates.
