# Export modes

The HTTP planner and browser use prepared Copy and Accurate exports. Copy is the default and never silently changes to re-encoding. A single complete source is copied byte for byte into prepared storage. Compatible complete sources are remuxed with compressed audio/video payloads preserved. This is not zero-copy original-file delivery.

## Subsection copy and approval

Copy of part of a recording supports non-reordered video with usable keyframe timestamps. The server proposes video boundaries at keyframe or file boundaries and rechecks the expanded range for gaps/overlaps. It returns `copy_range_approval_required` before creating a job. The browser shows requested/proposed video intervals and the AAC packet allowance and waits for explicit approval. Cancelling the proposal creates no job.

Approval retains the original selection and sends `accepted_start`/`accepted_end` for fresh server checks; changed video boundaries need another approval. Unsupported copy must be changed to Accurate explicitly. AAC packet edges may extend outside the video-aligned interval by the disclosed allowance.

## Accurate and timing

Accurate uses libx264 CRF 18 and AAC encoding, sharing the video/cut origin between tracks and preserving supported audio offsets. Full-range input becomes limited-range `yuv420p`, so the result is neither byte-identical nor lossless. Leading/trailing silence can maintain the selected window without stretching audio. Mixed audio configurations, extra tracks, rotation and higher bit depths are unsupported.

Timing is frame/packet-granular, not calibrated camera-clock time. Duration probes and candidate copy decode do not prove completeness or fidelity of every frame/sample.

## Setup and evidence

The live-container browser harness now uses prepared jobs, normal downloads and server release with private export scratch configured. Go and Chrome checks passed on PR #8's recorded combined head; see [prepared export verification](PREPARED_EXPORT_VERIFICATION.md) for the exact result and scope. The older routed-fixture browser script remains separate and unmigrated.

See [large-media exports](LARGE_MEDIA_EXPORTS.md) for scratch settings, quotas, session isolation, leases and request fields, and [export rendering](EXPORT_RENDERING.md) for implementation limits. Synthetic CI results do not qualify real-camera/full-resolution/multi-GiB or heavy-load operation. A new release or deployment still needs checks appropriate to its exact candidate and storage.
