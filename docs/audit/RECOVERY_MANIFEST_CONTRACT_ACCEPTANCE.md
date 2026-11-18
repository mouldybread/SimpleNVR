# Recovery producer / strict-reader contract acceptance

Prepared against a9adc67f7e09a90ef1e42474b3b25c6403ff86ae, included in the same
review batch as the runtime-export observer/diagnostic correction. All five
previously described files are included: three Go suites, this contract, and
AUDIT_ACCEPTANCE.md. The ledger records 6d64ed8 as user-reported green and
a9adc67 as failed, not still awaiting an unknown result. No checkbox is cleared.
The observer helpers and integration were subsequently pushed as 46386d5 and
623bbe1. This record accompanies the final recovery-contract/evidence write;
Go and corrected runtime-container qualification remain pending Actions.

## Existing coverage is not recreated or weakened

Source review covered the recovery writer, strict snapshot reader/parser, owned
checkpoint writer, inventory/admission/capture/publication integration, execution
provenance and their existing regressions. Existing basic state round trips,
root/nested ambiguity tests, size/token-budget checks, owned write/sync/close/
rename faults, retention-coverage preservation and shutdown joining remain.
The retention fix is not reopened and no environment/date workaround is proposed.
No production function, JSON schema, compatibility profile or budget is modified.

## event_recovery_lifecycle_contract_test.go

Twenty actual admitted-job paths combine zero/two prebuffer inputs with success,
partial assembly, capture failure, validation failure, publication collision,
and checkpoint failure at capturing/captured/assembling/publishing/published.
Two more rows combine first/second pin failure with strict inspection of the
full selection intent and actual successfully pinned prefix.

Unlike manually setting manifest states, the lifecycle matrix calls TriggerEvent
and handleEvent and delegates to the ORIGINAL descriptor-owned recovery writer,
preserving its execution checkpoint receipt. Each successful processing snapshot
must strictly parse and agree with read-only inspection. Inspection must not change
fixture entries, modes or bytes. Failure, publishing and published states retain
observed_processing_failure, publication_outcome_ambiguous and
published_checkpoint_unverified distinctions; inspection completion is not media
completeness. Stored coverage remains unknown with original uncertainty reasons.

Injected checkpoint failures occur BEFORE writer delegation. Require sticky failure,
unchanged last persisted bytes and no terminal-release retry. Real storage/sync
faults remain the existing owned-writer/storage suites' separate evidence. Require
retained sources/staging/destination collision bytes, closed inventory, cleared
borrow ownership and returned admission capacity. Terminal retention may advance
revision only after successful persistence, without replacing processing state or
last coverage. Published output with failed published-checkpoint persistence must
retain inspectable publishing intent, not reconstruction authority.

The process fixtures write synthetic text and synthetic probe output (or fail),
not valid media. Capture uses the real inherited /proc/self/fd/3 creator route;
assembly is an actual child and filesystem publisher with controlled probe results.
This validates lifecycle/checkpoint semantics, not real FFmpeg/media correctness,
footage completeness, ingestion timestamps, calibrated timing or durability.

## event_recovery_snapshot_contract_test.go

Seven real snapshot cases: stable control, identical bytes/new inode, in-place
growth, FIFO replacement, symlink replacement, original-name loss and cancellation.
A test-only Context.Err callback schedules mutation at the existing post-O_PATH,
pre-procfs-reopen check, requiring an observed retained identity first. It does not
replace open/stat/read or a production operation. Changed/canceled reads must not
return accepted bytes or a legacy/missing-manifest success. Saved original bytes
remain intact. Return must not leak a descriptor for the observed manifest inode;
this is not an exhaustive accounting of all application descriptors/watchers.

A joined writer performs twelve real legacy-writer atomic replacements while
readRecoverySnapshot makes 24 attempts. Every success must equal one complete
producer payload and strictly parse; a stable final read must observe revision 14.
Scheduling need not yield a rejected snapshot on every runner. This is successful-
snapshot integrity under concurrent replacement, not a claimed deterministic race
schedule or an expansion of owned-writer durability evidence.

## event_recovery_schema_contract_test.go

A synthetic nested template includes two inputs, declared gaps, notification/run
bookends and explicit unknown coverage. It is serialization-only, not eligibility.
108 cases mutate each of 27 required fields as missing/null/case-alias/wrong-type;
12 cases inject unknown authority, duplicate or escaped duplicate keys at four
nested object levels; 18 cases contradict diagnostic counters/binding/bookends/run
state. Total: 138 negative cases, plus a valid baseline.

Ten fuzz seeds include the nested baseline; legacy publishing/published/partial-pin
failure shapes; empty/invalid UTF-8; trailing JSON; size overflow; valid maximum-size
padding; and duplicate keys. The callback requires bounded refusal, immutable input,
no identity on error, camera binding, no positive diagnostic authority and stable
canonical marshal/parse/marshal. Ordinary required Go test discovery executes the
seed corpus; it is NOT an extended fuzz campaign. Optional separately reviewed run:

go test -run '^$' -fuzz '^FuzzRecoveryContractStrictManifest$' -fuzztime=20s -parallel=2 -timeout=45s .

## Evidence and limitations

The earlier native child-script rehearsal observed expected synthetic capture and
assembly sentinel bytes/exit codes, including fd 3; it is helper control-flow
rehearsal, not Go or media acceptance. The new Go suites' formatting/type checking,
compilation, race behavior and exact-head execution remain pending the Actions
qualification recorded in the preparation manifest. No local Go result is invented.

AUDIT_ACCEPTANCE.md's pre-edit Git blob was verified and all historical text and
original checkboxes are preserved. The complete combined review package contains
full source, content hashes and patch, avoiding ephemeral draft fragments.
All 48 runtime Python tooling regressions passed together in preparation; that
result is not a pass for the Go suites or a corrected runtime-container campaign.

This advances one part of the original writer/reader review, not whole-branch
clearance. Cross-artifact observations are not an atomic filesystem snapshot.
Pathname/in-place/hard-link mutation, unbounded enumeration, blocked kernel syscalls,
mount loss, fsync/power-loss durability, live RTSP/acquisition and intended-host
compatibility/cost remain separate gates. No successful probe, diagnostic, partial
or CI result authorizes deletion/recovery. Main remains untouched; automatic
recovery, staging expiry, cleanup activation, evidence deletion, comparator
activation, merge and deployment remain unauthorized.
