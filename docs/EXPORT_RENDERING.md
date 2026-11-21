# Prepared export rendering backend: implementation status

This is a staged backend implementation, not a completed integration. export_media.go adds copy/accurate rendering primitives and export_media_test.go adds media integration tests. The HTTP planner still calls the legacy renderer and the browser does not yet send a mode choice. These paths are NOT yet selectable through the app. Do not release the branch as complete.

## Copy

A single complete source is copied byte for byte into bounded prepared-output storage. It is not yet the planned zero-copy original-file HTTP delivery. Several complete sources are remuxed with explicit audio/video mapping, stream copy, and matching codec configuration/extradata checks. No nominal/average frame-rate equality test is used. Packet conversion is disabled for compatible concatenation, preserving compressed payloads. Additional tracks or incompatible configurations are refused, never silently selected/dropped or transcoded.

Subsections in copy mode currently return a boundary error. Keyframe probing, actual-range preview and explicit consent are still required before adjusted-boundary copy is enabled. No intervals are widened automatically.

## Accurate

Audio/video windows use the same video timestamp origin and cut reference, preserving relative audio offset rather than independently resetting tracks. AAC is retained by re-encoding in accurate mode; its sample rate/channel count are validated. Leading/trailing missing audio in an accepted input window is represented as silence to maintain the window duration, without stretching audio. Mixed audio presence or mismatched audio configurations across parts are currently refused.

8-bit yuv420p and full-range yuvj420p are supported within existing dimension bounds. Accurate full-range input is explicitly converted to limited-range yuv420p output. Video is re-encoded with libx264 CRF 18; accurate exports are not byte-identical or lossless. Copy does not perform this conversion. Rotation and higher-bit-depth profiles remain unsupported. Frame-rate strings are not used as a proxy for valid timestamps. Video is frame-granular, not exact camera wall-clock timing.

Fragmented MP4 accurate output uses delayed moov/edit-list handling with negative timestamps enabled for AAC encoder priming; this avoids treating encoder delay as source timing. Output must be a fresh empty regular file. Sources are pinned and revalidated, output writes are bounded, output tracks/durations are probed, and all subprocesses are joined before return. Checks do not by themselves certify every frame, colour value or audio sample.

## Evidence and remaining work

Local FFmpeg command experiments (not execution of the Go implementation) produced decodable H.264/AAC single/multi-part accurate output, preserved a deliberately delayed audio onset at approximately 250 ms, and preserved compressed video/AAC packet hashes when copying complete identical-profile clips. Go integration tests cover the camera metadata profile, original-byte copying, copied-packet hashes, multi-part accurate output, external A/V decode, delayed audio onset, source digests, cancellation and output limits. Go is unavailable in the assistant environment, so these Go tests have NOT been executed or compilation-verified.

Still required: HTTP job renderer wiring; API and UI explicit mode selection; keyframe-adjusted copy planning/consent; original descriptor delivery; real-camera and fractional/B-frame boundary timing validation; colour-range validation; large-file and cancellation-under-load tests; migration and execution of the real Chrome acceptance suite.
