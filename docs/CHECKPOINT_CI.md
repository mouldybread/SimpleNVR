# Checkpoint-only verification

Run the full verification suites at the end of a checkpoint, not after each
intermediate push. Ordinary branch pushes and PR open/reopen/synchronize events
do not request the test suites. Tests and acceptance assertions are unchanged.

## Request all suites from a PR

Apply the `run-checkpoint-ci` label to a same-repository PR. The Checkpoint
verification workflow listens only for `pull_request: labeled` and compares the
newly added label's name. Leaving the label attached does not cause later pushes
to run tests. Other label events may produce a workflow with skipped jobs; those
are not successful checkpoint verification. Fork PR label requests are skipped;
review fork changes before testing a trusted same-repository branch manually.

Create the label once if it does not exist:

```sh
gh label create run-checkpoint-ci --repo mouldybread/SimpleNVR \
  --description 'Explicitly request full checkpoint verification'
```

For the current PR, request verification with:

```sh
gh pr edit 13 --repo mouldybread/SimpleNVR --add-label run-checkpoint-ci
```

For a later checkpoint or another run after fixing a failure, remove the label
and add it again. Removing it does not run the suites:

```sh
gh pr edit 13 --repo mouldybread/SimpleNVR --remove-label run-checkpoint-ci
gh pr edit 13 --repo mouldybread/SimpleNVR --add-label run-checkpoint-ci
```

The same label edit can be performed by an authorized GitHub API client or the
connected assistant, with confirmation for the external write. No bot token or
write permission is granted to the test workflows. Do not rely on a label edit
made by another workflow's default GITHUB_TOKEN to trigger this workflow; use a
human/appropriately authorized API client or dispatch explicitly instead.

## Manual fallback

Once checkpoint-ci.yml exists on the default branch, use Actions > Checkpoint
verification > Run workflow and select the checkpoint branch. The CLI equivalent
for this work branch is:

```sh
gh workflow run checkpoint-ci.yml --repo mouldybread/SimpleNVR \
  --ref fix/recording-scan-window
```

This single request runs Safety verification, Prepared export Go verification,
and Prepared export browser verification. Each suite also retains its individual
workflow_dispatch entry point for targeted diagnosis; an individual suite run
is not full checkpoint verification. GitHub requires a workflow_dispatch workflow
to exist on the default branch before it can be dispatched. The PR label route
uses the PR workflow definition and does not require merging the checkpoint
changes merely to select the work branch. Existing runs are not canceled by
this configuration change.

## Commit and evidence identity

For a label request, the caller captures pull_request.head.sha from the event,
not the moving branch name and not the PR's synthetic merge SHA. For a manual
request it captures github.sha for the selected ref. The caller passes that same
immutable checkpoint_sha to all three reusable suites. Every checkout uses it;
artifact names and evidence assessments use it as well. GITHUB_SHA is set only
for individual evidence-script processes to match their checked-out commit;
GitHub's event metadata is not rewritten.

Record the requested SHA and run URL in the checkpoint notes. Require all three
suites, both browser matrix attempts in each browser suite, and the safety
aggregate check to complete successfully. A queued, skipped, canceled, or failed
job is not a verified checkpoint. If any commit is pushed afterward, that new
head is unverified until another explicit checkpoint request succeeds. Do not
reuse an older green result as evidence for a newer head. Source-head testing
does not certify a later merge result after the base branch changes.

## Branch protection and release publishing

This change does not edit repository rulesets or branch protection. Reusable
workflow calls can change displayed check names. After the first explicit run,
review required checks in repository settings and select the appropriate checks
from that run; keep merge protection enabled. Do not substitute a skipped label
router job for completed suite results. If required checks remain pending before
a checkpoint run, request verification rather than bypassing protection. Until
required-check settings are reviewed, do not assume GitHub enforces this entire
checkpoint policy automatically.

Release Docker Image remains tag-triggered for v* tags and still calls Safety
verification before publishing. This is an intentional release safety exception,
not CI on intermediate branch pushes. No release publishing permissions or
release triggers are changed.

## Configuration review

Confirm the three suite workflows have no push or pull_request triggers, retain
workflow_dispatch and workflow_call, and still contain their original tests.
Confirm checkpoint-ci.yml accepts only labeled PR events or explicit dispatch,
and only the exact run-checkpoint-ci label starts PR suites. Exercise the label
and manual routes at the next checkpoint; this scheduling change alone does not
claim that the existing Go failure is fixed or that any checkpoint passed.
