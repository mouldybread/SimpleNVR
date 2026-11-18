# Two-process streaming correspondence: paired CI experiment

Prepared on 3ad0281b2c54cbb5bd77cbd1819c88c619433f59, user-reported green. This stage adds paired native correctness checks and resource measurements; exact-new-head CI and measurement review remain pending. It does not change production code, workflows, limits, recording flags, checkpoint eligibility or cleanup behavior.

## Comparison and resource contract

The original four-process scratch collector remains unchanged as a control. The streaming candidate starts two FFmpeg children, extracting video and audio together and comparing through pipes without normalized scratch files. Both children can run concurrently; fewer launches does not imply lower peak CPU usage.

Streaming owns four OS pipes (eight initial parent descriptors), two comparator workers, four read workers and a cancellation watcher. All are closed or joined before return. Existing media descriptors remain borrowed. Four 64 KiB content buffers total 256 KiB; kernel pipe buffers, FFmpeg memory, stacks, diagnostics and runtime overhead are additional. Zero NormalizedScratchBytes means no owned normalized scratch writes, not zero physical I/O or zero source reads.

The 32 MiB normalized-stream ceiling, 128 MiB per-source limit, conservative aggregate admission, aggregate experimental 20-second collection deadline and two-second command pipe-join grace remain unchanged. At most one sentinel byte beyond a stream ceiling is read to establish refusal. Production's separate probe/resource contract is not expanded.

## Correctness and safety

Both implementations inspect the same ten native media fixtures. Normal and timestamp-shifted outputs must match. Omission, duplication, reordering, truncation and stream-specific reordering must be detected. Original scratch assertions are preserved. Streaming child/worker joins, owned-pipe closure and resource counts are checked.

The preceding safety stage covers byte boundaries, diagnostics, nonzero exits, partial launches, stalls, cancellation and 66 synchronous live-job mutation cases. The opposed-order fixture now explicitly constrains its controlled child pipes below a 64 KiB read chunk rather than assuming default kernel pipe capacity. It requires deadline refusal and complete teardown; setup errors must not masquerade as success. Finite buffering can still refuse some valid opposed interleavings; this is not a guarantee that all interleavings complete.

## Six paired observations

Keep the existing three generated profiles, each repeated twice: 14s 360p/1 Mbps, 110s 720p/2 Mbps, and 90s 1080p/4 Mbps. Each measurements.json row retains the scratch result and adds a nested Streaming result. Scratch always runs first; both variants use generated, cache-warm inputs. Fixed order and two repetitions do not constitute an unbiased general benchmark.

Record elapsed time including streaming teardown, separate child and collector CPU, kernel block-operation deltas, logical input-pass accounting, normalized bytes read, process/pipe/worker counts and owned normalized scratch bytes. No physical-device-byte, cold-cache, per-comparison RSS or full live-job adapter-cost claim is made. The existing historical children-RSS field is a process-lifetime high-water mark, not a paired memory measurement.

The smaller profiles must match with both implementations. The oversized profile must preserve the scratch writer-cap refusal and produce explicit streaming byte-cap refusal, not a deadline or unrelated error. Failure is not accepted merely because some error occurred. Completed rows are retained on subtest failures. Existing artifact names, upload behavior and required core/browser/resource gates are unchanged.

## Acceptance boundaries

No speedup or CPU/I/O improvement is claimed before reviewing exact-new-head evidence. RuntimeQualified and CleanupAuthorized remain false. Timestamp normalization intentionally permits shifted timestamps: content equality adds no NVR-ingestion-time coverage, source-identity or cleanup proof. Do not merge, deploy, enable cleanup or integrate the comparator into production on these results alone.
