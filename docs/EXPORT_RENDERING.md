# Export rendering

Copy and Accurate are connected to the HTTP job planner and selectable in the browser. Neither mode silently falls back to the other. [Large-media exports](LARGE_MEDIA_EXPORTS.md) describes storage, limits, job lifetime and the API.

## Copy

A single complete source is copied byte for byte into bounded prepared-output storage. Multiple complete sources are remuxed with explicit video/audio mapping and matching codec configuration/extradata checks. Packet conversion is disabled for compatible concatenation, preserving compressed payloads. Nominal or average frame-rate equality is not used as a substitute for actual configuration compatibility. Extra tracks and incompatible configurations are refused, never silently selected, dropped or transcoded.

### Part of a recording

Subsection copy supports non-reordered video with usable decoded keyframe timestamps. The server plans an earlier video start and later video end at available keyframe or file boundaries and reinspects expanded coverage for gaps/overlaps. It returns a proposal before creating a job or reserving output storage. The browser displays the requested/proposed video intervals and AAC packet allowance and waits for explicit approval. Cancelling that proposal creates no job.

Approval keeps the original requested selection and adds the accepted video boundaries. The server recomputes the plan and requires a new approval when those video boundaries change. Reordered video, missing keyframe information, incompatible tracks or undecodable candidate output are refused; the user must explicitly choose Accurate instead.

AAC packet boundaries can extend outside the video-aligned interval by the disclosed allowance. Candidate copy output is duration-probed and externally decoded before readiness. A prepared copy is not zero-copy HTTP delivery of the original recording.

## Accurate

Audio/video windows share the video timestamp origin and cut reference, preserving supported relative audio offset rather than independently resetting each track. AAC is re-encoded; sample rate and channel count are validated. Missing leading/trailing audio within an accepted input window is represented as silence to keep its duration without stretching audio. Mixed audio presence or mismatched audio configurations across parts are refused.

8-bit `yuv420p` and full-range `yuvj420p` are supported within the renderer's dimension bounds. Full-range input is explicitly converted to limited-range `yuv420p`. Video is encoded with libx264 CRF 18, so accurate output is not byte-identical or lossless. Copy does not perform that conversion. Extra tracks, rotation and higher bit depths remain unsupported.

Video timing is frame-granular, not calibrated camera wall-clock time. Accurate fragmented MP4 uses delayed `moov`/edit-list handling and permits negative timestamps for AAC encoder priming so encoder delay is not treated as source timing.

## Safeguards

Output must be a fresh empty regular file, never an existing recording. Inputs are pinned and revalidated, output writes are bounded, track configuration/durations are probed, and subprocesses are joined before return. These checks do not certify every frame, colour value or audio sample.

## Evidence and remaining qualification

GitHub reported all 11 check runs successful on PR #8 head `b8a6178136f620d88f82099ddc7a3a91cdd5cff7`, including Go build/race/vet, the prepared-job Chrome runs and safety verification. The Go tests include original-byte copy, copied packet hashes, accurate audio offset, external decode, source immutability and resource/cancellation checks. The live-browser harness uses real authenticated HTTPS jobs and browser downloads. See [prepared export verification](PREPARED_EXPORT_VERIFICATION.md).

That synthetic evidence is not full-resolution real-camera, multi-GiB, colour-fidelity or heavy-load qualification. Representative fractional/B-frame timing and real-camera colour behaviour still need deployment-specific validation. Original-file delivery without a prepared copy is not implemented. The older routed-fixture browser script remains separate from the migrated live-runtime harness.
