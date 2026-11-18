# Combined runtime-export observer correction and safe diagnostics

Preparation base: a9adc67f7e09a90ef1e42474b3b25c6403ff86ae.
The observer helpers and integration were pushed as
46386d532b82e64aab81ca8451fb663747d9b46d and
623bbe16564aed009735e1a0bd8aa820490eaa61. This preparation/evidence record
accompanies the final recovery-contract write in the same 13-file batch, described
in RECOVERY_MANIFEST_CONTRACT_ACCEPTANCE.md. No production source, image definition,
media compatibility, process budget, retention or deletion policy is changed.
Main remains untouched and the branch remains unmerged.

## Failure evidence and diagnosis

The user reported 6d64ed8 green and a9adc67 failed. The latter first made the
runtime-export campaign mandatory, so the earlier green does not establish a
successful runtime-export campaign. The exact-head report supplied through Dropbox
was read as extracted JSON, not independently downloaded Actions artifacts or
byte-verified ZIP contents. It records:

- Successful whole-copy slow consumption: 32,882,114 bytes, 4,014 reads, external
  decode success and 12-second duration, followed by a clean running baseline.
- Failure in disconnect-and-both-admissions-reuse before its first pressure
  witness, with assertion: sustained source-matched pipe-write and owned TCP queue
  backpressure not reached.
- Both SIGTERM cases not-started and no persistence observations.
- One source pin, anonymous manifest and matching child/stdout pipe. From elapsed
  16.517 seconds onward, rchar remains 11,072,704 and the TCP queue 1,009,328 bytes.
  The leader wchan is futex_do_wait, while process wchar keeps receiving roughly
  119-120-byte increments. This does not itself prove worker-pipe backpressure.

Native host FFmpeg rehearsal observed the missing distinction: a futex-waiting
leader, a separate pipe_write worker, stable worker wchar and continuing status
writes in the leader/process aggregate. It also found a duplicate writable
stdout descriptor. The old predicate incorrectly required the leader itself to
be the blocked writer and the process-wide counter to stop. These are established
test-observer defects, not established production export defects. Host rehearsal
is not the exact Alpine runtime/HTTPS campaign or deployment qualification.

## Integrated correction

The new test-only runtime_export_writer_observer.py supplies a bounded shell
function, record decoder and predicate. The driver invokes it only for selected
source-matched stream-copy children; parsing still binds all records to that child.
Require real pipe_write/anon_pipe_write tasks, stable process and task identities,
and stable per-writer-task wchar. Leader wait and aggregate counters remain
observable but are not substituted for the writer task. Futex waits alone never
satisfy the condition.

Require descriptor 1 and that every observed writable pipe names the same stdout
pipe inode, which must also be owned by the application. Aliases to that SAME pipe
are allowed; any different writable pipe or incomplete/process-changed scan refuses
the witness. The test-only FFmpeg shim retains all application arguments and input
pacing, but redirects child stderr to /dev/null to remove ambiguity with Docker's
stderr pipe. This discards child stderr diagnostics only in this synthetic campaign;
production logging is unchanged. Application Go logs, structured observations and
external decode checks remain. No ptrace syscall reads or added capabilities.

Inspection refusal limits are 64 descriptors and 32 tasks per selected child,
within the existing 32-KiB observation limit. These are bounded test-inspection
limits, not expanded application thread/process allowances. Proc observations are
still non-atomic; they do not remove general in-place/pathname/kernel-I/O limits.

The existing source pin/manifest/media-child counts, application pipe ownership,
positive unchanged owned TCP queues, at least three samples spanning 0.5 seconds,
12-second readiness/cleanup bounds, two-admission reuse, SIGTERM, private persistence,
source/sentinel hashes and restart requirements all remain. No weaker fallback,
new runtime timeout, larger media fixture, production budget or TLS relaxation.

## Safe failure visibility, composed once

runtime_export_diagnostics.py reads at most 256 KiB plus one refusal byte and
rejects nonregular/symlink report files. It emits only fixed case statuses,
allowlisted internally authored assertion reasons/types, bounded numeric counters
and known wait names, including bounded writer-task counters. It never emits raw
cookies, command stderr, proc paths, addresses, arbitrary error strings or unknown
fields. The assertion allowlist is fixed in the committed helper, not learned from
the report at runtime. Capped history is labelled as last recorded state, not
claimed to be the failure-time state.

The shell invokes this helper within five seconds plus a two-second kill allowance
from its EXIT trap before fixture removal, prints the summary and retains it as
runtime-export-console-summary.json. Original report bytes and campaign exit status
are preserved; diagnostics/fixture cleanup failures still fail the shell. A summary
never authorizes acceptance or cleanup. The broadened test_runtime_export*.py unit
pattern appears once, including all old and new modules without duplicate discovery.
The unchanged required core step and always-upload wiring already retain evidence.

## Preparation checks and remaining qualification

The complete reconstructed driver, modified existing unit module and the two new
helper/test pairs ran together: all 48 methods passed (22 original, 11 writer,
15 diagnostics). This includes real TLS positive control, real host FFmpeg task
scanning, hostile/malformed report handling and actual execution of the updated
EXIT trap with isolated Docker/sudo substitutes for success, campaign failure and
fixture-removal failure. It is tooling evidence, not real container SIGTERM proof.

All modified pre-existing file contents were Git-blob checked before editing.
The combined patch was checked and round-tripped against complete modified-file
baselines; the review package contains full proposed files, not truncated fragments.
The unchanged driver body after the observer functions remains present. Python
syntax and Bash syntax checks are included in the preparation record.

Exact-head Actions must still compile/race-test the new Go contracts and execute
the corrected hardened runtime campaign. No corrected campaign pass, cost
acceptance, ingestion coverage, production fix, deletion authority or deployment
qualification is claimed. Do not credit the old report with the new task witness.
No merge, deployment, automatic recovery, staging expiry, evidence deletion or
comparator activation is authorized.
