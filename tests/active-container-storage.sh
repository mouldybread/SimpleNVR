#!/usr/bin/env bash
set -euo pipefail
image=${1:?usage: bash tests/active-container-storage.sh IMAGE}
work=$(mktemp -d "${RUNNER_TEMP:-/tmp}/simplenvr-active-container.XXXXXX")
state=$work/state
fixture=$work/fixture
bin=$work/bin
name=simplenvr-active-storage-$$
cleanup() {
  docker logs "$name" 2>/dev/null || true
  docker rm -f "$name" >/dev/null 2>&1 || true
  sudo rm -rf -- "$work"
}
trap cleanup EXIT
mkdir -p "$state/id_continuous" "$state/id_event/temp/retained-evidence" "$fixture" "$bin"
chmod 755 "$work" "$fixture" "$bin"
printf 'synthetic retained event input\n' > "$state/id_event/temp/retained-evidence/event.mp4"
printf 'synthetic checkpoint evidence, not a valid recovery authorization\n' > "$state/id_event/temp/retained-evidence/recovery.json"
python3 - "$state/cameras.json" <<'PY'
import json,sys
cameras={}
for ident,mode in [('id_continuous','continuous'),('id_event','event_based')]:
    cameras[ident]={'id':ident,'name':ident,'rtsp_url':'rtsp://synthetic-fixture/stream','record_dir':'/recordings/'+ident,'is_recording':True,'retention_days':7,'recording_mode':mode}
with open(sys.argv[1],'w') as f:
    json.dump({'cameras':cameras,'camera_order':list(cameras),'events':[]},f)
PY
sudo chown -R 0:0 "$state"
sudo chmod 700 "$state"
docker run --rm --network none --entrypoint /usr/bin/ffmpeg \
  --mount "type=bind,src=$fixture,dst=/fixture" "$image" \
  -nostdin -v error -f lavfi -i testsrc2=size=256x144:rate=10 \
  -t 2 -c:v mpeg4 -threads 1 -an -y /fixture/source.mp4
# Substitute only the capture input. This is real FFmpeg in the runtime image,
# but it is deliberately NOT live RTSP or Dahua acceptance.
cat > "$bin/ffmpeg" <<'SH'
#!/bin/sh
segment=0
last=
for arg do
  test "$arg" != '-segment_list' || segment=1
  last=$arg
done
if test "$segment" -eq 1; then
  exec /usr/bin/ffmpeg -nostdin -v error -re -stream_loop -1 -i /fixture/source.mp4 -c copy -f segment -segment_time 5 -segment_format mp4 -reset_timestamps 1 -strftime 1 -segment_list pipe:1 -segment_list_type csv -segment_list_size 0 "$last"
fi
exec /usr/bin/ffmpeg -nostdin -v error -re -stream_loop -1 -i /fixture/source.mp4 -t 600 -c copy -f mp4 -movflags faststart -y "$last"
SH
chmod 755 "$bin/ffmpeg"
python3 -c 'import secrets; print(secrets.token_urlsafe(32))' | docker run --rm -i \
  --mount "type=bind,src=$state,dst=/recordings" "$image" auth set-password ci-admin
sudo sha256sum "$state/id_event/temp/retained-evidence/event.mp4" "$state/id_event/temp/retained-evidence/recovery.json" > "$work/evidence.before"
docker create --name "$name" --network none --read-only --cap-drop ALL --cap-add NET_BIND_SERVICE \
  --security-opt no-new-privileges --tmpfs /certs:rw,nosuid,nodev,noexec \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m \
  --env PATH=/fixture-bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
  --env SIMPLENVR_MIN_FREE_BYTES=1 \
  --mount "type=bind,src=$state,dst=/recordings" \
  --mount "type=bind,src=$fixture,dst=/fixture,readonly" \
  --mount "type=bind,src=$bin,dst=/fixture-bin,readonly" "$image" >/dev/null
for cycle in 1 2; do
  docker start "$name" >/dev/null
  active=0
  for attempt in $(seq 1 90); do
    if docker exec "$name" sh -c '
      wget --no-check-certificate -q -O /dev/null https://127.0.0.1/auth/login || exit 1
      pids=$(pidof ffmpeg) || exit 1
      set -- $pids
      test "$#" -ge 2 || exit 1
      count=$(find /recordings/id_event/buffer -type f -name "buffer_*.mp4" | wc -l)
      test "$count" -ge 2 || exit 1
      for f in /recordings/id_continuous/*.mp4.tmp; do test ! -s "$f" || exit 0; done
      exit 1
    '; then active=1; break; fi
    sleep 0.5
  done
  test "$active" -eq 1
  docker stop --timeout 75 "$name" >/dev/null
  test "$(docker inspect -f '{{.State.ExitCode}}' "$name")" -eq 0
  docker logs "$name" 2>&1 | grep -F 'Shutdown complete; recording intent persisted'
  sudo python3 - "$state" "$cycle" <<'PY'
import glob,json,os,stat,sys
root=sys.argv[1]
p=os.path.join(root,'cameras.json')
assert stat.S_IMODE(os.stat(p).st_mode)==0o600
with open(p) as f: state=json.load(f)
assert set(state['cameras'])=={'id_continuous','id_event'}
assert all(cam['is_recording'] for cam in state['cameras'].values())
outputs=glob.glob(os.path.join(root,'id_continuous','*_partial.mp4'))
assert len(outputs)>=int(sys.argv[2]) and any(os.path.getsize(p)>0 for p in outputs), 'no new retained partial output'
assert not os.path.exists(os.path.join(root,'id_event','buffer')), 'buffer worker not cleaned up'
PY
  sudo sha256sum "$state/id_event/temp/retained-evidence/event.mp4" "$state/id_event/temp/retained-evidence/recovery.json" > "$work/evidence.after"
  cmp "$work/evidence.before" "$work/evidence.after"
done
printf 'ACTIVE_CONTAINER_STORAGE_VERIFIED image_id=%s cycles=2 continuous=active event_buffer=active input=file_fixture live_rtsp=not_tested\n' "$(docker image inspect -f '{{.Id}}' "$image")"
