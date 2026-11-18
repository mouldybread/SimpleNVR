#!/usr/bin/env bash
set -euo pipefail
evidence=${1:?evidence directory required}
mkdir -p "$evidence"
head=$(git rev-parse HEAD)
expected_head=${GITHUB_SHA:?GitHub head required}
package=$(go list .)
set +e
go test -json -race -count=1 -timeout=120s ./... 2>&1 | tee "$evidence/go-test.jsonl"
statuses=("${PIPESTATUS[@]}")
set -e
test_exit=${statuses[0]}
tee_exit=${statuses[1]}
python3 - "$evidence/run.json" "$head" "$package" "$test_exit" "$tee_exit" <<'PY'
import json,sys
path,head,package,test_exit,tee_exit=sys.argv[1:]
with open(path,'w') as f:
    json.dump({'schema':1,'head':head,'root_package':package,
               'command':['go','test','-json','-race','-count=1','-timeout=120s','./...'],
               'test_exit':int(test_exit),'tee_exit':int(tee_exit),
               'runtime_qualified':False,'cleanup_authorized':False},f,indent=2)
    f.write('\n')
PY
python3 tests/core_ci_evidence.py "$evidence" --expected-head "$expected_head"
test "$test_exit" -eq 0
test "$tee_exit" -eq 0
