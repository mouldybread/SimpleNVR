"""Bounded metadata-only diagnostics; no routes, bodies, headers, or traces."""
import asyncio
import hashlib
import json
import re
import socket
import struct
import time
from collections import deque
from importlib.metadata import PackageNotFoundError, version
from pathlib import Path
from urllib.parse import urlsplit


def network_error(value):
    return value if isinstance(value, str) and re.fullmatch(r"net::ERR_[A-Z_]{1,100}", value) else "unclassified-network-error"


class LoginSubmissionError(RuntimeError):
    pass


async def observe_login_post(page, origin, report, timeout=30):
    outcome = asyncio.get_running_loop().create_future()
    started = time.monotonic()
    result = {"form_submissions_attempted": 1, "outcome": "pending", "response_observed": False}
    report["login_submission"] = result

    def matches(request):
        return request.url == origin + "/auth/login" and request.method == "POST"

    def responded(response):
        if matches(response.request) and not outcome.done():
            outcome.set_result(("response", response))

    def failed(request):
        if matches(request) and not outcome.done():
            outcome.set_result(("requestfailed", network_error(request.failure)))

    page.on("response", responded)
    page.on("requestfailed", failed)
    try:
        await page.get_by_role("button", name="Sign in", exact=True).click(no_wait_after=True)
        try:
            kind, value = await asyncio.wait_for(outcome, timeout)
        except asyncio.TimeoutError:
            result["outcome"] = "response-timeout"
            raise LoginSubmissionError("Login POST produced neither a response nor a failure event within the deadline") from None
        result["outcome"] = kind
        if kind == "requestfailed":
            result["failure"] = value
            raise LoginSubmissionError("Login POST failed without an observed HTTP response: " + value)
        result.update({"response_observed": True, "status": value.status})
        return value
    except BaseException:
        if result["outcome"] == "pending":
            result["outcome"] = "submission-interrupted"
        raise
    finally:
        result["elapsed_seconds"] = round(time.monotonic() - started, 3)
        page.remove_listener("response", responded)
        page.remove_listener("requestfailed", failed)
        if not outcome.done():
            outcome.cancel()



NETWORK_BYTES = 262144
ROUTE_ROWS = 2048
ROUTE_PATHS = ("/proc/net/route", "/proc/net/ipv6_route")


def bounded_network_read(path):
    try:
        with Path(path).open("rb") as stream:
            data = stream.read(NETWORK_BYTES + 1)
        if len(data) > NETWORK_BYTES:
            return None, "truncated"
        return data.decode("ascii").strip(), "ok"
    except (OSError, UnicodeError):
        return None, "unavailable"


def structural_routes(text, ipv6=False):
    if text is None:
        return {"status": "unavailable"}
    if len(text) > NETWORK_BYTES:
        return {"status": "truncated"}
    lines = text.splitlines()
    if not ipv6:
        if not lines or lines[0].split() != "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT".split():
            return {"status": "malformed"}
        lines = lines[1:]
    if len(lines) > ROUTE_ROWS:
        return {"status": "truncated"}
    rows = []
    for line in lines:
        fields = line.split()
        if len(fields) != (10 if ipv6 else 11):
            return {"status": "malformed"}
        try:
            if ipv6:
                if any(not re.fullmatch(r"[0-9a-fA-F]{32}", fields[i]) for i in (0, 2, 4)):
                    return {"status": "malformed"}
                numbers = [int(value, 16) for value in fields[:9]]
                if not 0 <= numbers[1] <= 128 or not 0 <= numbers[3] <= 128:
                    return {"status": "malformed"}
                row = [fields[9]] + [numbers[i] for i in (0, 1, 2, 3, 4, 5, 8)]
            else:
                numbers = [int(fields[i], 16 if i in (1, 2, 3, 7) else 10) for i in range(1, 11)]
                row = [fields[0]] + [numbers[i - 1] for i in (1, 2, 3, 6, 7, 8, 9, 10)]
            if any(value < 0 for value in numbers):
                return {"status": "malformed"}
        except ValueError:
            return {"status": "malformed"}
        rows.append(row)
    digest = hashlib.sha256(json.dumps(sorted(rows), separators=(",", ":")).encode()).hexdigest()
    return {"status": "ok", "entries": len(rows), "sha256": digest}


def route_notifications(data):
    """Decode only fixed numeric metadata, never addresses or attributes."""
    events, offset = [], 0
    kinds = {16: ("link", "new"), 17: ("link", "delete"), 20: ("address", "new"), 21: ("address", "delete"), 24: ("route", "new"), 25: ("route", "delete")}
    count = 0
    while offset < len(data):
        if len(data) - offset < 16 or count >= 256:
            return events, "malformed-or-limited"
        size, kind, _, _, _ = struct.unpack_from("=IHHII", data, offset)
        if size < 16 or offset + size > len(data):
            return events, "malformed"
        body = data[offset + 16:offset + size]
        if kind == 4:
            return events, "overrun"
        if kind == 2:
            if len(body) < 4 or struct.unpack_from("=i", body)[0] != 0:
                return events, "netlink-error"
        if kind in kinds:
            category, action = kinds[kind]
            minimum = {"link": 16, "address": 8, "route": 12}[category]
            if len(body) < minimum:
                return events, "malformed"
            event = {"kind": category, "action": action, "family": body[0]}
            if category in ("link", "address"):
                event["ifindex"] = struct.unpack_from("=i" if category == "link" else "=I", body, 4)[0]
            else:
                event.update(table=body[4], protocol=body[5], scope=body[6], route_type=body[7])
            events.append(event)
        offset += (size + 3) & ~3
        count += 1
    return events, None


def host_network_state():
    def read(path):
        value, status = bounded_network_read(path)
        return value if status == "ok" else status
    try:
        links = [{"name": path.name, **{key: read(path / key) for key in ("ifindex", "operstate", "carrier", "mtu", "flags")}} for path in sorted(Path("/sys/class/net").iterdir())[:64]]
    except OSError:
        links = []
    reads = {path: bounded_network_read(path) for path in (*ROUTE_PATHS, "/etc/resolv.conf")}
    return {"links": links,
            "fingerprints": {path: hashlib.sha256(value.encode()).hexdigest() if value is not None else None for path, (value, _) in reads.items()},
            "read_status": {path: status for path, (_, status) in reads.items()},
            "route_structure": {path: structural_routes(reads[path][0], ipv6=path.endswith("ipv6_route")) if reads[path][1] == "ok" else {"status": reads[path][1]} for path in ROUTE_PATHS}}


class BrowserNetworkEvidence:
    def __init__(self, evidence, origin, sampler=host_network_state):
        self.path = Path(evidence) / "browser-network-evidence.json"
        self.origin = urlsplit(origin)
        self.sampler = sampler
        self.events = deque(maxlen=512)
        self.dropped = 0
        self.started = time.monotonic()
        self.task = None
        self.cdp_attached = False
        self.route_socket = None
        self.route_loop = None
        self.route_attached = False
        try:
            self.playwright_version = version("playwright")
        except PackageNotFoundError:
            self.playwright_version = "not-installed"

    def record(self, event, **fields):
        if len(self.events) == self.events.maxlen:
            self.dropped += 1
        self.events.append({"elapsed_seconds": round(time.monotonic() - self.started, 3), "event": event, **fields})

    def category(self, url):
        parsed = urlsplit(url)
        if (parsed.scheme, parsed.netloc) != (self.origin.scheme, self.origin.netloc):
            return "other-origin"
        return {"/": "root", "/auth/login": "login", "/api/timeline/recordings": "timeline", "/api/download": "export"}.get(parsed.path, "media" if parsed.path.startswith("/recordings/") else "other-path")

    def request(self, event):
        request = event.get("request", {})
        self.record("cdp-request", request_id=event.get("requestId"), category=self.category(request.get("url", "")), method=request.get("method"), resource_type=event.get("type"))

    def response(self, event):
        response = event.get("response", {})
        self.record("cdp-response", request_id=event.get("requestId"), category=self.category(response.get("url", "")), status=response.get("status"), protocol=response.get("protocol"))

    def failure(self, event):
        self.record("cdp-loading-failed", request_id=event.get("requestId"), resource_type=event.get("type"), failure=network_error(event.get("errorText")), canceled=bool(event.get("canceled")))

    async def attach(self, context, page):
        try:
            client = await asyncio.wait_for(context.new_cdp_session(page), 5)
            client.on("Network.requestWillBeSent", self.request)
            client.on("Network.responseReceived", self.response)
            client.on("Network.loadingFailed", self.failure)
            await asyncio.wait_for(client.send("Network.enable"), 5)
            self.cdp_attached = True
            self.record("cdp-attached")
        except Exception as error:
            self.record("cdp-unavailable", error_type=type(error).__name__)

    def start_route_monitor(self):
        sock = None
        try:
            loop = asyncio.get_running_loop()
            sock = socket.socket(socket.AF_NETLINK, socket.SOCK_RAW, socket.NETLINK_ROUTE)
            sock.setblocking(False)
            # RTMGRP_LINK, IPv4/IPv6 IFADDR and IPv4/IPv6 ROUTE.
            sock.bind((0, 0x551))
            loop.add_reader(sock.fileno(), self.read_route_events)
            self.route_socket, self.route_loop = sock, loop
            self.route_attached = True
            self.record("rtnetlink-attached")
        except Exception as error:
            if sock is not None:
                try:
                    sock.close()
                except OSError:
                    pass
            self.record("rtnetlink-unavailable", error_type=type(error).__name__)

    def stop_route_monitor(self):
        sock, loop = self.route_socket, self.route_loop
        self.route_socket = self.route_loop = None
        if sock is not None:
            try:
                loop.remove_reader(sock.fileno())
            except Exception as error:
                self.record("rtnetlink-detach-failed", error_type=type(error).__name__)
            finally:
                try:
                    sock.close()
                except OSError as error:
                    self.record("rtnetlink-close-failed", error_type=type(error).__name__)

    def read_route_events(self):
        if self.route_socket is None:
            return
        try:
            data, _, flags, _ = self.route_socket.recvmsg(65536)
            if flags & socket.MSG_TRUNC:
                self.record("rtnetlink-loss", reason="datagram-truncated")
                return
            events, problem = route_notifications(data)
            for event in events:
                self.record("rtnetlink-notification", **event)
            if problem:
                self.record("rtnetlink-loss", reason=problem)
        except BlockingIOError:
            pass
        except Exception as error:
            self.record("rtnetlink-loss", reason="socket-error", error_type=type(error).__name__, errno=getattr(error, "errno", None))
            self.stop_route_monitor()

    async def sample(self):
        previous = None
        while True:
            state = await asyncio.to_thread(self.sampler)
            if state != previous:
                changed = [] if previous is None else [key for key in ("links", "fingerprints", "read_status", "route_structure") if state.get(key) != previous.get(key)]
                self.record("host-network-state", state=state, changed_sections=changed)
                previous = state
            await asyncio.sleep(0.25)

    async def __aenter__(self):
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.record("diagnostics-start", playwright_version=self.playwright_version)
        self.start_route_monitor()
        self.task = asyncio.create_task(self.sample())
        return self

    async def __aexit__(self, exc_type, exc, traceback):
        self.stop_route_monitor()
        self.task.cancel()
        try:
            await self.task
        except asyncio.CancelledError:
            pass
        except Exception as error:
            self.record("sampler-failed", error_type=type(error).__name__)
        self.record("diagnostics-end")
        self.path.write_text(json.dumps({"playwright_version": self.playwright_version, "cdp_attached": self.cdp_attached, "rtnetlink_attached": self.route_attached, "dropped_events": self.dropped, "events": list(self.events)}, indent=2) + "\n")
        return False
