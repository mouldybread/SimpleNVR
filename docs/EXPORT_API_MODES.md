# Export modes: current integration status

The HTTP planner calls the prepared copy/accurate renderers and the browser offers both modes. This status supersedes older staged progress notes saying those paths are not yet wired. The branch is still NOT release-ready: Go compilation/tests, real-browser harness migration and full runtime validation remain outstanding.

Copy is the default and never silently falls back to re-encoding. Whole recordings are copied byte for byte into prepared storage (single source) or remuxed with compatible compressed A/V payloads (multiple sources). This is not yet zero-copy original-file HTTP delivery.

Subsection copy is limited to non-reordered video with usable decoded keyframe timestamps. The server rounds video boundaries to keyframe/file boundaries, reinspects expanded coverage and returns a 409 JSON copy_range_approval_required proposal BEFORE job admission/storage reservation. The UI displays requested and proposed local dates/times and AAC packet allowance, and waits for Use proposed copy range. Canceling the proposal creates no job. Approval sends accepted_start/end with the immutable original range; the server recomputes the proposal and refuses stale boundaries. Changed proposals require another click. Unsupported/reordered subsection copy must be changed to Accurate explicitly.

AAC packet edges can extend outside the video-aligned interval by the disclosed packet allowance. These are metadata-derived/frame-packet granular boundaries, not calibrated camera-clock claims. Copy candidates are duration-probed and externally decoded before readiness; this does not certify every source frame/sample or colour value.

Accurate mode explicitly re-encodes video (libx264 CRF 18) and AAC audio. It shares the video timestamp/cut origin, preserves supported audio offset, and converts full-range input to limited-range yuv420p. It is not lossless or byte-identical. Missing leading/trailing audio may be represented as silence to maintain the selected window, without audio stretching. Mixed audio configurations, extra tracks, rotation and higher bit depths remain unsupported.

The UI uses normal authenticated download URLs, not video Blobs. Cancellation/release requires server acknowledgement. Scratch configuration, server limits, session isolation and lease lifetime remain documented in LARGE_MEDIA_EXPORTS.md.

New focused mode/approval UI regressions passed in Node in UTC and America/Chicago using a DOM fixture; syntax checks also passed. Go tests are added but have not been executed here. Old real-browser harnesses still contain Blob-era assumptions and need migration while retaining their media, hash, duration, playback and cancellation assertions. Large-file, slow-client, full-resolution real-camera and end-to-end API/media/browser validation must pass before completion is claimed.
