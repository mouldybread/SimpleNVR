"""Real HTTPS application acceptance: native playback and prepared export jobs.
No application functions, media events or HTTP response bodies are replaced.
"""
import argparse
import asyncio
import hashlib
import json
import os
import subprocess
from collections import deque
from pathlib import Path
from urllib.parse import urlsplit
from playwright.async_api import async_playwright, Error as PlaywrightError
from browser_ci_diagnostics import BrowserNetworkEvidence, observe_login_post
from prepared_export_protocol import create_ready, release

BASE = "2026-11-10T12:00:"
CAMERA = "id_browser"


def media_metadata(path):
    subprocess.run(["ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", str(path), "-f", "null", "-"], check=True, timeout=30, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
    result = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "stream=codec_type,codec_name,duration:format=duration", "-of", "json", str(path)], check=True, timeout=10, capture_output=True)
    return json.loads(result.stdout)


def duration(metadata):
    videos = [stream for stream in metadata["streams"] if stream["codec_type"] == "video"]
    assert len(videos) == 1 and videos[0]["codec_name"] == "h264", metadata
    return float(videos[0].get("duration", metadata["format"]["duration"]))


def decode(path):
    # Preserve the baseline helper's float-returning interface for edge suites.
    return duration(media_metadata(path))


def packet_hashes(path):
    result = subprocess.run(["ffprobe", "-v", "error", "-select_streams", "v:0", "-show_packets", "-show_data_hash", "sha256", "-show_entries", "packet=data_hash", "-of", "json", str(path)], check=True, timeout=15, capture_output=True)
    packets = json.loads(result.stdout)["packets"]
    assert packets and all(p.get("data_hash", "").startswith("SHA256:") for p in packets)
    return [p["data_hash"] for p in packets]


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
        report["login_navigation_recovery"] = {"method": "GET", "path": "/", "attempt": 1, "reason": "net::ERR_NETWORK_CHANGED", "completed": False}
        recovered = await page.goto(root, wait_until="load")
        assert recovered is not None and recovered.status == 200 and page.url == root
        report["login_navigation_recovery"]["completed"] = True
    roots = [event for event in network_events if event.get("event") == "response" and event.get("path") == "/" and event.get("method") == "GET" and event.get("resource_type") == "document"]
    assert page.url == root and roots and roots[-1]["status"] == 200


async def application_ready(page, origin):
    assert page.url == origin + "/"
    await page.wait_for_function("typeof allCameras !== 'undefined' && allCameras.some(c => c.id === 'id_browser')")
    picker = page.locator("#timeline-date-picker")
    await picker.fill("2026-11-10")
    await picker.dispatch_event("change")
    await page.wait_for_function("timelineSegments.some(s => s.camera_id === 'id_browser' && s.start_time === '2026-11-10T12:00:00Z')")
    await page.wait_for_function("!isUpdatingGrid && timelineAbort === null && loadDataTimeoutId === null && videoPlayers.length === 1")
    assert await page.locator("#video-grid video").count() == 1


async def prepare_ui(page, start, end, mode="accurate"):
    # Export uses every camera selected for playback; the fixture selects only id_browser.
    assert await page.evaluate("JSON.stringify(selectedCameras)") == '["id_browser"]'
    await page.locator("#download-btn").click()
    assert await page.locator("#export-panel").is_visible()
    assert await page.locator("#timeline-range-overlay").is_visible()
    await page.evaluate("([start,end]) => {downloadStartTime=new Date(start);downloadEndTime=new Date(end);updateDownloadTimeDisplay();}", [start, end])
    await page.locator("#export-mode-" + mode).click()
    assert await page.locator("#export-mode-" + mode).get_attribute("aria-pressed") == "true"
    await page.locator("#export-start-btn").click()


async def native_playback(page, report):
    checks = report["checks_passed"]
    report["before_first_seek"] = await playback_snapshot(page)
    await page.evaluate("() => {videoPlayers[0].muted=true;seekToTime(new Date('2026-11-10T12:00:01Z'));}")
    await page.wait_for_function("videoPlayers[0]?.error || (videoPlayers[0]?.readyState >= 2 && videoPlayers[0].currentTime > 1.3 && !videoPlayers[0].paused)")
    state = await playback_snapshot(page)
    report["first_playback"] = state
    video = state["players"][0]
    assert video["error"] is None and video["readyState"] >= 2 and video["currentTime"] > 1.3 and not video["paused"], state
    frames = await page.evaluate("() => new Promise((resolve,reject) => {const timer=setTimeout(()=>reject(new Error('No native decoded-frame callback within 5 seconds')),5000);videoPlayers[0].requestVideoFrameCallback((_,m) => {clearTimeout(timer);resolve({frames:m.presentedFrames,width:m.width,height:m.height});});})")
    assert frames["frames"] > 0 and frames["width"] == 256 and frames["height"] == 144
    report["native_frame_witness"] = frames
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
    await page.evaluate("() => {videoPlayers[0].src='/recordings/id_browser/missing.mp4';}")
    await page.wait_for_function("!isPlaying && videoPlayers[0].error !== null")
    await page.evaluate("seekToTime(new Date('2026-11-10T12:00:01Z'))")
    await page.wait_for_function("videoPlayers[0]?.readyState >= 2 && !videoPlayers[0].paused && videoPlayers[0].currentTime > 1.2 && !videoPlayers[0].error")
    checks.append("native-404-media-failure-and-application-retry")
    await page.evaluate("togglePlayPause()")


async def exports(page, context, p, args, report):
    checks, request = report["checks_passed"], context.request
    selection = {"camera_id": CAMERA, "start_time": BASE + "00Z", "end_time": BASE + "16Z", "mode": "copy"}
    anonymous = await p.request.new_context(ignore_https_errors=True)
    try:
        for path in ("/api/exports/status?id=" + "a"*64, "/api/exports/download?id=" + "a"*64, "/recordings/id_browser/20261110_120000_fixture.mp4"):
            response = await anonymous.get(args.origin + path)
            assert response.status == 401
            assert response.headers.get("content-type", "").split(";")[0] != "video/mp4"
        for path, body in (("/api/exports/create", selection), ("/api/exports/cancel", {"id": "a"*64})):
            assert (await anonymous.post(args.origin + path, data=body)).status == 401
    finally:
        await anonymous.dispose()
    checks.append("unauthenticated-prepared-routes-and-private-media-refused")

    whole = await create_ready(request, args.origin, selection)
    response = await request.get(args.origin + whole["download_url"])
    assert response.status == 200 and response.headers["content-type"].split(";")[0] == "video/mp4"
    raw = await response.body()
    assert len(raw) == whole["bytes"]
    saved = args.evidence / "runtime-whole-copy.mp4"
    saved.write_bytes(raw)
    assert abs(duration(await asyncio.to_thread(media_metadata, saved)) - 16) < 0.3
    sources = []
    for stamp in ("120000", "120008"):
        source = args.evidence / (stamp + "-source.mp4")
        fetched = await request.get(args.origin + f"/recordings/id_browser/20261110_{stamp}_fixture.mp4")
        assert fetched.status == 200
        source.write_bytes(await fetched.body())
        sources.extend(await asyncio.to_thread(packet_hashes, source))
    assert await asyncio.to_thread(packet_hashes, saved) == sources, "whole-copy video payload changed"
    head = await request.head(args.origin + whole["download_url"])
    assert head.status == 200 and int(head.headers["content-length"]) == len(raw)
    part = await request.get(args.origin + whole["download_url"], headers={"Range": "bytes=0-11"})
    assert part.status == 206 and await part.body() == raw[:12]
    assert part.headers["content-range"] == f"bytes 0-11/{len(raw)}"
    await release(request, args.origin, whole)
    checks.append("prepared-copy-packet-hashes-decode-head-range-and-release")

    response = await request.post(args.origin + "/api/exports/create", data={**selection, "start_time": BASE + "15Z", "end_time": BASE + "20Z"})
    assert response.status == 409 and response.headers.get("x-export-plan-status") == "gap"
    assert response.headers.get("content-type", "").split(";")[0] != "video/mp4"
    checks.append("prepared-gap-refused-without-download")

    audio = await create_ready(request, args.origin, {**selection, "mode": "accurate", "start_time": "2026-11-10T12:30:00.5Z", "end_time": "2026-11-10T12:30:03.5Z"})
    response = await request.get(args.origin + audio["download_url"])
    assert response.status == 200
    audio_path = args.evidence / "accurate-audio.mp4"
    audio_path.write_bytes(await response.body())
    metadata = await asyncio.to_thread(media_metadata, audio_path)
    assert abs(duration(metadata) - 3) < 0.3
    tracks = [s for s in metadata["streams"] if s["codec_type"] == "audio"]
    assert len(tracks) == 1 and tracks[0]["codec_name"] == "aac"
    assert abs(float(tracks[0]["duration"]) - 3) < 0.25
    await release(request, args.origin, audio)
    checks.append("prepared-accurate-audio-track-duration-and-decode")

    start, end = BASE + "00.5Z", BASE + "15.5Z"
    # Copy extends the range to safe cuts without any approval step: the first
    # create request is accepted and the job reports the range it will save.
    async with page.expect_response(lambda r: urlsplit(r.url).path == "/api/exports/create") as created:
        await prepare_ui(page, start, end, "copy")
    accepted = await created.value
    assert accepted.status == 202, accepted.status
    body = await accepted.json()
    assert body.get("saved_start") and body.get("saved_end"), body
    assert "accepted_" not in (accepted.request.post_data or "")
    await page.wait_for_selector('.export-job[data-state="ready"]', timeout=90000)
    assert await page.evaluate("exportUiBatch.jobs[0].mode") == "copy"
    assert await page.evaluate("exportUiBatch.jobs[0].serverId !== null")
    covered = await page.evaluate("() => {const j=exportUiBatch.jobs[0];return !!j.saved && Date.parse(j.saved.start)<=Date.parse(j.start) && Date.parse(j.saved.end)>=Date.parse(j.end) && Date.parse(j.saved.start)>=Date.parse(j.start)-30000 && Date.parse(j.saved.end)<=Date.parse(j.end)+30000;}")
    assert covered, body
    assert "Saved range" in await page.locator(".export-job").inner_text()
    await page.get_by_role("button", name="Release prepared file / continue", exact=True).click()
    await page.wait_for_function("!exportUiBatch.running")
    checks.append("real-copy-range-extension-saved-range-and-release")

    await prepare_ui(page, start, end, "accurate")
    pids = await wait_pids(args.container, True)
    await page.locator(".export-job").get_by_role("button", name="Cancel", exact=True).click()
    await page.wait_for_selector('.export-job[data-state="canceled"]')
    await page.wait_for_function("!exportUiBatch.running")
    await wait_pids(args.container, False)
    assert await page.evaluate("exportUiBatch.jobs[0].url") is None
    report["canceled_runtime_ffmpeg_pids"] = pids
    checks.append("ui-cancel-active-paced-runtime-render-and-child-reap")

    await prepare_ui(page, start, end, "accurate")
    await page.wait_for_selector('.export-job[data-state="ready"]', timeout=90000)
    assert "re-encoded" in await page.locator(".export-job").inner_text()
    url = await page.evaluate("exportUiBatch.jobs[0].url")
    assert url.startswith("/api/exports/download?id=") and not url.startswith("blob:")
    response = await request.get(args.origin + url)
    assert response.status == 200
    received_hash = hashlib.sha256(await response.body()).hexdigest()
    async with page.expect_download() as pending:
        await page.get_by_role("button", name="Download prepared video", exact=True).click()
    download = await pending.value
    assert await download.failure() is None
    saved = args.evidence / "browser-saved-trim.mp4"
    await download.save_as(saved)
    assert hashlib.sha256(saved.read_bytes()).hexdigest() == received_hash
    span = duration(await asyncio.to_thread(media_metadata, saved))
    assert abs(span - 15) < 0.3, span
    assert "not verified" in await page.locator(".export-job").inner_text()
    await page.get_by_role("button", name="Release prepared file / continue", exact=True).click()
    await page.wait_for_function("!exportUiBatch.running && exportUiBatch.jobs[0].url === null")
    assert (await request.get(args.origin + url)).status == 404
    await wait_pids(args.container, False)
    checks.append("real-ui-save-hash-duration-decode-and-server-release")
    report.update({"saved_trim_sha256": received_hash, "saved_trim_duration_seconds": span})
    await export_queue_panel(page, context, args, report)
    await export_range_handles(page, report)


async def export_queue_panel(page, context, args, report):
    # Session queue panel against the real server: list, download link and Clear all.
    checks, request = report["checks_passed"], context.request
    await prepare_ui(page, BASE + "00.5Z", BASE + "15.5Z", "copy")
    await page.wait_for_selector('.export-job[data-state="ready"]', timeout=90000)
    job_id = await page.evaluate("exportUiBatch.jobs[0].serverId")
    assert job_id and len(job_id) == 64, job_id
    await page.locator("#export-refresh-btn").click()
    item = page.locator(f'#export-queue-list .export-queue-item[data-job-id="{job_id}"]')
    await item.wait_for(timeout=30000)
    assert await item.get_attribute("data-state") == "ready"
    link = item.locator("a")
    assert await link.count() == 1
    assert await link.get_attribute("href") == "/api/exports/download?id=" + job_id
    listing = await (await request.get(args.origin + "/api/exports/list")).json()
    assert any(job["id"] == job_id and job["state"] == "ready" for job in listing["jobs"]), listing
    expected = hashlib.sha256(await (await request.get(args.origin + "/api/exports/download?id=" + job_id)).body()).hexdigest()
    async with page.expect_download() as pending:
        await link.click()
    download = await pending.value
    assert await download.failure() is None
    saved = args.evidence / "browser-queue-download.mp4"
    await download.save_as(saved)
    assert hashlib.sha256(saved.read_bytes()).hexdigest() == expected
    assert duration(await asyncio.to_thread(media_metadata, saved)) >= 14.5
    checks.append("real-queue-panel-list-and-download-link-saves-decodable-file")

    await page.locator("#export-clear-btn").click()
    await page.wait_for_function("document.getElementById('export-queue-list').textContent.includes('No exports in this session')")
    assert (await request.get(args.origin + "/api/exports/download?id=" + job_id)).status == 404
    listing = await (await request.get(args.origin + "/api/exports/list")).json()
    assert listing["jobs"] == [], listing
    assert "Clear all" in await page.locator(".export-job").inner_text()
    await page.wait_for_function("!exportUiBatch.running")
    checks.append("real-clear-all-removes-session-jobs-and-downloads")


async def export_range_handles(page, report):
    # Range bar over the real timeline canvas: keyboard, pointer drag, clamp and close.
    checks = report["checks_passed"]
    await page.evaluate("() => {downloadStartTime=new Date('2026-11-10T11:00:00Z');downloadEndTime=new Date('2026-11-10T13:00:00Z');updateDownloadTimeDisplay();}")
    assert await page.locator("#timeline-range-overlay").is_visible()
    await page.locator("#timeline-range-end-handle").focus()
    before = await page.evaluate("downloadEndTime.getTime()")
    await page.keyboard.press("ArrowRight")
    assert await page.evaluate("downloadEndTime.getTime()") == before + 60000
    await page.keyboard.press("ArrowLeft")
    assert await page.evaluate("downloadEndTime.getTime()") == before
    checks.append("real-range-handle-keyboard")

    async def drag_start_handle(fraction):
        canvas = await page.locator("#timeline-canvas").bounding_box()
        handle = await page.locator("#timeline-range-start-handle").bounding_box()
        y = handle["y"] + handle["height"] / 2
        await page.mouse.move(handle["x"] + handle["width"] / 2, y)
        await page.mouse.down()
        await page.mouse.move(canvas["x"] + canvas["width"] * fraction, y, steps=8)
        await page.mouse.up()

    await drag_start_handle(0.3)
    expected = await page.evaluate("TimelineGeometry.timeAt(0.3, timelineViewport())")
    actual = await page.evaluate("downloadStartTime.getTime()")
    assert abs(actual - expected) < 120000, (actual, expected)
    await drag_start_handle(0.9)
    assert await page.evaluate("downloadStartTime.getTime()") == await page.evaluate("downloadEndTime.getTime() - 1000")
    checks.append("real-range-handle-pointer-drag-and-clamp")

    await page.get_by_role("button", name="Close export panel", exact=True).click()
    assert not await page.locator("#timeline-range-overlay").is_visible()
    assert not await page.locator("#export-panel").is_visible()
    assert await page.locator("#download-btn").get_attribute("aria-expanded") == "false"
    checks.append("real-export-panel-close-hides-range-bar")


async def run(args):
    args.evidence.mkdir(parents=True, exist_ok=True)
    report = {"commit": args.commit, "image_id": args.image_id, "backend": "real HTTPS runtime container", "routed_responses": False, "checks_passed": [], "status": "failed", "runtime_qualified": False, "cleanup_authorized": False, "limits": "Synthetic non-reordered H264/AAC; headless Chrome; deterministic state-directed ranges. No live RTSP, OS save dialogs, multi-GiB or full-resolution camera qualification."}
    async with BrowserNetworkEvidence(args.evidence, args.origin) as diagnostics, async_playwright() as p:
        browser = await p.chromium.launch(channel="chrome", headless=True)
        report.update({"browser": browser.version, "browser_channel": "chrome", "playwright_version": diagnostics.playwright_version, "network_diagnostics": "browser-network-evidence.json"})
        context = await browser.new_context(ignore_https_errors=True, accept_downloads=True, timezone_id="UTC")
        page, errors, media_network = None, [], deque(maxlen=100)
        def remember(response):
            path = urlsplit(response.url).path
            if path.startswith('/recordings/') or path in ('/api/timeline/recordings', '/auth/login', '/'):
                media_network.append({"event":"response","path":path,"method":response.request.method,"status":response.status,"resource_type":response.request.resource_type})
        def failed(request):
            path = urlsplit(request.url).path
            if path.startswith('/recordings/') or path in ('/api/timeline/recordings', '/auth/login', '/'):
                media_network.append({"event":"requestfailed","path":path,"method":request.method,"resource_type":request.resource_type,"failure":request.failure})
        try:
            context.set_default_timeout(30000)
            page = await context.new_page()
            await diagnostics.attach(context, page)
            report["codec_support"] = await page.evaluate("() => {const v=document.createElement('video');return {h264:v.canPlayType('video/mp4; codecs=\"avc1.64000a\"'),aac:v.canPlayType('audio/mp4; codecs=\"mp4a.40.2\"')};}")
            assert report["codec_support"]["h264"]
            page.on("pageerror", lambda e: errors.append(str(e)[:1000]) if len(errors) < 30 else None)
            page.on("response", remember)
            page.on("requestfailed", failed)
            await page.goto(args.origin + "/auth/login")
            await page.locator('[name="username"]').fill("ci-browser")
            await page.locator('[name="password"]').fill(os.environ["SIMPLENVR_E2E_PASSWORD"])
            await submit_login_once(page, args.origin, report, media_network)
            report["checks_passed"].append("real-login-form-session")
            await application_ready(page, args.origin)
            report["checks_passed"].append("real-camera-template-and-timeline")
            report["private_media_preflight"] = await private_media_preflight(context, args.origin)
            report["checks_passed"].append("authenticated-private-media-range-preflight")
            await native_playback(page, report)
            await exports(page, context, p, args, report)
            assert not errors, errors
            report["status"] = "passed-with-navigation-recovery" if report.get("login_navigation_recovery") else "passed"
        except BaseException as error:
            report["failure"] = type(error).__name__ + ": " + str(error)[:2000]
            if page is not None:
                try:
                    report["failure_playback_snapshot"] = await asyncio.wait_for(playback_snapshot(page), timeout=5)
                except Exception as diagnostic_error:
                    report["diagnostic_failure"] = str(diagnostic_error)[:1000]
            raise
        finally:
            report["page_errors"], report["media_network"] = errors, list(media_network)
            (args.evidence / "runtime-browser-report.json").write_text(json.dumps(report, indent=2) + "\n")
            print(json.dumps(report, indent=2), flush=True)
            await context.close()
            await browser.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    for name in ("origin", "container", "commit", "image-id"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    asyncio.run(run(parser.parse_args()))
