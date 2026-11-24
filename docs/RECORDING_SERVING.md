# Private recording serving (Plan v2 checkpoint 3)

See [recording layout](RECORDING_LAYOUT.md) for publication, timeline, export and retention rules.
This document covers checkpoint 3 = Serving in issue #11's Plan v2. Earlier integration
notes using slices A-E do not renumber Plan v2 checkpoints.

## Day-folder and filename contract

Authenticated recording requests use
`/recordings/<camera>/<YYYYMMDD>/<start>_E<end>_<label>.mp4`.
The day must be a valid eight-digit calendar date and equal the filename's UTC
start date. The filename must parse as a stamped recording with a positive span
of at most 24 hours. Invalid stamped names and day/name mismatches return 404,
even when a regular file exists at the requested path. Serving does not probe
media duration or recursively search for a different file.

Cross-midnight recordings remain under the start day, not the end day. For example:

```
/recordings/a/20261110/20261110_235958_E20261111_000006_fixture.mp4
```

Published partial recordings use `_partial` before `.mp4` and follow the same
start-day rule, including cross-midnight partials. Temporary `.mp4.tmp` files
are not exposed. Existing direct-file requests retain their separate handling;
this change does not add a legacy migration mechanism.

## Serving safeguards

Authentication intercepts `/recordings/` before the mux and routes it to
`servePrivateRecordingAt`; the older `serveRecordingHandler` is not this
production serving path. The camera must be registered with the expected
recording directory. Only GET and HEAD are allowed; Range handling remains
provided by `http.ServeContent`. Private responses retain
`Cache-Control: private, no-store` and `X-Content-Type-Options: nosniff`.

Path validation rejects arbitrary nesting, traversal and unsafe components.
Root, camera and day directories are opened fd-relative with O_PATH,
O_DIRECTORY and O_NOFOLLOW. The final entry is observed without opening a
FIFO/device for data, must be regular, and is reopened through its pinned
procfs descriptor with device/inode identity revalidation. Symlinks and
special files remain refused; an already opened descriptor stays pinned
across pathname or directory replacement.

## Regression coverage and review status

`private_media_day_test.go` covers ordinary, cross-midnight and published partial
files through shared GET/HEAD/Range assertions, including body, content length,
content range, content type and private headers. Wrong-day cases create real
regular files and require 404 for GET/HEAD with and without Range. Day symlink,
file symlink, directory and FIFO fixtures explicitly pass day/name validation
before testing filesystem rejection. Existing traversal, registered-directory,
FIFO timeout/worker-join and opened-inode assertions remain.

The browser's `private_media_preflight` remains an authenticated Range request
requiring 206, video/mp4 and the expected MP4 prefix. Anonymous private-media
refusal and native playback assertions remain in place; these final serving
slices do not modify browser scripts.

Source review of implementation head `cd30e90bd31f019eed2d1c81a171a4e9d0856ed0`
found no further serving implementation or targeted regression-test work in the
agreed checkpoint 3 scope. This is a source-review assessment, not execution
verification: the assistant has not compiled or run these changes.

## Final acceptance remains pending

The documentation commit containing this file becomes a new, unverified head.
After documentation and PR-status updates, capture the exact final PR head and
explicitly request the full [checkpoint CI](CHECKPOINT_CI.md) run. The user will
trigger that run; this documentation change does not request it.

Checkpoint 3 is not complete until all eight required checks succeed on that
exact final head: safety core, both Chrome acceptance attempts, ordered-content
resource measurements, safety aggregate Required checks, prepared_go, and both
prepared_browser attempts. Queued, skipped, canceled or failed checks do not
satisfy acceptance. Record the final SHA and run URL after inspecting the results.
Any later commit requires another acceptance run. Older green results do not
verify the new head. Keep PR #13 draft and issue #11 open; checkpoint 3 acceptance
does not establish completion of the other Plan v2 checkpoints.
