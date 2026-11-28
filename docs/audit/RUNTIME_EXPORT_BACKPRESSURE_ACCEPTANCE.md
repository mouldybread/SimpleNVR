# Runtime-container export backpressure and shutdown acceptance

Preparation base: a7f9f6618c992fd201421fc492032fc19d6eb37e.
That head is green by user report, not independently downloaded Actions results.
This candidate has not run in the runtime image yet. No newly reproduced production
defect is claimed and no production implementation is changed.

## Integrated batch

- tests/acceptance/runtime-export-acceptance.sh owns an isolated synthetic state directory,
  fixture generator, password initializer and hardened runtime container.
- tests/acceptance/runtime_export_acceptance.py exercises authenticated real HTTPS exports,
  bounded ownership observations, disconnect, SIGTERM, persistence and restart.
- tests/acceptance/test_runtime_export_acceptance.py contains 22 tooling regression methods.
- .github/workflows/safety-ci.yml runs the campaign as a mandatory core step after
  the existing recording/restart and SIGTERM smoke checks, then retains evidence
  with always(). The existing aggregate requires core. Both isolated Chrome
  attempts, all existing Go/storage checks and correspondence resources remain.
- This document records the contract, preparation evidence and limitations.

The campaign uses a separate disposable container, not the browser container.
There is therefore no change to browser ordering, authentication, the 300-second
expanded driver, 340-second readiness wrapper, eight-minute execution step or
25-minute browser job. No existing required check or timeout is relaxed. Core
also retains its existing 25-minute job budget. Its stock tooling installation
adds OpenSSL for the real TLS tooling regression; the application image and its
production dependencies do not change.

## Fixture and bounds

Generate a 12-second 512x288, 10-fps, video-only yuv420p H264 file with temporal
noise, lossless ultrafast encoding and one-second GOPs, using /usr/bin/ffmpeg in
the exact built runtime image. Require source size between 8 and 40 MiB; refuse
an out-of-range fixture rather than assuming it can exert pressure. Cap received
whole-copy output at 48 MiB. A test-only PATH shim execs real /usr/bin/ffmpeg with
-re and every original application argument. No fake child, output, handler or
HTTP response is substituted. The pacing changes the exercise timing; this is
not unmodified-speed or intended-host performance qualification.

The container retains read-only root, dropped capabilities except NET_BIND_SERVICE,
no-new-privileges, 64-MiB /tmp tmpfs, separate /certs tmpfs, root-private fixture
state, read-only shim mount and an ephemeral loopback-only HTTPS publication.
No production trim/process/media budget changes. The shell allows 20 seconds
for tooling tests, 60 for media generation, 20 for password initialization and
240 for the campaign, with an explicit six-minute workflow step. Each stop uses
the existing 75-second container stop allowance. Docker exec observations and
external probes are individually bounded. Existing checks retain their budgets.

## Required sequence

1. Snapshot hashes and sizes of the source and three synthetic retained evidence
   files: temp/retained-evidence/event.mp4, its intentionally invalid recovery.json,
   and buffer/retained-unreported.mp4. These are preservation sentinels, not valid
   recovery manifests or evidence of successful capture. Verify camera intent is
   no_recording, retention 365, camera order and event data. Log in through real
   session authentication; keep the cookie/password only in memory/environment.
2. Receive a whole-copy export with normal receive buffering, 8192-byte reads and
   a two-millisecond application delay per read. Require pinned-descriptor policy,
   whole-copy headers, bounded body, multiple reads, external complete FFmpeg
   decode and approximately 12-second duration. This successful slow-reader
   control is not counted as the non-reading backpressure exercise.
3. Open an authenticated whole-copy export with a small real client TCP receive
   buffer, accept video headers, then do not consume the response body. Require
   the sustained combined witness below. Disconnect, then require no observed
   source pins, anonymous export/trim descriptors, named export/trim temporaries
   or direct runtime FFmpeg/FFprobe children. Hash-check retained files again.
4. Start two further whole-copy requests concurrently in owner lifetime. Require
   both video responses, two source pins, two anonymous manifests, two matching
   children and two backpressured owned sockets at the same time. A successful
   single retry does not satisfy this requirement. Disconnect both and observe
   cleanup again. This demonstrates reuse of both configured admissions through
   real work; it is not direct observation of the internal channel length.
5. Start a new whole-copy request and reach the same sustained witness. Keep the
   client unread/open while docker stop sends SIGTERM. Require clean zero exit,
   no OOM, one new shutdown/persistence completion marker, newly written private
   configuration, unchanged no_recording intent/events and identical retained
   file bytes. Restart the same fixture, resolve its current published endpoint,
   wait for real HTTPS readiness and establish a new session. Verify preserved
   files and an export-resource-clean running baseline.
6. Request a boundary trim (0.2 to 11.8 seconds) with a joined non-daemon client
   header reader. Before SIGTERM require a source-matched real trim child, pinned
   source, anonymous trim output with bytes, increasing child CPU ticks and no
   completed HTTP response. Keep that request alive while signaling. Require
   clean exit and fresh persistence/preservation observations. Join the client;
   reject any successful video/download response headers. A transport exception
   or non-video empty/default HTTP response may occur: no blanket status-code
   assumption is made. Restart/re-resolve/reauthenticate, require both admissions
   usable concurrently again, disconnect/join observations, then stop the idle
   fixture cleanly as the final campaign phase.

## Backpressure witness and failure semantics

A queue alone, a child alone, or a paced/sleeping child is insufficient. At least
three observations spanning 0.5 seconds must agree on:

- source-matched direct FFmpeg children, with stable PID/start-time identities;
- each child's stdout pipe also owned by the application;
- an explicit pipe_write or anon_pipe_write kernel wait state;
- unchanged child wchar counters, so the observed output pipeline is stalled;
- positive, unchanged established HTTPS TCP send queues whose socket inodes are
  present in application descriptors;
- the expected count of pinned inputs, anonymous manifests, children and sockets.

Only counters/ownership fields are emitted. The kernel network tables are filtered
before output; addresses and complete proc cmdline/environment are not captured.
A missing/hidden/unsupported wchan, unavailable procfs or inaccessible ownership
observation fails the exercise. There is no weaker passing fallback. Pressure
must be reached within 12 seconds; cleanup within 12 seconds; render readiness
within eight seconds. The fixture label and image identity are checked before
runtime lifecycle actions, and the endpoint must be a single loopback mapping.

This is a combined end-to-end stalled-output-pipeline witness. Unlike the existing
Go integration test's ResponseWriter observer, it does not directly instrument a
Go writer call or prove which goroutine is blocked. The two kinds of evidence
remain distinct. The source-specific child/pipe and owned socket checks avoid
counting ordinary input pacing or unrelated socket pressure as success.

## Persistence and ownership limits

Disconnect observations run while the application is alive, before the container
is removed. They check real descriptor targets and direct media children rather
than inferring resource release from container exit. After SIGTERM, clean exit,
a new completion marker, changed configuration mtime, mode 0600, preserved data
and usable restarts are the observable runtime contract.

There is NO test-only persistence latch or timestamp inside the shutdown
coordinator here. These runtime observations do not independently prove internal
join-before-persist ordering or that the parent reaped a child before namespace
teardown. That ordering retains the existing Go export/network/phase and shutdown
coordinator test evidence. Restarted tmpfs being empty is not evidence that every
pre-exit descriptor was explicitly closed. No claim is made about hidden internal
watchers, every descriptor, in-place mutation, interruptible kernel file I/O,
fsync durability, power loss or preserved capture completeness.

## Evidence and teardown

runtime-export-report.json is initialized failed; each named case begins
not-started, is written as running before execution and becomes passed only after
its assertions. Keep bounded partial proc observations, selected full witnesses,
source/sentinel hashes, saved positive-control MP4, header/decode results,
shutdown summaries and restart/reuse observations. A running/incomplete report
left by an outer timeout is a failed/incomplete campaign, never acceptance.
Exceptions retain safe types and fixed assertion messages, not arbitrary socket,
command or credential-bearing error strings. Cleanup failures fail the campaign.
A separate fixture-cleanup JSON records shell outcome and fixture-removal failure.
The log summary inspects both Docker output streams but retains only bounded byte
count/completion/failure-marker observations, not raw logs with network details.

Artifacts: runtime-export-acceptance-<full SHA>-<run_attempt>, retained 14 days,
including on failure. The source fixture is synthetic and disposable. The EXIT
trap removes only the three names and private directory created by this harness;
this does not authorize deleting application staging or real recordings.
The GitHub connector does not expose artifact/full-log download tools. Artifact
inspection remains pending unless the exact evidence is actually supplied.

## Preparation evidence and broader reconciliation

Preparation passed all 22 tooling regression methods, including a genuine local
TLS positive control for both receive-buffer modes, malformed/insufficient
witness rejection, stability resets, two-slot counts, resource-clean assertions,
partial-failure reports, response/client close handling, joined header workers,
non-video cancellation semantics and markers on Docker stderr. Bash syntax and
Python syntax were checked. These are tooling results, not a runtime-container
campaign pass.

A host-tooling characterization of the same generator produced 32,880,199 bytes,
12.000000 seconds, H264/yuv420p 512x288, matching 10/1 rates. Runtime-image fixture
sizing is still enforced in Actions; this sample does not qualify runtime codecs
or guarantee kernel wait-state availability. No exact-candidate Actions success
is claimed at preparation.

AUDIT_ACCEPTANCE.md's historical current-status paragraph still names 3f7c225;
the handoff and this focused reconciliation carry a7f9f66 as latest user-reported
green. Its old preparation/pending passages are not current absence claims.
Existing export Go tests, active-container capture/restart, native browser failure
and recovery, correspondence interruption/retry and all-four-report assessment
remain implemented scoped evidence. This batch does not recreate those suites.

Outstanding: exact-candidate Actions/runtime artifact review; actual numerical
same-head correspondence costs and intended-host fit; any remaining original
synthetic/browser/timeline cases after reconciliation; whole-branch source and
writer/reader/destructive-path/lock/resource/auth/publication/persistence review;
live Dahua/RTSP/reconnect and deployment filesystem/mount/procfs/blocked-I/O,
quota and power-loss qualification. Correspondence activation, cost acceptance,
positive ingestion-time coverage, cleanup authorization and deployment
qualification remain false. Main remains untouched; merge, deployment, automatic
recovery, staging expiry and evidence deletion remain unauthorized.
