#!/usr/bin/env bash
set -euo pipefail
evidence=${1:?evidence directory required}
mkdir -p "$evidence"
evidence=$(cd "$evidence" && pwd)
head=$(git rev-parse HEAD)
expected_head=${GITHUB_SHA:?GitHub head required}
package=$(go list .)
media="$evidence/bframe-media"
mkdir -p "$media/bin"
helper=$(pwd)/tests/capture-copy-media.py
for tool in ffmpeg ffprobe; do
  printf '#!/bin/sh\nexec python3 "$SIMPLENVR_CAPTURE_HELPER" %s "$@"\n' "$tool" > "$media/bin/$tool"
  chmod 700 "$media/bin/$tool"
done
set +e
SIMPLENVR_CAPTURE_HELPER="$helper" SIMPLENVR_CAPTURE_DIR="$media" SIMPLENVR_CAPTURE_PATH="$PATH" PATH="$media/bin:$PATH" \
  go test -json -race -count=1 -timeout=120s -run '^(TestCopyAcrossTwoBFrameRecordingsWithAudio|TestPreparedKeyframeCopy.*|TestCopyVideoTimingValidator|TestKeyframeCopyVideoPacketTiming)$' ./... 2>&1 | tee "$evidence/bframe-focused.jsonl"
focused_statuses=("${PIPESTATUS[@]}")
set -e
printf 'test_exit=%s tee_exit=%s\n' "${focused_statuses[0]}" "${focused_statuses[1]}" > "$evidence/bframe-focused-status.txt"
# Probe retained synthetic inputs and outputs outside the test process.
probe_exit=0
for file in "$media"/media-*.mp4; do
  [ -f "$file" ] || continue
  ffprobe -v trace -show_streams -show_packets -show_data_hash sha256 -of json "$file" > "$file.probe.json" 2> "$file.trace.log" || probe_exit=1
done
{ go version; ffmpeg -version; } > "$evidence/media-environment.txt" 2>&1
set +e
go test -json -race -count=1 -timeout=240s ./... 2>&1 | tee "$evidence/go-test.jsonl"
statuses=("${PIPESTATUS[@]}")
set -e
test_exit=${statuses[0]}
tee_exit=${statuses[1]}
python3 - "$evidence/run.json" "$head" "$package" "$test_exit" "$tee_exit" <<'PY'
import json,sys
path,head,package,test_exit,tee_exit=sys.argv[1:]
with open(path,'w') as f:
    json.dump({'schema':1,'head':head,'root_package':package,
               'command':['go','test','-json','-race','-count=1','-timeout=240s','./...'],
               'test_exit':int(test_exit),'tee_exit':int(tee_exit),
               'runtime_qualified':False,'cleanup_authorized':False},f,indent=2)
    f.write('\n')
PY
python3 tests/core_ci_evidence.py "$evidence" --expected-head "$expected_head"
test "$test_exit" -eq 0
test "$tee_exit" -eq 0
test "${focused_statuses[0]}" -eq 0
test "${focused_statuses[1]}" -eq 0
test "$probe_exit" -eq 0
