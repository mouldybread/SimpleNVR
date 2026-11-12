"""Live HTTPS application/runtime-image acceptance; no routed responses or DOM stubs."""
import argparse
import asyncio
import hashlib
import json
import os
import subprocess
from pathlib import Path
from collections import deque
from urllib.parse import urlencode, urlsplit
from playwright.async_api import async_playwright, Error as PlaywrightError
from browser_ci_diagnostics import BrowserNetworkEvidence, observe_login_post

BASE = "2026-11-10T12:00:"
CAMERA = "id_browser"


def export_path(start, end):
    return "/api/download?" + urlencode({"camera_id": CAMERA, "start_time": start, "end_time": end})


def decode(path):
    subprocess.run(["ffmpeg", "-nostdin", "-v", "error", "-i", str(path), "-f", "null", "-"], check=True, timeout=30, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    result = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "json", str(path)], check=True, timeout=10, capture_output=True)
    return float(json.loads(result.stdout)["format"]["duration"])


def ffmpeg_pids(container):
    result = subprocess.run(["docker", "exec", container, "pidof", "ffmpeg"], timeout=10, capture_output=True, text=True)
    assert result.returncode in (0, 1), result.stderr
    return result.stdout.split()


async def wait_pids(container, present):
    for _ in range(100):
        pids = await asyncio.to_thread(ffmpeg_pids, container)
        if bool(pids) == present:
            return pids
        await asyncio.sleep(0.1)
    raise AssertionError("runtime FFmpeg ownership did not reach expected state")


async def playback_snapshot(page):
    return await page.evaluate("""() => {
        const finite=value=>Number.isFinite(value)?value:null;
        const players=typeof videoPlayers==='undefined'?[]:videoPlayers;
        return {
            gridUpdating:typeof isUpdatingGrid==='undefined'?null:isUpdatingGrid,
            playingIntent:typeof isPlaying==='undefined'?null:isPlaying,
            pendingSeek:typeof pendingSeekTime==='undefined'?null:pendingSeekTime,
            generation:typeof seekGeneration==='undefined'?null:seekGeneration,
            sessions:typeof playbackSessions==='undefined'?null:playbackSessions.size,
            selected:typeof selectedCameras==='undefined'?[]:selectedCameras,
            segments:typeof timelineSegments==='undefined'?[]:timelineSegments.slice(0,8),
            players:players.filter(Boolean).slice(0,4).map(video=>({
                source:video.getAttribute('src'),currentSource:video.currentSrc,
                connected:video.isConnected,muted:video.muted,paused:video.paused,
                readyState:video.readyState,networkState:video.networkState,
                currentTime:finite(video.currentTime),duration:finite(video.duration),
                error:video.error?{code:video.error.code,message:video.error.message}:null,
                mp4Support:video.canPlayType('video/mp4'),
                h264Support:video.canPlayType('video/mp4; codecs="avc1.64000a"')
            })),
            visibleText:document.body.innerText.slice(-2000)
        };
    }""")


async def private_media_preflight(context, origin):
    path = "/recordings/id_browser/20261110_120000_fixture.mp4"
    response = await context.request.get(origin + path, headers={"Range": "bytes=0-11"})
    body = await response.body()
    result = {"status": response.status, "type": response.headers.get("content-type"), "content_range": response.headers.get("content-range"), "prefix_hex": body[:12].hex()}
    assert response.status == 206, result
    assert response.headers.get("content-type", "").split(";")[0] == "video/mp4", result
    assert len(body) == 12 and body[4:8] == b"ftyp", result
    return result


async def submit_login_once(page, origin, report, network_events):
    root = origin + "/"
    # Observe the real form POST; never resubmit credentials on a network error.
    response = await observe_login_post(page, origin, report)
    location = response.headers.get("location", "")
    report["login_response"] = {"status": response.status, "redirect_path": urlsplit(location).path}
    assert response.status == 303 and location == "/", report["login_response"]
    try:
        await page.wait_for_url(root, wait_until="load")
    except PlaywrightError as error:
        report["login_navigation_failure"] = {"error": str(error)[:2000], "page_url": page.url}
        changed = any(event.get("event") == "requestfailed" and event.get("path") == "/" and event.get("method") == "GET" and event.get("resource_type") == "document" and event.get("failure") == "net::ERR_NETWORK_CHANGED" for event in network_events)
        if not changed:
            raise
        # Only this observed error, after a proven successful POST, gets one
        # GET recovery. A second failure propagates; there is no retry loop.
        report["login_navigation_recovery"] = {"method": "GET", "path": "/", "attempt": 1, "reason": "net::ERR_NETWORK_CHANGED", "completed": False}
        recovered = await page.goto(root, wait_until="load")
        assert recovered is not None and recovered.status == 200 and page.url == root, "Login GET recovery did not return the authenticated root document"
        report["login_navigation_recovery"]["completed"] = True
    roots = [event for event in network_events if event.get("event") == "response" and event.get("path") == "/" and event.get("method") == "GET" and event.get("resource_type") == "document"]
    assert page.url == root and roots and roots[-1]["status"] == 200, "Authenticated root navigation did not complete successfully"


async def application_ready(page, origin):
    assert page.url == origin + "/", "Application readiness requires the authenticated root document"
    await page.wait_for_function("typeof allCameras !== 'undefined' && allCameras.some(c => c.id === 'id_browser')")
    picker = page.locator("#timeline-date-picker")
    await picker.fill("2026-11-10")
    await picker.dispatch_event("change")
    await page.wait_for_function("timelineSegments.some(s => s.camera_id === 'id_browser' && s.start_time === '2026-11-10T12:00:00Z')")
    await page.wait_for_function("!isUpdatingGrid && timelineAbort === null && loadDataTimeoutId === null && videoPlayers.length === 1")
    assert await page.locator("#video-grid video").count() == 1


async def prepare_ui(page, start, end):
    if await page.locator("#download-modal").is_visible():
        await page.locator("#download-modal .close").click()
    await page.locator("#download-btn").click()
    # There are no short-range text inputs in this UI. Select a deterministic
    # range through its real application state, without replacing functions.
    await page.evaluate("([start,end]) => {downloadCameras=['id_browser']; downloadStartTime=new Date(start); downloadEndTime=new Date(end);}", [start, end])
    await page.locator("#export-start-btn").click()


async def run(args):
    args.evidence.mkdir(parents=True, exist_ok=True)
    report = {"commit": args.commit, "image_id": args.image_id, "backend": "real HTTPS runtime container", "routed_responses": False, "checks_passed": [], "status": "failed", "limits": "Synthetic H264/AAC files, headless Chromium, test-directed seek/range selection. No live RTSP, OS save dialog, late transport truncation, metadata-delay or autoplay-denial acceptance."}
    checks = report["checks_passed"]
    password = os.environ["SIMPLENVR_E2E_PASSWORD"]
    async with BrowserNetworkEvidence(args.evidence, args.origin) as diagnostics, async_playwright() as p:
        browser = await p.chromium.launch(channel="chrome", headless=True)
        report["browser"] = browser.version
        report["browser_channel"] = "chrome"
        report["playwright_version"] = diagnostics.playwright_version
        report["network_diagnostics"] = "browser-network-evidence.json"
        context = await browser.new_context(ignore_https_errors=True, accept_downloads=True, timezone_id="UTC")
        page = None
        errors = []
        media_network = deque(maxlen=100)
        def remember_response(response):
            path = urlsplit(response.url).path
            if path.startswith('/recordings/') or path in ('/api/timeline/recordings', '/auth/login', '/'):
                media_network.append({"event":"response","path":path,"method":response.request.method,"status":response.status,"resource_type":response.request.resource_type})
        def remember_failure(request):
            path = urlsplit(request.url).path
            if path.startswith('/recordings/') or path in ('/api/timeline/recordings', '/auth/login', '/'):
                media_network.append({"event":"requestfailed","path":path,"method":request.method,"resource_type":request.resource_type,"failure":request.failure})
        try:
            context.set_default_timeout(30000)
            page = await context.new_page()
            await diagnostics.attach(context, page)
            report["codec_support"] = await page.evaluate("""() => {
                const video=document.createElement('video');
                return {h264:video.canPlayType('video/mp4; codecs="avc1.64000a"'),aac:video.canPlayType('audio/mp4; codecs="mp4a.40.2"')};
            }""")
            assert report["codec_support"]["h264"], "Selected Chrome channel lacks required H264 support: " + json.dumps(report["codec_support"])
            page.on("pageerror", lambda error: errors.append(str(error)[:1000]) if len(errors)<30 else None)
            page.on("response", remember_response)
            page.on("requestfailed", remember_failure)
            await page.goto(args.origin + "/auth/login")
            await page.locator('[name="username"]').fill("ci-browser")
            await page.locator('[name="password"]').fill(password)
            await submit_login_once(page, args.origin, report, media_network)
            checks.append("real-login-form-session")
            await application_ready(page, args.origin)
            checks.append("real-camera-template-and-timeline")
            report["private_media_preflight"] = await private_media_preflight(context, args.origin)
            checks.append("authenticated-private-media-range-preflight")
            report["before_first_seek"] = await playback_snapshot(page)

            await page.evaluate("() => {videoPlayers[0].muted=true; seekToTime(new Date('2026-11-10T12:00:01Z'));}")
            await page.wait_for_function("videoPlayers[0]?.error || (videoPlayers[0]?.readyState >= 2 && videoPlayers[0].currentTime > 1.3 && !videoPlayers[0].paused)")
            state = await playback_snapshot(page)
            report["first_playback"] = state
            video = state["players"][0]
            assert video["error"] is None and video["readyState"] >= 2 and video["currentTime"] > 1.3 and not video["paused"], state
            frames = await page.evaluate("() => new Promise((resolve,reject) => {const timer=setTimeout(()=>reject(new Error('No native decoded-frame callback within 5 seconds')),5000);videoPlayers[0].requestVideoFrameCallback((_,m) => {clearTimeout(timer);resolve({frames:m.presentedFrames,width:m.width,height:m.height});});})")
            assert frames["frames"] > 0 and frames["width"] == 256 and frames["height"] == 144
            checks.append("native-h264-playback-decoded-frames")
            await page.evaluate("togglePlayPause()")
            paused = await page.evaluate("videoPlayers[0].currentTime")
            await page.wait_for_timeout(400)
            assert await page.evaluate("videoPlayers[0].paused && !isPlaying")
            assert abs(await page.evaluate("videoPlayers[0].currentTime") - paused) < 0.1
            await page.evaluate("togglePlayPause()")
            await page.wait_for_function("before => !videoPlayers[0].paused && videoPlayers[0].currentTime > before + 0.2", arg=paused)
            checks.append("application-pause-resume-with-native-clock")
            await page.evaluate("() => {videoPlayers[0].currentTime=7.6;}")
            await page.wait_for_function("videoPlayers[0]?.currentSrc.includes('20261110_120008') && videoPlayers[0].currentTime > 0.1 && !videoPlayers[0].paused")
            checks.append("native-ended-contiguous-clip-transition")
            # A real 404 and native media error, not a dispatched fake event.
            await page.evaluate("() => {videoPlayers[0].src='/recordings/id_browser/missing.mp4';}")
            await page.wait_for_function("!isPlaying && videoPlayers[0].error !== null")
            await page.evaluate("seekToTime(new Date('2026-11-10T12:00:01Z'))")
            await page.wait_for_function("videoPlayers[0]?.readyState >= 2 && !videoPlayers[0].paused && videoPlayers[0].currentTime > 1.2 && !videoPlayers[0].error")
            checks.append("native-404-media-failure-and-application-retry")
            await page.evaluate("togglePlayPause()")

            anonymous = await p.request.new_context(ignore_https_errors=True)
            try:
                for path in [export_path(BASE + "00Z", BASE + "16Z"), "/recordings/id_browser/20261110_120000_fixture.mp4"]:
                    response = await anonymous.get(args.origin + path)
                    assert response.status == 401
                    assert response.headers.get("content-type", "").split(";")[0] != "video/mp4"
            finally:
                await anonymous.dispose()
            checks.append("unauthenticated-export-and-private-media-refused")

            response = await context.request.get(args.origin + export_path(BASE + "00Z", BASE + "16Z"), timeout=90000)
            assert response.status == 200, (response.status, await response.text())
            assert response.headers.get("x-export-range-policy") == "whole-clips-untrimmed"
            whole = args.evidence / "runtime-whole-copy.mp4"
            whole.write_bytes(await response.body())
            duration = await asyncio.to_thread(decode, whole)
            assert abs(duration - 16) < 0.3, duration
            checks.append("runtime-whole-clip-export-external-decode")

            for start, end, status, header, value in [
                (BASE + "15Z", BASE + "20Z", 409, "x-export-plan-status", "gap"),
                ("2026-11-10T12:30:00.5Z", "2026-11-10T12:30:03.5Z", 422, "x-export-trim-status", "unsupported"),
            ]:
                response = await context.request.get(args.origin + export_path(start, end), timeout=30000)
                assert response.status == status, (response.status, await response.text())
                assert response.headers.get(header) == value
                assert response.headers.get("content-type", "").split(";")[0] != "video/mp4"
            checks.append("runtime-gap-and-audio-trim-refusals")

            start, end = BASE + "00.5Z", BASE + "15.5Z"
            await prepare_ui(page, start, end)
            pids = await wait_pids(args.container, True)
            await page.locator(".export-job").get_by_role("button", name="Cancel", exact=True).click()
            await page.wait_for_selector('.export-job[data-state="canceled"]')
            await page.wait_for_function("!exportUiBatch.running")
            await wait_pids(args.container, False)
            assert await page.evaluate("exportUiBatch.jobs[0].url") is None
            report["canceled_runtime_ffmpeg_pids"] = pids
            checks.append("ui-cancel-active-paced-runtime-render-and-child-reap")

            await prepare_ui(page, start, end)
            await page.wait_for_selector('.export-job[data-state="ready"]', timeout=90000)
            assert "Frame-granular trim" in await page.locator(".export-job").inner_text()
            url = await page.evaluate("exportUiBatch.jobs[0].url")
            received_hash = await page.evaluate("async url => {const bytes=await (await fetch(url)).arrayBuffer();return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',bytes))).map(x=>x.toString(16).padStart(2,'0')).join('');}", url)
            async with page.expect_download() as pending:
                await page.get_by_role("button", name="Hand received file to browser", exact=True).click()
            download = await pending.value
            assert await download.failure() is None
            saved = args.evidence / "browser-saved-trim.mp4"
            await download.save_as(saved)
            assert hashlib.sha256(saved.read_bytes()).hexdigest() == received_hash
            duration = await asyncio.to_thread(decode, saved)
            assert abs(duration - 15) < 0.3, duration
            assert "not verified" in await page.locator(".export-job").inner_text()
            await page.get_by_role("button", name="Release prepared file / continue", exact=True).click()
            await page.wait_for_function("!exportUiBatch.running && exportUiBatch.jobs[0].url === null")
            assert await page.evaluate("async url => {try{await fetch(url);return false;}catch(_){return true;}}", url)
            await wait_pids(args.container, False)
            checks.append("real-ui-trim-save-hash-duration-decode-and-object-url-release")
            report.update({"saved_trim_sha256": received_hash, "saved_trim_duration_seconds": duration, "native_frame_witness": frames})
            assert not errors, errors
            report["status"] = "passed-with-navigation-recovery" if report.get("login_navigation_recovery") else "passed"
        except Exception as error:
            report["failure"] = type(error).__name__ + ": " + str(error)[:2000]
            if page is not None:
                try:
                    report["failure_playback_snapshot"] = await asyncio.wait_for(playback_snapshot(page), timeout=5)
                except Exception as diagnostic_error:
                    report["diagnostic_failure"] = str(diagnostic_error)[:1000]
            raise
        finally:
            report["page_errors"] = errors
            report["media_network"] = list(media_network)
            print(json.dumps({key:report[key] for key in ["status", "checks_passed", "browser", "browser_channel", "playwright_version", "network_diagnostics", "codec_support", "login_submission", "login_response", "login_navigation_failure", "login_navigation_recovery", "failure", "failure_playback_snapshot", "private_media_preflight", "page_errors", "media_network"] if key in report}, indent=2), flush=True)
            (args.evidence / "runtime-browser-report.json").write_text(json.dumps(report, indent=2) + "\n")
            await context.close()
            await browser.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ["origin", "container", "commit", "image-id"]:
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    asyncio.run(run(parser.parse_args()))
