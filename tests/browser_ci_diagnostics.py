"""Bounded metadata-only diagnostics; no routes, bodies, headers, or traces."""
import asyncio
import hashlib
import json
import re
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


def host_network_state():
    def read(path):
        try:
            return path.read_text().strip()
        except OSError:
            return "unavailable"
    try:
        links = [{"name": path.name, **{key: read(path / key) for key in ("ifindex", "operstate", "carrier", "mtu", "flags")}} for path in sorted(Path("/sys/class/net").iterdir())]
    except OSError:
        links = []
    return {"links": links, "fingerprints": {path: hashlib.sha256(read(Path(path)).encode()).hexdigest() for path in ("/proc/net/route", "/proc/net/ipv6_route", "/etc/resolv.conf")}}


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

    async def sample(self):
        previous = None
        while True:
            state = await asyncio.to_thread(self.sampler)
            if state != previous:
                self.record("host-network-state", state=state)
                previous = state
            await asyncio.sleep(0.25)

    async def __aenter__(self):
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.record("diagnostics-start", playwright_version=self.playwright_version)
        self.task = asyncio.create_task(self.sample())
        return self

    async def __aexit__(self, exc_type, exc, traceback):
        self.task.cancel()
        try:
            await self.task
        except asyncio.CancelledError:
            pass
        except Exception as error:
            self.record("sampler-failed", error_type=type(error).__name__)
        self.record("diagnostics-end")
        self.path.write_text(json.dumps({"playwright_version": self.playwright_version, "cdp_attached": self.cdp_attached, "dropped_events": self.dropped, "events": list(self.events)}, indent=2) + "\n")
        return False
