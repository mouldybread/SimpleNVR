#!/usr/bin/env bash
set -euo pipefail
image=${1:?usage: bash tests/runtime-export-acceptance.sh IMAGE EVIDENCE_DIR}
evidence=${2:?evidence directory required}
work=$(mktemp -d "${RUNNER_TEMP:-/tmp}/simplenvr-runtime-export.XXXXXX")
state=$work/state
bin=$work/bin
name=simplenvr-runtime-export-$$
mkdir -p "$state/id_export/temp/retained-evidence" "$state/id_export/buffer" "$bin" "$evidence"
chmod 755 "$work" "$bin"
cleanup() {
  status=$?
  cleanup_failed=0
  # Keep safe failure facts visible before disposable fixture teardown.
  timeout --kill-after=2 5 python3 tests/runtime_export_diagnostics.py \
    "$evidence/runtime-export-report.json" | tee "$evidence/runtime-export-console-summary.json" || cleanup_failed=1
  # No raw auth/proc/network captures. Keep a bounded completion-marker count
  # even when the campaign fails before it can finish its JSON report.
  docker logs --tail 400 "$name" 2>&1 | python3 -c '
import json,sys
raw=sys.stdin.buffer.read(262145)
print(json.dumps({"log_bytes_capped":len(raw)>262144,"completion_markers":raw.count(b"Shutdown complete; recording intent persisted"),"shutdown_failure_marker":b"Shutdown/server failure:" in raw}))
' > "$evidence/runtime-export-log-summary.json" || true
  for owned in "$name" "$name-generator" "$name-auth"; do
    if existing=$(docker ps -aq --filter "name=^/${owned}$"); then
      if test -n "$existing"; then timeout --kill-after=2 15 docker rm -f "$owned" >/dev/null 2>&1 || cleanup_failed=1; fi
    else
      cleanup_failed=1
    fi
  done
  timeout --kill-after=2 15 sudo rm -rf -- "$work" || cleanup_failed=1
  python3 - "$status" "$cleanup_failed" <<'PY' > "$evidence/runtime-export-fixture-cleanup.json" || cleanup_failed=1
import json,sys
print(json.dumps({'campaign_exit':int(sys.argv[1]),'fixture_cleanup_failed':bool(int(sys.argv[2]))}))
PY
  if test "$cleanup_failed" -ne 0; then exit 1; fi
  exit "$status"
}
trap cleanup EXIT
timeout --kill-after=2 20 python3 -m unittest discover -s tests -p 'test_runtime_export*.py' -v \
  2>&1 | tee "$evidence/runtime-export-tooling-tests.log"
# Deliberately complex, bounded, video-only input. Generate with the exact
# runtime image, before applying the pacing shim to application exports.
timeout --kill-after=5 60 docker run --rm --name "$name-generator" --network none --entrypoint /usr/bin/ffmpeg \
  --mount "type=bind,src=$state,dst=/recordings" "$image" \
  -nostdin -v error -f lavfi -i testsrc2=size=512x288:rate=10 \
  -filter_threads 1 -vf noise=alls=100:allf=t+u -t 12 \
  -c:v libx264 -threads 1 -preset ultrafast -crf 0 -g 10 -pix_fmt yuv420p -an \
  -movflags +faststart -y /recordings/id_export/20261110_130000_fixture.mp4
printf 'synthetic retained input; not a complete recording\n' > "$state/id_export/temp/retained-evidence/event.mp4"
printf 'synthetic invalid checkpoint; no recovery authorization\n' > "$state/id_export/temp/retained-evidence/recovery.json"
printf 'synthetic unreported prebuffer\n' > "$state/id_export/buffer/retained-unreported.mp4"
python3 - "$state/cameras.json" <<'PY'
import json,sys
camera={'id':'id_export','name':'Export fixture','rtsp_url':'rtsp://unused-fixture/stream','record_dir':'/recordings/id_export','is_recording':False,'retention_days':365,'recording_mode':'no_recording'}
with open(sys.argv[1],'w') as f:
    json.dump({'cameras':{'id_export':camera},'camera_order':['id_export'],'events':[]},f)
PY
# Preserve application arguments. This fixture alone makes stderr non-pipe
# so all writable child pipes can be bound to the owned stdout inode.
cat > "$bin/ffmpeg" <<'SH'
#!/bin/sh
exec /usr/bin/ffmpeg -re "$@" 2>/dev/null
SH
chmod 755 "$bin/ffmpeg"
SIMPLENVR_E2E_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_urlsafe(32))')
export SIMPLENVR_E2E_PASSWORD
printf '%s\n' "$SIMPLENVR_E2E_PASSWORD" | timeout --kill-after=5 20 docker run --rm --name "$name-auth" -i \
  --mount "type=bind,src=$state,dst=/recordings" "$image" auth set-password ci-export
sudo chown -R 0:0 "$state"
sudo chmod 700 "$state"
docker run -d --name "$name" --label simplenvr.safety.fixture=runtime-export \
  --read-only --cap-drop ALL --cap-add NET_BIND_SERVICE \
  --security-opt no-new-privileges --tmpfs /certs:rw,nosuid,nodev,noexec \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m -p 127.0.0.1::443 \
  --env PATH=/fixture-bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
  --mount "type=bind,src=$state,dst=/recordings" \
  --mount "type=bind,src=$bin,dst=/fixture-bin,readonly" "$image" >/dev/null
image_id=$(docker image inspect -f '{{.Id}}' "$image")
timeout --kill-after=10 240 python3 tests/runtime_export_acceptance.py \
  --container "$name" --state "$state" --evidence "$evidence" \
  --commit "${GITHUB_SHA:-$(git rev-parse HEAD)}" --image-id "$image_id"
test "$(docker inspect -f '{{.State.Running}}' "$name")" = false
test "$(docker inspect -f '{{.State.ExitCode}}' "$name")" -eq 0
