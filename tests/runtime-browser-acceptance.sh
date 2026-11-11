#!/usr/bin/env bash
set -euo pipefail
image=${1:?usage: bash tests/runtime-browser-acceptance.sh IMAGE EVIDENCE_DIR}
evidence=${2:?evidence directory required}
work=$(mktemp -d "${RUNNER_TEMP:-/tmp}/simplenvr-runtime-browser.XXXXXX")
state=$work/state
bin=$work/bin
name=simplenvr-runtime-browser-$$
mkdir -p "$state/id_browser" "$bin" "$evidence"
chmod 755 "$work" "$bin"
cleanup() {
  docker logs "$name" > "$evidence/runtime-container.log" 2>&1 || true
  docker rm -f "$name" >/dev/null 2>&1 || true
  sudo rm -rf -- "$work"
}
trap cleanup EXIT
# Generate fixtures using the actual runtime image codecs, not host FFmpeg.
for stamp in 120000 120008; do
  docker run --rm --network none --entrypoint /usr/bin/ffmpeg \
    --mount "type=bind,src=$state,dst=/recordings" "$image" \
    -nostdin -v error -f lavfi -i testsrc2=size=256x144:rate=10 \
    -t 8 -c:v libx264 -threads 1 -g 10 -pix_fmt yuv420p -an -movflags +faststart \
    -y "/recordings/id_browser/20261110_${stamp}_fixture.mp4"
done
docker run --rm --network none --entrypoint /usr/bin/ffmpeg \
  --mount "type=bind,src=$state,dst=/recordings" "$image" \
  -nostdin -v error -f lavfi -i testsrc2=size=256x144:rate=10 \
  -f lavfi -i sine=frequency=440:sample_rate=48000 -t 4 \
  -c:v libx264 -threads 1 -g 10 -pix_fmt yuv420p -c:a aac -movflags +faststart \
  -y /recordings/id_browser/20261110_123000_audio.mp4
python3 - "$state/cameras.json" <<'PY'
import json,sys
cam={'id':'id_browser','name':'Browser fixture','rtsp_url':'rtsp://unused-fixture/stream','record_dir':'/recordings/id_browser','is_recording':False,'retention_days':365,'recording_mode':'no_recording'}
with open(sys.argv[1],'w') as f:
    json.dump({'cameras':{'id_browser':cam},'camera_order':['id_browser'],'events':[]},f)
PY
# Pace real FFmpeg input so cancellation can observe an active runtime render.
# Preserve all application arguments and execute the real binary, not a fake MP4.
cat > "$bin/ffmpeg" <<'SH'
#!/bin/sh
exec /usr/bin/ffmpeg -re "$@"
SH
chmod 755 "$bin/ffmpeg"
SIMPLENVR_E2E_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_urlsafe(32))')
export SIMPLENVR_E2E_PASSWORD
printf '%s\n' "$SIMPLENVR_E2E_PASSWORD" | docker run --rm -i \
  --mount "type=bind,src=$state,dst=/recordings" "$image" auth set-password ci-browser
sudo chown -R 0:0 "$state"
sudo chmod 700 "$state"
docker run -d --name "$name" --read-only --cap-drop ALL --cap-add NET_BIND_SERVICE \
  --security-opt no-new-privileges --tmpfs /certs:rw,nosuid,nodev,noexec \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m \
  -p 127.0.0.1::443 \
  --env PATH=/fixture-bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
  --mount "type=bind,src=$state,dst=/recordings" \
  --mount "type=bind,src=$bin,dst=/fixture-bin,readonly" "$image" >/dev/null
ready=0
for attempt in $(seq 1 60); do
  if docker exec "$name" wget --no-check-certificate -q -O /dev/null https://127.0.0.1/auth/login; then ready=1; break; fi
  sleep 0.5
done
test "$ready" -eq 1
port=$(docker inspect -f '{{(index (index .NetworkSettings.Ports "443/tcp") 0).HostPort}}' "$name")
image_id=$(docker image inspect -f '{{.Id}}' "$image")
timeout 300 python3 tests/runtime-browser-acceptance.py \
  --origin "https://127.0.0.1:$port" --container "$name" \
  --commit "${GITHUB_SHA:-$(git rev-parse HEAD)}" --image-id "$image_id" --evidence "$evidence"
docker stop --timeout 75 "$name" >/dev/null
test "$(docker inspect -f '{{.State.ExitCode}}' "$name")" -eq 0
