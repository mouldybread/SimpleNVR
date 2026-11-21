# Prepared export verification status

The prepared-export Go workflow triggers on pushes to fix/large-media-exports, pull requests and manual dispatch. It installs the Go version from go.mod, Node 22 and FFmpeg; verifies dependency files; builds the exact checkout; runs the existing complete race-enabled core evidence command (not a filtered export-only subset); and runs go vet. It retains the Go execution evidence with the commit SHA and run attempt. A missing dependency/build/test is not treated as a pass.

New native Go tests exercise non-reordered H264/AAC keyframe planning, actual copy rendering, external decode, duration and source immutability, plus reordered-video refusal. The required-media environment makes missing FFmpeg/Node a failure rather than an optional local skip.

Six standard-library protocol regressions passed locally: no POST retry after an uncertain create, queued/preparing/ready transitions, terminal failure refusal, status/control-size rejection, release acknowledgement plus HTTP 404, and job/download URL binding. These tests do NOT prove the live backend or browser.

Go and Docker are unavailable in the assistant execution environment. The Go/native suites have not run there; CI must produce the actual results after this commit is pushed. This workflow is not a release-qualification claim.

The live-browser migration is the next separate commit: mount private export scratch outside recordings, preserve existing native playback/diagnostic checks, use prepared create/status/download/release APIs, exercise explicit copy proposal approval/cancellation, validate accurate AAC output, retain saved-download hashes/duration/external decode and active-render cancellation/reaping. Its migration draft has only passed Python/shell syntax checks here; no live-container browser pass is claimed. The older routed-fixture browser-export-acceptance.py also remains a separate legacy harness and must not be reported as migrated.
