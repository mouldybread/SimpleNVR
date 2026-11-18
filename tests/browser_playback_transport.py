"""Test-only real TLS media truncation. No production proxy or media fallback."""
import asyncio
import hashlib
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import re
import socket
import ssl
import subprocess
import tempfile
import threading
import time

from browser_playback_edges import (FIRST, STATE, evaluate, wait, frame, require,
                                    prepare_page, check_playing, require_observer, native_event)

MAX_MEDIA = 4 << 20
MAX_REQUESTS = 32


def certificate(directory):
    directory = Path(directory)
    key, cert = directory / "key.pem", directory / "cert.pem"
    subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
                    "-subj", "/CN=localhost", "-addext", "subjectAltName=IP:127.0.0.1",
                    "-keyout", str(key), "-out", str(cert)], check=True, timeout=10,
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return cert, key


def byte_range(header, size):
    if header is None:
        return 0, size - 1, 200
    require(isinstance(header, str) and len(header) <= 64, "range header invalid")
    match = re.fullmatch(r"bytes=([0-9]+)-([0-9]*)", header)
    require(match is not None, "unsupported range")
    start = int(match[1]);end = int(match[2]) if match[2] else size - 1
    require(0 <= start <= end and start < size, "unsatisfied range")
    return start, min(end, size - 1), 206


class TLSMedia:
    """Serve only fixed in-memory fixture bytes on an ephemeral loopback port."""
    def __init__(self, media, cert, key):
        require(type(media) is bytes and 65536 <= len(media) <= MAX_MEDIA, "bounded nonempty media fixture required")
        self.media = media
        self.prefix = min(96 << 10, len(media) - 16384)
        self.lock = threading.Lock()
        self.cut = threading.Event();self.closing = threading.Event();self.prefix_sent = threading.Event()
        self.requests = [];self.active = self.started = self.finished = self.excess = 0
        self.fault_claimed = False
        self.cut_requested_at = self.cut_at = None
        self.cut_reason = None
        self.errors = []
        tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        tls.load_cert_chain(str(cert), str(key))
        owner = self
        class Server(ThreadingHTTPServer):
            daemon_threads = False
            block_on_close = True
            allow_reuse_address = False
            def get_request(self):
                raw, address = super().get_request()
                raw.settimeout(5)
                try:
                    return tls.wrap_socket(raw, server_side=True), address
                except BaseException:
                    raw.close();raise
            def handle_error(self, request, address):
                with owner.lock: owner.errors.append("unhandled relay handler failure")
        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"
            def log_message(self, *args): pass
            def do_HEAD(self): self.respond(False)
            def do_GET(self): self.respond(True)
            def respond(self, send_body):
                with owner.lock:
                    owner.started += 1;owner.active += 1
                    row = {"path": self.path.split('?', 1)[0], "method": self.command,
                           "status": None, "bytes_written": 0}
                    allowed = len(owner.requests) < MAX_REQUESTS
                    if allowed: owner.requests.append(row)
                    else: owner.excess += 1
                try:
                    if not allowed:
                        self.close_connection = True;return
                    path = row["path"]
                    if path not in ("/control.mp4", "/fault.mp4"):
                        self.empty(row, 404);return
                    try:
                        start, end, status = byte_range(self.headers.get("Range"), len(owner.media))
                    except AssertionError:
                        self.empty(row, 416, content_range="bytes */" + str(len(owner.media)));return
                    faulty = path == "/fault.mp4" and send_body
                    with owner.lock:
                        refuse = faulty and owner.fault_claimed
                        if faulty and not refuse: owner.fault_claimed = True
                    if refuse:
                        self.empty(row, 503);return
                    # A fault control must start at the beginning and advertise
                    # the full source range, never an honestly short response.
                    if faulty and (start != 0 or end != len(owner.media) - 1):
                        self.empty(row, 416);return
                    row.update(status=status, start=start, end=end, advertised=end-start+1)
                    self.send_response(status)
                    self.send_header("Content-Type", "video/mp4")
                    self.send_header("Content-Length", str(end-start+1))
                    self.send_header("Accept-Ranges", "bytes")
                    self.send_header("Cache-Control", "no-store")
                    self.send_header("Connection", "close")
                    if status == 206:
                        self.send_header("Content-Range", f"bytes {start}-{end}/{len(owner.media)}")
                    self.end_headers();self.close_connection = True
                    if not send_body: return
                    data = owner.media[:owner.prefix] if faulty else owner.media[start:end+1]
                    row["bytes_written"] = self.wfile.write(data)
                    self.wfile.flush()
                    if faulty:
                        owner.prefix_sent.set()
                        signaled = owner.cut.wait(12)
                        with owner.lock:
                            owner.cut_at = time.monotonic()
                            owner.cut_reason = "closing" if owner.closing.is_set() else "requested" if signaled else "gate-timeout"
                        try: self.connection.shutdown(socket.SHUT_RDWR)
                        except OSError: pass
                except (OSError, ssl.SSLError) as error:
                    row["socket_error"] = type(error).__name__
                finally:
                    with owner.lock:
                        owner.active -= 1;owner.finished += 1
            def empty(self, row, status, content_range=None):
                row["status"] = status
                self.send_response(status)
                self.send_header("Content-Length", "0")
                self.send_header("Cache-Control", "no-store")
                self.send_header("Connection", "close")
                if content_range: self.send_header("Content-Range", content_range)
                self.end_headers();self.close_connection = True
        self.server = Server(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, kwargs={"poll_interval": 0.02}, daemon=False)
        self.origin = "https://127.0.0.1:" + str(self.server.server_address[1])

    def start(self):
        self.thread.start()

    def request_cut(self):
        require(self.prefix_sent.is_set(), "fault prefix not sent")
        with self.lock: self.cut_requested_at = time.monotonic()
        self.cut.set()

    def close(self):
        self.closing.set();self.cut.set()
        if self.thread.is_alive(): self.server.shutdown()
        self.server.server_close()
        if self.thread.ident is not None: self.thread.join(timeout=2)
        with self.lock:
            require(not self.thread.is_alive() and self.active == 0 and self.started == self.finished,
                    "TLS relay work not joined")

    def snapshot(self):
        with self.lock:
            return {"source_bytes": len(self.media), "source_sha256": hashlib.sha256(self.media).hexdigest(),
                    "prefix_bytes": self.prefix, "requests": [dict(r) for r in self.requests],
                    "active": self.active, "started": self.started, "finished": self.finished,
                    "excess_requests": self.excess, "errors": list(self.errors),
                    "cut_requested_at": self.cut_requested_at, "cut_at": self.cut_at,
                    "cut_reason": self.cut_reason, "listener_joined": not self.thread.is_alive()}


def event_count(state, name, path):
    return sum(e.get("name") == name and e.get("path") == path and e.get("trusted") is True for e in state["events"])


async def transport_case(context, origin, legacy, diagnostics, report):
    # Fetch exactly one authenticated real backend fixture. The relay never
    # receives credentials or chooses an outbound URL, and serves no other file.
    response = await context.request.get(origin + FIRST, timeout=15000)
    require(response.status == 200 and response.headers.get("content-type", "").split(';')[0] == "video/mp4",
            "transport source was not real authenticated MP4")
    length = response.headers.get("content-length", "")
    require(length.isdigit() and 65536 <= int(length) <= MAX_MEDIA, "bounded source Content-Length required")
    media = await response.body()
    require(len(media) == int(length), "source body length mismatch")
    require(65536 <= len(media) <= MAX_MEDIA and media[4:8] == b"ftyp", "transport fixture outside contract")
    page = cdp = relay = None
    report.update({"checks_passed": [], "test_tls_relay": True,
                   "source_selection": "direct native src injection into established application session",
                   "production_response_replaced": False, "fake_media_events": False})
    with tempfile.TemporaryDirectory(prefix="simplenvr-browser-tls-") as directory:
        cert, key = await asyncio.to_thread(certificate, directory)
        try:
            relay = TLSMedia(media, cert, key);relay.start()
            page, cdp = await prepare_page(context, origin, legacy, diagnostics)
            await evaluate(cdp, "videoPlayers[0].muted=true;seekToTime(new Date('2026-11-10T12:00:01Z'));true")
            await wait(cdp, "isPlaying && !videoPlayers[0].paused && videoPlayers[0].currentTime>1.2")
            check_playing(await evaluate(cdp, STATE), FIRST)
            await page.locator("#play-pause").click()
            await evaluate(cdp, "videoPlayers[0].src=" + json.dumps(relay.origin + "/control.mp4") + ";true")
            await wait(cdp, "videoPlayers[0].readyState>=1 && videoPlayers[0].currentSrc.includes('/control.mp4')")
            await page.locator("#play-pause").click()
            await wait(cdp, "isPlaying && !videoPlayers[0].paused && videoPlayers[0].currentTime>0.2")
            control = await evaluate(cdp, STATE);check_playing(control, "/control.mp4", 0.2)
            report["positive_control"] = {"state": control, "decoded_frame": await frame(cdp)}
            report["checks_passed"].append("real-tls-positive-control-decoded")
            await page.locator("#play-pause").click()
            await evaluate(cdp, "videoPlayers[0].src=" + json.dumps(relay.origin + "/fault.mp4") + ";true")
            await wait(cdp, "videoPlayers[0].readyState>=1 && videoPlayers[0].currentSrc.includes('/fault.mp4')")
            await page.locator("#play-pause").click()
            await wait(cdp, "isPlaying && !videoPlayers[0].paused && videoPlayers[0].currentTime>0.2")
            before = await evaluate(cdp, STATE);check_playing(before, "/fault.mp4", 0.2)
            report["before_cut"] = {"state": before, "decoded_frame": await frame(cdp)}
            report["decoded_before_cut_at"] = time.monotonic()
            relay.request_cut()
            await wait(cdp, "videoPlayers[0] && (videoPlayers[0].error!==null || !videoPlayers[0].currentSrc.includes('/fault.mp4'))")
            failed = await evaluate(cdp, STATE);require_observer(failed)
            report["after_cut"] = failed
            visibility = await evaluate(cdp, "({hidden:videoPlayers[0].style.display==='none',unavailable:getComputedStyle(videoPlayers[0].parentElement.querySelector('.no-video-message')).display==='block',pending:mediaCleanups.length})")
            report["failure_visibility"] = visibility
            require(visibility["hidden"] is True and visibility["unavailable"] is True and visibility["pending"] == 0,
                    "late error did not expose unavailable media and clear pending load")
            require(failed["path"] == "/fault.mp4" and failed["error"] in (2, 3)
                    and failed["paused"] is True and failed["playingIntent"] is False and failed["sessions"] == 0,
                    "late transport error did not stop/invalidate application playback")
            require(native_event(failed, "error", "/fault.mp4"), "trusted late native error absent")
            report["checks_passed"].append("post-decode-tls-truncation-stops-application")
            await asyncio.sleep(0.4)
            stable = await evaluate(cdp, STATE)
            require(stable["path"] == failed["path"] and stable["generation"] == failed["generation"]
                    and stable["playingIntent"] is False, "late error silently advanced or restarted")
            await page.locator("#play-pause").click()
            refused = await evaluate(cdp, STATE)
            require(refused["playingIntent"] is False and refused["sessions"] == 0 and refused["path"] == "/fault.mp4"
                    and "Select a recording time" in refused["text"], "invalidated source restarted without selection")
            report["no_seek_retry"] = refused
            report["checks_passed"].append("invalidated-media-needs-new-selection")
            previous = event_count(refused, "loadedmetadata", FIRST)
            await evaluate(cdp, "seekToTime(new Date('2026-11-10T12:00:01Z'));true")
            await wait(cdp, "isPlaying && !videoPlayers[0].paused && !videoPlayers[0].error && videoPlayers[0].currentTime>1.2")
            recovered = await evaluate(cdp, STATE);check_playing(recovered, FIRST)
            require(recovered["generation"] > failed["generation"] and event_count(recovered, "loadedmetadata", FIRST) > previous,
                    "retry did not establish new native media/session observation")
            report["recovery"] = {"state": recovered, "decoded_frame": await frame(cdp)}
            report["checks_passed"].append("explicit-timeline-seek-recovers-native-playback")
        finally:
            try:
                if page is not None:
                    try: report["final_state"] = await evaluate(cdp, STATE)
                    finally: await page.close()
            finally:
                if relay is not None:
                    try:
                        await asyncio.to_thread(relay.close)
                    except BaseException as error:
                        report["relay_cleanup_failure"] = type(error).__name__ + ": " + str(error)[:500]
                        raise
                    finally:
                        report["relay"] = relay.snapshot()
        snapshot = report["relay"]
        require(snapshot["cut_reason"] == "requested" and snapshot["cut_at"] >= snapshot["cut_requested_at"] >= report["decoded_before_cut_at"],
                "cut was not gated by witnessed native decoding")
        require(snapshot["listener_joined"] and snapshot["active"] == 0 and not snapshot["errors"] and snapshot["excess_requests"] == 0,
                "transport fixture failed cleanup or request ceiling")
        fault = [r for r in snapshot["requests"] if r["path"] == "/fault.mp4" and r["method"] == "GET"]
        require(fault and fault[0]["bytes_written"] == snapshot["prefix_bytes"] < fault[0]["advertised"]
                and all(r["status"] == 503 and r["bytes_written"] == 0 for r in fault[1:]), "fault delivery/retry contract changed")
