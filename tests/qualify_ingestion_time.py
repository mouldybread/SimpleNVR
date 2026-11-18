#!/usr/bin/env python3
"""Offline qualification of a CANDIDATE, not production ingestion-time support.

Requires stock ffmpeg/ffprobe, libx264 and AAC. Uses generated media, temporary
files and loopback UDP only. Run: python3 tests/qualify_ingestion_time.py report.json
Exit 0 means this limited fixture passed, 1 means candidate unqualified, 2 means
infrastructure/fixture failure. None of these outcomes certifies RTSP or coverage.
"""
import csv
import io
import json
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import tempfile
import time


def run(args, timeout=10):
    p = subprocess.run(args, capture_output=True, text=True, timeout=timeout)
    if p.returncode:
        raise RuntimeError(f"fixture command exited {p.returncode}: {p.stderr[-4000:]}")
    return p.stdout


def stop(p):
    if p is None:
        return
    if p.poll() is None:
        p.terminate()
        try:
            p.wait(timeout=2)
        except subprocess.TimeoutExpired:
            p.kill()
    p.wait(timeout=2)


def capture(ffmpeg, ffprobe, source, directory, wallclock):
    directory.mkdir()
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    # The brief bind/launch gap is not an exclusive port reservation. A collision
    # or missing stream is a fixture error, never a successful qualification.
    url = f"udp://127.0.0.1:{port}"
    args = [ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "warning", "-nostats"]
    if wallclock:
        args += ["-copyts", "-use_wallclock_as_timestamps", "1"]
    args += ["-i", url + "?timeout=1000000", "-c", "copy", "-f", "segment",
             "-segment_time", "1", "-segment_format", "mp4", "-reset_timestamps", "1",
             "-segment_list", "pipe:1", "-segment_list_type", "csv",
             "-segment_list_size", "0", str(directory / "part_%03d.mp4")]
    sender = receiver = None
    with tempfile.TemporaryFile() as receiver_log, tempfile.TemporaryFile() as sender_log:
        before = time.time()
        try:
            receiver = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=receiver_log)
            time.sleep(0.3)
            sender = subprocess.Popen(
                [ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-re",
                 "-i", str(source), "-c", "copy", "-output_ts_offset", "3600",
                 "-f", "mpegts", url + "?pkt_size=1316"],
                stdout=subprocess.DEVNULL, stderr=sender_log)
            stdout, _ = receiver.communicate(timeout=7)
            sender.wait(timeout=2)
            if sender.returncode:
                sender_log.seek(0)
                raise RuntimeError("fixture sender failed: " + sender_log.read(4000).decode(errors="replace"))
            receiver_log.seek(0)
            diagnostics = receiver_log.read(12000).decode(errors="replace")
        finally:
            stop(receiver)
            stop(sender)
            if receiver is not None and receiver.stdout is not None:
                receiver.stdout.close()
        after = time.time()
    rows = list(csv.reader(io.StringIO(stdout.decode("utf-8", errors="strict"))))
    if not rows or any(len(row) != 3 for row in rows):
        raise RuntimeError("missing or malformed completion CSV")
    packets = []
    segments = []
    for path in sorted(directory.glob("part_*.mp4")):
        data = json.loads(run([ffprobe, "-v", "error", "-select_streams", "v:0",
                              "-show_entries", "packet=pts_time,dts_time",
                              "-of", "json", str(path)]))
        video = data.get("packets", [])
        if not video or any("pts_time" not in p or "dts_time" not in p for p in video):
            raise RuntimeError("missing video packet timestamps")
        packets.extend(video)
        segments.append({"name": path.name, "video_packets": len(video)})
    if not packets:
        raise RuntimeError("no output video packets")
    offsets = sorted({round(float(p["pts_time"]) - float(p["dts_time"]), 6) for p in packets})
    return {"receiver_exit": receiver.returncode, "host_window": [before, after],
            "csv": rows, "segments": segments, "video_packets": len(packets),
            "composition_offsets_seconds": offsets, "diagnostics": diagnostics}


def qualify():
    ffmpeg, ffprobe = shutil.which("ffmpeg"), shutil.which("ffprobe")
    if not ffmpeg or not ffprobe:
        raise RuntimeError("ffmpeg and ffprobe are required")
    with tempfile.TemporaryDirectory(prefix="nvr-ingestion-qualification-") as root:
        root = Path(root)
        source = root / "source.mp4"
        run([ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
             "-i", "testsrc2=size=160x120:rate=10", "-f", "lavfi", "-i",
             "sine=frequency=440:sample_rate=48000", "-t", "3", "-c:v", "libx264",
             "-preset", "fast", "-g", "10", "-bf", "2", "-sc_threshold", "0",
             "-c:a", "aac", "-y", str(source)])
        control = capture(ffmpeg, ffprobe, source, root / "control", False)
        candidate = capture(ffmpeg, ffprobe, source, root / "candidate", True)
        if control["receiver_exit"] != 0 or control["video_packets"] != 30 or not any(
                offset > 0 for offset in control["composition_offsets_seconds"]):
            raise RuntimeError("control did not preserve the expected B-frame fixture")
        before, after = candidate["host_window"]
        checks = {
            "receiver_exited_zero": candidate["receiver_exit"] == 0,
            "csv_starts_in_observed_host_window": all(
                before <= float(row[1]) <= after for row in candidate["csv"]),
            "same_video_packet_count": candidate["video_packets"] == control["video_packets"],
            "same_composition_offset_values": (
                candidate["composition_offsets_seconds"] == control["composition_offsets_seconds"]),
        }
        return {"ffmpeg": run([ffmpeg, "-version"]).splitlines()[0],
                "scope": "Generated H.264 B-frames + AAC over loopback UDP/MPEG-TS, source offset 3600s; not RTSP/application/deployment acceptance",
                "candidate_flags": ["-copyts", "-use_wallclock_as_timestamps", "1"],
                "capture_duration_flag_tested": False,
                "limitation": "CSV/packet inspection only: no decoded-frame equivalence, AAC timing validation, UTC accuracy, clock-step, capture -t, segment-to-output correspondence or positive cleanup proof",
                "control": control, "candidate": candidate, "checks": checks,
                "limited_fixture_passed": all(checks.values()),
                "runtime_qualified": False}


if __name__ == "__main__":
    if len(sys.argv) != 2:
        raise SystemExit("usage: qualify_ingestion_time.py REPORT.json")
    try:
        report = qualify()
        code = 0 if report["limited_fixture_passed"] else 1
    except Exception as error:
        report = {"infrastructure_error": str(error), "runtime_qualified": False}
        code = 2
    Path(sys.argv[1]).write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    raise SystemExit(code)
