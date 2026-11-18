# Bounded manual orphan-cleanup inspection

Preparation base: c59ee5123d7111054bc63f7267da3b6776c739a9 on fix/recording-safety-audit.
The base is green by user report; its corrected handlers.go brace was independently
read back. Preserve failed intermediate 79bb674 as historical evidence.
Main remains c5be6bf0cfea03071c7df1c3b0f5064753c881dd; keep the audit branch unmerged.

## Production issue and scope

The manual orphan-cleanup path still used unbounded os.ReadDir on the recordings
root, candidate directories and repeated recovery-evidence inspections. Even though
its unlink scope was conservative, a large directory could allocate a large list
and repeatedly rescan it while recordingControlMu was held. This was explicitly
left open in ORPHAN_CLEANUP_BOUNDARIES.md, not an already-remediated retention bug.

The candidate replaces those production reads with chunked bounded inspection and
adds explicit limit outcomes. It does not invoke cleanup, change eligibility to
delete a file, recurse through staging or enable staging expiry/recovery.

## Exact production limits

| Limit | Value | Meaning |
| --- | --- | --- |
| Read chunk | 64 entries | Maximum requested from a directory reader per call; the final request may be smaller. |
| Complete directory list | 8,192 entries | At most this many entries may be accepted as a complete list. An extra observed entry refuses the whole list. |
| Whole-pass entry observations | 262,144 | Counts all observed names, including repeated evidence scans and single-entry staging peeks. |
| Whole-pass directory opens | 4,096 | Counts attempted directory opens, including failed opens and empty-directory peeks. |

One additional entry may be observed to detect that an entry ceiling was exceeded;
that partial observation is never delivered as an authoritative prefix. These are
inspection/refusal limits for one explicit cleanup request, not recording quotas,
retention periods, capture timeouts or CI timeout changes. They are compile-time
production values; tests lower budgets per instance without global/environment hooks.

Repeated evidence checks consume the SAME pass budget, rather than resetting it per
candidate or unlink. This deliberately bounds the existing repeated-scan algorithm;
it does not turn it into a linear algorithm. A large directory can therefore hit
the pass ceiling after only part of its eligible media has been removed. No automatic
retry or budget escalation occurs. Large roots/directories can be refused outright.

## Conservative results

- A reader returns a complete, name-sorted list or an error with no prefix. Read,
  cancellation, limit and close errors do not authorize partial-list deletion or
  an empty-directory claim. No-progress readers also refuse rather than looping.
- Inspection opens require a directory and reject a leaf symlink. Existing Lstat
  checks preserve unexpected staging types; temp/buffer nonempty checks read only
  one entry, not their entire contents.
- Request cancellation is checked before and after each read chunk. Shutdown remains
  checked by the existing wrapper between bounded operations. Neither interrupts
  an already blocked kernel syscall.
- Initial root-limit failure returns HTTP 413 with a fixed no-files-removed message.
  Other initial root inspection failures retain the fixed HTTP 500 response.
- A limit after root enumeration ends the pass with limit_reached=true. Existing
  HTTP 200 partial-result JSON remains, with success=false and successful-unlink
  Count/Bytes preserved. A failed or unattempted unlink is never counted.
- Cancellation/deadline interruption is reported as interrupted=true, not silently
  converted to a successful empty scan. Limit exhaustion alone does not set that flag.
- Staging or inspection uncertainty preserves the affected candidate. Limit exhaustion
  prevents further candidates, unlinks or empty-directory removal in that pass.
- Registered/private directories, root .tmp evidence, opaque temp/buffer contents,
  stable symlink/nonregular media and newly arriving recovery evidence keep their
  existing protections. Camera-registration serialization from the preceding batch
  is unchanged. Nil per-call operation seams now refuse safely rather than panic.

## Regression candidate: seven functions, 39 cases

- Fourteen collector cases: empty/below/exact/over limits, cumulative entry ceiling,
  single-entry peeks, failed/oversized/non-progress reads, a compound EOF/read error, cancellation before/after
  reading and a deadline error. Requested chunk sizes and prefix refusal are asserted.
- Five real disposable filesystem pass-limit cases: root, candidate, cumulative
  entries, before-first-unlink and after-one-successful-unlink refusal.
- Five evidence cases: a .tmp entry beyond a rejected list, nonempty temp/buffer
  directories larger than a peek, and budget refusal during empty-staging peeks.
- Six directory-open cases: regular file, symlink, FIFO, canceled, missing and normal.
- Four HTTP cases: initial 413, partial-limit JSON, cancellation and deadline JSON.
- Four invalid-operation cases, checked both at the core and handler boundaries.
- One oversized injected-list case proving no prefix authorizes removals.

The real-pass fixtures retain remaining bytes, modes and names, verify first-unlink
accounting, and check cancellation is distinct from limits. All fixtures are test-
owned temporary directories. Injected read failures are control-flow evidence, not
actual mount-loss or disk failure qualification. Existing orphan-cleanup and camera-
registration regressions remain required; no workflow, gate, timeout or dependency
change is proposed. Exact-candidate Actions is the execution gate.

## Remaining limits and authority

This bounds directory-list memory/observation work, not elapsed wall time. Filesystem
calls, global control-lock acquisition and response writes can still block. Other
request-local work and the registered-camera snapshot are not a universal process
memory quota. The per-request budget is owner-serialized, not a concurrent shared
scanner object.

Deletion remains pathname-based with a last-observation/unlink race. Leaf no-follow
inspection does not pin the entire ancestor chain or make compare-and-unlink atomic.
Trusted filesystem ownership is still required; no hostile-path, mount-loss or power-
loss guarantee is added. The existing explicit published-MP4 scope is not broadened.

This supersedes only the old unbounded orphan-enumeration/repeated-inspection claim.
Other destructive-path, security/frontend, final evidence/15-skip reconciliation,
live Dahua/RTSP and intended-storage acceptance gates remain open. Nothing here
authorizes production cleanup execution, evidence deletion, automatic recovery,
comparator activation, merge or deployment.
