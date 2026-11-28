"""Bounded runtime-image export lifecycle acceptance; synthetic fixtures only."""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import socket
import ssl
import subprocess
import threading
import time
from urllib.parse import urlencode
from runtime_export_writer_observer import THREAD_PROBE, attach_thread_observations, writer_pressure_key

CAMERA = "id_export"
SOURCE = "/recordings/id_export/20261110/20261110_130000_E20261110_130012_fixture.mp4"
MARKER = "Shutdown complete; recording intent persisted"
BODY_CAP = 48 << 20
EXPECTED_FILES = ["id_export/20261110/20261110_130000_E20261110_130012_fixture.mp4",
                  "id_export/temp/retained-evidence/event.mp4",
                  "id_export/temp/retained-evidence/recovery.json",
                  "id_export/buffer/retained-unreported.mp4"]


def require(value, message):
    if not value:
        raise AssertionError(message)


def command(args, timeout=10):
    result = subprocess.run(args, capture_output=True, timeout=timeout)
    require(result.returncode == 0, "fixture command failed: " + args[0])
    require(len(result.stdout) + len(result.stderr) <= 256 << 10, "fixture command output cap")
    return result.stdout.decode("utf-8", "strict")


# Emit only ownership/queue counters. Never emit cmdlines, environment, auth
# files, request headers, network addresses or the complete process table.
PROBE = THREAD_PROBE + r"""
set -eu
test "$(readlink /proc/1/exe)" = /app/simplenvr
readlink /proc/1/fd/0 >/dev/null
for f in /proc/1/fd/*; do
  t=$(readlink "$f") || continue
  case "$t" in
    /recordings/id_export/20261110/20261110_130000_E20261110_130012_fixture.mp4|/tmp/simplenvr-export-*|/tmp/simplenvr-trim-*|pipe:*|socket:*)
      size=0
      case "$t" in /tmp/simplenvr-trim-*) size=$(stat -L -c %s "$f");; esac
      printf 'F\t%s\t%s\t%s\n' "${f##*/}" "$t" "$size";;
  esac
done
for p in /proc/[0-9]*; do
  exe=$(readlink "$p/exe") || continue
  case "$exe" in /usr/bin/ffmpeg|/usr/bin/ffprobe) ;; *) continue;; esac
  ppid=$(awk '$1=="PPid:" {print $2}' "$p/status") || continue
  test "$ppid" = 1 || continue
  printf 'P\t%s\t%s\n' "${p##*/}" "${exe##*/}"
  test "$exe" = /usr/bin/ffmpeg || continue
  src=$(readlink "$p/fd/4") || continue
  test "$src" = /recordings/id_export/20261110/20261110_130000_E20261110_130012_fixture.mp4 || continue
  pipe=$(readlink "$p/fd/1") || continue
  wait=$(cat "$p/wchan") || continue
  counts=$(awk '$1=="rchar:" {r=$2} $1=="wchar:" {w=$2} END {print r " " w}' "$p/io") || continue
  stats=$(awk '{print $14+$15, $22}' "$p/stat") || continue
  mode=copy
  if tr '\000' '\n' < "$p/cmdline" | grep -qx -- '-filter_complex'; then mode=trim; fi
  printf 'C\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "${p##*/}" "$pipe" "$wait" "$counts" "$stats" "$mode" "$src"
  if test "$mode" = copy; then observe_export_writer "$p"; fi
done
for table in /proc/1/net/tcp /proc/1/net/tcp6; do
  test -r "$table" || exit 1
  awk '$2 ~ /:01BB$/ && $4=="01" {split($5,q,":"); print "T\t" $10 "\t" q[1]}' "$table"
done
for f in /tmp/simplenvr-export-* /tmp/simplenvr-trim-*; do
  test ! -e "$f" || printf 'N\t%s\n' "${f##*/}"
done
"""


def parse_probe(text):
    require(len(text) <= 32768, "proc observation cap")
    row = {"pins": 0, "anonymous_exports": 0, "anonymous_trims": [],
           "pipes": [], "children": [], "media_children": [], "tx_queues": [], "named_temps": []}
    sockets, tcp = set(), []
    thread_records = []
    for line in text.splitlines():
        parts = line.split("\t")
        if parts[0] in ("D", "W", "B"):
            thread_records.append(parts)
        elif parts[0] == "F" and len(parts) == 4:
            _, fd, target, size = parts
            require(fd.isdigit() and size.isdigit(), "invalid descriptor observation")
            if target == SOURCE:
                row["pins"] += 1
            elif target.startswith("/tmp/simplenvr-export-"):
                require(target.endswith(" (deleted)"), "named export descriptor")
                row["anonymous_exports"] += 1
            elif target.startswith("/tmp/simplenvr-trim-"):
                require(target.endswith(" (deleted)"), "named trim descriptor")
                row["anonymous_trims"].append(int(size))
            elif re.fullmatch(r"pipe:\[\d+\]", target):
                row["pipes"].append(target)
            elif re.fullmatch(r"socket:\[\d+\]", target):
                sockets.add(target[8:-1])
            else:
                raise AssertionError("unexpected descriptor target")
        elif parts[0] == "C" and len(parts) == 8:
            _, pid, pipe, wait, counts, stats, mode, source = parts
            require(pid.isdigit() and source == SOURCE and mode in ("copy", "trim"), "invalid child identity")
            require(re.fullmatch(r"\d+ \d+", counts) and re.fullmatch(r"\d+ \d+", stats), "invalid child counters")
            read, written = map(int, counts.split())
            cpu, start = map(int, stats.split())
            row["children"].append({"pid": int(pid), "start": start, "pipe": pipe,
                                    "wait": wait, "read": read, "written": written,
                                    "cpu": cpu, "mode": mode})
        elif parts[0] == "P" and len(parts) == 3:
            require(parts[1].isdigit() and parts[2] in ("ffmpeg", "ffprobe"), "invalid media child")
            row["media_children"].append({"pid": int(parts[1]), "tool": parts[2]})
        elif parts[0] == "T" and len(parts) == 3:
            require(parts[1].isdigit() and re.fullmatch(r"[0-9A-Fa-f]+", parts[2]), "invalid TCP counters")
            tcp.append((parts[1], int(parts[2], 16)))
        elif parts[0] == "N" and len(parts) == 2:
            require(re.fullmatch(r"simplenvr-(export|trim)-[^/]+", parts[1]), "invalid temp observation")
            row["named_temps"].append(parts[1])
        else:
            raise AssertionError("malformed proc observation")
    row["tx_queues"] = sorted(queue for inode, queue in tcp if inode in sockets)
    return attach_thread_observations(row, thread_records)


def pressure_key(row, count):
    return writer_pressure_key(row, count)


class StablePressure:
    def __init__(self, count, duration=0.5):
        self.count, self.duration = count, duration
        self.key, self.since, self.samples = None, None, 0

    def observe(self, row, now):
        key = pressure_key(row, self.count)
        if key is None or key != self.key:
            self.key, self.since, self.samples = key, now, 1 if key else 0
            return False
        self.samples += 1
        return self.samples >= 3 and now - self.since >= self.duration


def clean(row):
    return not (row["pins"] or row["anonymous_exports"] or row["anonymous_trims"] or
                row["media_children"] or row["children"] or row["named_temps"])


class FixtureTLS(http.client.HTTPSConnection):
    def __init__(self, port, small_receive=False):
        self.small_receive = small_receive
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE  # Only the fixture's loopback endpoint.
        super().__init__("127.0.0.1", port, timeout=30, context=context)

    def connect(self):
        raw = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        try:
            raw.settimeout(self.timeout)
            if self.small_receive:
                raw.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4096)
            raw.connect(("127.0.0.1", self.port))
            self.sock = self._context.wrap_socket(raw, server_hostname="localhost")
        except BaseException:
            raw.close()
            raise


class PreheaderRequest:
    def __init__(self, connection):
        self.connection = connection
        self.done = threading.Event()
        self.result = None
        self.thread = threading.Thread(target=self.run, name="runtime-export-preheader", daemon=False)

    def run(self):
        try:
            response = self.connection.getresponse()
            try:
                self.result = {"status": response.status,
                               "video": response.getheader("Content-Type", "").startswith("video/"),
                               "disposition": bool(response.getheader("Content-Disposition")),
                               "trim_applied": response.getheader("X-Export-Trim-Applied") is not None}
            finally:
                response.close()
        except (OSError, http.client.HTTPException) as error:
            self.result = {"transport_error": type(error).__name__}
        finally:
            self.done.set()

    def join(self):
        self.thread.join(2)
        if self.thread.is_alive():
            if self.connection.sock is not None:
                try:
                    self.connection.sock.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
            self.connection.close()
            self.thread.join(2)
        require(not self.thread.is_alive(), "preheader client did not join")


def no_video_response(result):
    return bool(result) and ("transport_error" in result or
                            not (result["video"] or result["disposition"] or result["trim_applied"]))


SNAPSHOT = r"""
import hashlib,json,os,pathlib,stat,sys
root=pathlib.Path(sys.argv[1])
files=json.loads(sys.argv[2]); result={}
for relative in files:
    p=root/relative
    assert stat.S_ISREG(p.lstat().st_mode)
    h=hashlib.sha256()
    with p.open('rb') as f:
        for chunk in iter(lambda:f.read(1<<20),b''): h.update(chunk)
    result[relative]={'size':p.stat().st_size,'sha256':h.hexdigest()}
p=root/'cameras.json'; meta=p.stat(); data=json.loads(p.read_text())
cam=data['cameras']['id_export']
assert set(data['cameras'])=={'id_export'} and data['camera_order']==['id_export']
assert cam['id']=='id_export' and cam['is_recording'] is False
assert cam['recording_mode']=='no_recording' and cam['retention_days']==365
print(json.dumps({'files':result,'config_mode':stat.S_IMODE(meta.st_mode),
                  'config_mtime_ns':meta.st_mtime_ns,'config_size':meta.st_size,
                  'camera_intent':'no_recording','events':data['events']}))
"""


class Campaign:
    def __init__(self, args):
        self.args = args
        self.cookie = None
        self.port = None
        self.connections = []
        self.responses = []
        self.preheader = None
        self.report = {"commit": args.commit, "image_id": args.image_id, "status": "failed",
                       "runtime_qualified": False, "cleanup_authorized": False,
                       "production_defect_identified": False,
                       "cases": {name: {"status": "not-started"} for name in (
                           "whole-copy-slow-reader", "disconnect-and-both-admissions-reuse",
                           "sigterm-stream-backpressure-restart", "sigterm-preheader-trim-restart")},
                       "observations": [], "persistence": []}

    def docker(self, *args, timeout=10):
        return command(["docker", *args], timeout)

    def snapshot(self):
        return json.loads(command(["sudo", "-n", "python3", "-c", SNAPSHOT,
                                   str(self.args.state), json.dumps(EXPECTED_FILES)]))

    def observe(self):
        row = parse_probe(self.docker("exec", self.args.container, "sh", "-c", PROBE))
        if len(self.report["observations"]) < 100:
            self.report["observations"].append({"elapsed": round(time.monotonic() - self.started, 3), **row})
        return row

    def await_clean(self):
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            row = self.observe()
            if clean(row):
                return row
            time.sleep(0.1)
        raise AssertionError("owned export descriptors/children/temp entries did not clear")

    def pressure(self, count=1):
        tracker = StablePressure(count)
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            row = self.observe()
            if tracker.observe(row, time.monotonic()):
                return {"stable_samples": tracker.samples, "stable_seconds": round(time.monotonic() - tracker.since, 3),
                        "state": row}
            time.sleep(0.2)
        raise AssertionError("sustained source-matched pipe-write and owned TCP queue backpressure not reached")

    def connection(self, small_receive=False):
        conn = FixtureTLS(self.port, small_receive)
        self.connections.append(conn)
        return conn

    def close_connections(self):
        errors = []
        for response in self.responses:
            try:
                response.close()
            except OSError as error:
                errors.append(type(error).__name__)
        self.responses.clear()
        for conn in self.connections:
            try:
                conn.close()
            except OSError as error:
                errors.append(type(error).__name__)
        self.connections.clear()
        require(not errors, "client close failed")

    def login(self):
        # Resolve the published endpoint after every start; no saved browser
        # session, previous port, or previous network readiness is assumed.
        info = json.loads(self.docker("inspect", self.args.container))[0]
        require(info["Config"].get("Labels", {}).get("simplenvr.safety.fixture") == "runtime-export", "not an owned export fixture")
        require(info["Image"] == self.args.image_id, "runtime image identity changed")
        ports = info["NetworkSettings"]["Ports"]["443/tcp"]
        require(len(ports) == 1 and ports[0]["HostIp"] == "127.0.0.1", "non-loopback fixture endpoint")
        self.port = int(ports[0]["HostPort"])
        deadline = time.monotonic() + 30
        while True:
            conn = self.connection()
            try:
                conn.request("GET", "/auth/login", headers={"Connection": "close"})
                response = conn.getresponse()
                require(response.status == 200, "runtime login page unavailable")
                require(len(response.read(65537)) <= 65536, "login response cap")
                break
            except (OSError, http.client.HTTPException):
                if time.monotonic() >= deadline:
                    raise AssertionError("runtime restart HTTPS readiness timed out") from None
                time.sleep(0.2)
            finally:
                conn.close()
        conn = self.connection()
        try:
            body = urlencode({"username": "ci-export", "password": os.environ["SIMPLENVR_E2E_PASSWORD"]})
            conn.request("POST", "/auth/login", body, {"Content-Type": "application/x-www-form-urlencoded", "Connection": "close"})
            response = conn.getresponse()
            require(response.status == 303 and response.getheader("Location") == "/", "runtime session login failed")
            cookies = response.msg.get_all("Set-Cookie", [])
            require(len(cookies) == 1, "unexpected runtime session cookies")
            self.cookie = cookies[0].split(";", 1)[0]
            require(self.cookie and "\r" not in self.cookie and "\n" not in self.cookie, "invalid session cookie")
            require(len(response.read(65537)) <= 65536, "login response cap")
        finally:
            conn.close()
        self.close_connections()

    def request(self, trim=False, small_receive=False):
        query = {"camera_id": CAMERA, "start_time": "2026-11-10T13:00:00.2Z" if trim else "2026-11-10T13:00:00Z",
                 "end_time": "2026-11-10T13:00:11.8Z" if trim else "2026-11-10T13:00:12Z"}
        conn = self.connection(small_receive)
        conn.request("GET", "/api/download?" + urlencode(query), headers={"Cookie": self.cookie, "Connection": "close"})
        return conn

    def whole(self, small_receive=False):
        conn = self.request(small_receive=small_receive)
        response = conn.getresponse()
        self.responses.append(response)
        require(response.status == 200 and response.getheader("Content-Type") == "video/mp4" and
                response.getheader("X-Export-Trim-Applied") == "false" and
                response.getheader("X-Export-Input-Policy") == "pinned-descriptors", "whole-copy video response required")
        return conn, response

    def unchanged(self):
        snapshot = self.snapshot()
        require(snapshot["files"] == self.before["files"] and snapshot["events"] == self.before["events"], "source or retained synthetic evidence changed")
        return snapshot

    def shutdown(self, label):
        # Docker logs may put application records on either output stream.
        def markers():
            result = subprocess.run(["docker", "logs", "--tail", "400", self.args.container], capture_output=True, timeout=10)
            require(result.returncode == 0 and len(result.stdout) + len(result.stderr) <= 256 << 10, "shutdown log observation failed")
            combined = result.stdout + result.stderr
            return combined.count(MARKER.encode()), b"Shutdown/server failure:" in combined
        old_count, _ = markers()
        before = self.snapshot()
        started = time.monotonic()
        self.docker("stop", "--timeout", "75", self.args.container, timeout=80)
        info = json.loads(self.docker("inspect", self.args.container))[0]
        count, failure = markers()
        require(not info["State"]["Running"] and info["State"]["ExitCode"] == 0 and not info["State"]["OOMKilled"], "runtime shutdown was not a clean exit")
        require(count == old_count + 1 and not failure, "new successful persistence/shutdown marker missing")
        after = self.unchanged()
        require(after["config_mode"] == 0o600 and after["config_mtime_ns"] != before["config_mtime_ns"], "fresh private persisted configuration not observed")
        record = {"case": label, "seconds": round(time.monotonic() - started, 3), "exit_code": 0,
                  "new_completion_markers": count - old_count, "state": after,
                  "internal_persistence_order_observed": False}
        self.report["persistence"].append(record)
        return record

    def restart(self):
        self.close_connections()
        self.cookie = None
        self.docker("start", self.args.container)
        self.login()
        self.unchanged()
        self.await_clean()

    def case(self, name, action):
        row = self.report["cases"][name] = {"status": "running"}
        self.save()
        try:
            action(row)
            row["status"] = "passed"
        except BaseException as error:
            row["status"] = "failed"
            row["failure_type"] = type(error).__name__
            if isinstance(error, AssertionError):
                row["failure"] = str(error)[:500]
            raise
        finally:
            self.save()

    def save(self):
        self.args.evidence.mkdir(parents=True, exist_ok=True)
        (self.args.evidence / "runtime-export-report.json").write_text(json.dumps(self.report, indent=2, allow_nan=False) + "\n")

    def slow_copy(self, row):
        _, response = self.whole()
        path = self.args.evidence / "runtime-slow-whole.mp4"
        total = reads = 0
        with path.open("wb") as output:
            while True:
                chunk = response.read(8192)
                if not chunk:
                    break
                total += len(chunk)
                reads += 1
                require(total <= BODY_CAP, "whole-copy body cap")
                output.write(chunk)
                time.sleep(0.002)
        response.close()
        self.close_connections()
        require(total >= 8 << 20 and reads > 100, "slow-reader control fixture too small")
        command(["ffmpeg", "-nostdin", "-v", "error", "-i", str(path), "-f", "null", "-"], timeout=30)
        duration = float(json.loads(command(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "json", str(path)]))["format"]["duration"])
        require(abs(duration - 12) < 0.3, "slow-copy duration mismatch")
        row.update({"bytes": total, "reads": reads, "duration": duration,
                    "sha256": hashlib.sha256(path.read_bytes()).hexdigest(), "external_decode": "passed"})
        self.await_clean()
        self.unchanged()

    def disconnect(self, row):
        conn, response = self.whole(small_receive=True)
        row["pressure"] = self.pressure()
        response.close()
        conn.close()
        self.close_connections()
        row["after_disconnect"] = self.await_clean()
        self.unchanged()
        responses = []
        try:
            for _ in range(2):
                _, response = self.whole(small_receive=True)
                responses.append(response)
            row["both_slots_reused"] = self.pressure(2)
        finally:
            for response in responses:
                response.close()
            self.close_connections()
        row["after_reuse"] = self.await_clean()
        self.unchanged()

    def stream_shutdown(self, row):
        _, response = self.whole(small_receive=True)
        try:
            row["pressure"] = self.pressure()
            row["shutdown"] = self.shutdown("stream-backpressure")
        finally:
            response.close()
            self.close_connections()
        self.restart()
        row["restart_authenticated_clean"] = True

    def trim_shutdown(self, row):
        conn = self.request(trim=True)
        self.preheader = PreheaderRequest(conn)
        self.preheader.thread.start()
        deadline, initial = time.monotonic() + 8, {}
        while time.monotonic() < deadline:
            require(not self.preheader.done.is_set(), "trim responded before active render/shutdown witness")
            state = self.observe()
            children = state["children"]
            if len(children) == 1 and children[0]["mode"] == "trim" and state["pins"] == 1 and any(state["anonymous_trims"]):
                child = children[0]
                key = (child["pid"], child["start"])
                if key in initial and child["cpu"] > initial[key]:
                    row["render_before_headers"] = state
                    break
                initial.setdefault(key, child["cpu"])
            time.sleep(0.2)
        else:
            raise AssertionError("active source-matched rendering with output and CPU progress not reached")
        require(not self.preheader.done.is_set(), "trim headers arrived before SIGTERM")
        row["shutdown"] = self.shutdown("trim-preheader")
        self.preheader.join()
        row["client_result"] = self.preheader.result
        require(no_video_response(self.preheader.result), "canceled preheader trim started a video response")
        self.preheader = None
        self.restart()
        # Reuse both admissions after the second restart as well.
        responses = []
        try:
            for _ in range(2):
                _, response = self.whole(small_receive=True)
                responses.append(response)
            row["restart_both_slots"] = self.pressure(2)
        finally:
            for response in responses:
                response.close()
            self.close_connections()
        self.await_clean()
        self.unchanged()

    def run(self):
        self.started = time.monotonic()
        try:
            self.before = self.snapshot()
            size = self.before["files"][EXPECTED_FILES[0]]["size"]
            require(8 << 20 <= size <= 40 << 20, "source fixture outside reviewed size bounds")
            self.report["fixture_before"] = self.before
            self.login()
            self.await_clean()
            self.case("whole-copy-slow-reader", self.slow_copy)
            self.case("disconnect-and-both-admissions-reuse", self.disconnect)
            self.case("sigterm-stream-backpressure-restart", self.stream_shutdown)
            self.case("sigterm-preheader-trim-restart", self.trim_shutdown)
            self.report["final_shutdown"] = self.shutdown("final-idle-fixture")
            self.report["status"] = "passed"
        except BaseException as error:
            self.report["failure_type"] = type(error).__name__
            if isinstance(error, AssertionError):
                self.report["failure"] = str(error)[:500]
            raise
        finally:
            try:
                if self.preheader is not None:
                    self.preheader.join()
                self.close_connections()
            except BaseException as error:
                self.report["status"] = "failed"
                self.report["cleanup_failure_type"] = type(error).__name__
                if isinstance(error, AssertionError):
                    self.report["cleanup_failure"] = str(error)[:500]
                raise
            finally:
                self.save()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--container", required=True)
    parser.add_argument("--state", required=True, type=Path)
    parser.add_argument("--evidence", required=True, type=Path)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--image-id", required=True)
    args = parser.parse_args()
    require(re.fullmatch(r"simplenvr-runtime-export-[0-9]+", args.container), "fixture name required")
    require(re.fullmatch(r"[0-9a-f]{40}", args.commit), "exact candidate SHA required")
    require(args.state.is_absolute() and args.state.name == "state" and
            args.state.parent.name.startswith("simplenvr-runtime-export."), "private fixture state required")
    try:
        Campaign(args).run()
    except Exception as error:
        raise SystemExit("Runtime export acceptance failed: " + type(error).__name__) from None


if __name__ == "__main__":
    main()
