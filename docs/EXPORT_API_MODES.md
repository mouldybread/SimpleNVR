# Export modes: integration and verification status

The HTTP planner and browser are wired to prepared copy/accurate exports. Copy is the default and never silently switches to re-encoding. Whole single-source copies preserve original bytes in prepared storage; compatible multiple sources preserve compressed A/V payloads when remuxed. This is not zero-copy original-file HTTP delivery.

Subsection copy supports non-reordered video with usable decoded keyframe timestamps. The server proposes video boundaries at keyframe/file boundaries, reinspects expanded coverage and returns copy_range_approval_required before admitting a job. The UI shows the requested/proposed intervals and AAC packet allowance and waits for explicit approval. Canceling a proposal creates no job. Approval retains the original selection and sends accepted_start/end for fresh server checks. Changed proposals require another click. Unsupported copy must be changed to Accurate explicitly.

AAC packet edges may extend outside the video-aligned interval by the disclosed allowance. Timing is metadata/frame/packet granular, not calibrated camera-clock time. Candidate copy outputs are duration-probed and externally decoded before readiness; this is not a proof of every source frame/sample or colour value.

Accurate explicitly re-encodes video with libx264 CRF 18 and audio with AAC. Both tracks share the video/cut origin; supported audio offsets are preserved. Full-range input becomes limited-range yuv420p, so accurate exports are not lossless or byte-identical. Leading/trailing silence can maintain the selected window without stretching audio. Mixed audio configurations, extra tracks, rotation and higher bit depths remain unsupported.

The live-container browser acceptance harness has been migrated to prepared jobs, real downloads and server release. Its container now mounts private export scratch. Branch-triggered Go and two-attempt Chrome workflows test the same pushed commit. See PREPARED_EXPORT_VERIFICATION.md for assertions and the distinction between local helper checks and actual CI results. The older routed-fixture browser script remains separate and unmigrated.

Release readiness is NOT established until both workflows pass on the final commit and failures are resolved. Nothing in this change merges, deploys or certifies real-camera/full-resolution/multi-GiB operation. Scratch settings, quotas, session isolation and leases remain described in LARGE_MEDIA_EXPORTS.md.
