# Published deletion engine, subtask 1

Baseline: 914750b4460c7007a974fea312eb272a5d7187f7. New helper/tests only;
no HTTP, retention, publication or staging caller is converted in this commit.

The caller must hold the shared media lease, provide a pure selection predicate
and a registry/policy/shutdown guard. The helper never reacquires/releases it.
Only directly contained regular .mp4 files can enter the plan. Stable symlinks,
special files and directories are skipped, not followed; staging/.tmp is opaque.
Inspection counts all top-level names and refuses over 8192 before unlinking.
Root/parent descriptors are pinned and named identities rechecked. Selected
entry identities are preflighted together and checked again before each unlinkat.
The directory is never removed and late arrivals are never added to the plan.

The pass stops at the first error. Count/Bytes include successful unlinks only;
Failures=1 denotes an errored pass, not a count of every undeleted file. Directory
sync is attempted after any successful unlink, including partial failure. No
transaction, rollback, hostile-writer safety, atomic compare/unlink, ancestor or
mount identity, or power-loss proof is claimed. Lease/guard checks do not make
external races atomic. Production still uses its existing deletion paths.

Six test functions (including partial-outcome and parent/limit subcases) are authored,
not executed: Go/gofmt/Docker unavailable. Preparation checks are not build/race/
vet evidence. Next subtask: wire explicit deletion and its admission tests; then
wire retention with unchanged expiry selection and no nested gate acquisition.
Publication/checkpoint/staging-release coordination remains open. No merge.
