"""Native playback edge acceptance. No play overrides or fabricated media events."""
import asyncio
import time
from urllib.parse import urlsplit

FIRST = "/recordings/id_browser/20261110_120000_fixture.mp4"
SECOND = "/recordings/id_browser/20261110_120008_fixture.mp4"
AUDIO = "/recordings/id_browser/20261110_123000_audio.mp4"
CASES = ("metadata-pause-resume", "metadata-retarget", "paused-native-end-resume", "autoplay-denial-retry")
EVENT_SCRIPT = r"""(() => {
 const events=[];window.__nvrEdgeEvents=events;
 for (const name of ['loadedmetadata','playing','pause','ended','error']) {
  document.addEventListener(name,event=>{
   const video=event.target;
   if (!(video instanceof HTMLVideoElement) || !video.closest('#video-grid')) return;
   let path='';try {path=new URL(video.currentSrc,location.href).pathname;}catch(_){}
   events.push({name,trusted:event.isTrusted,path,readyState:video.readyState,time:video.currentTime});
   if (events.length>128)events.shift();
  },true);
 }
})()"""
STATE = r"""(() => {
 const v=videoPlayers[0];
 let path='';try {path=new URL(v?.currentSrc || v?.getAttribute('src') || '',location.href).pathname;}catch(_){}
 return {path,playingIntent:isPlaying,paused:v?.paused,ended:v?.ended,readyState:v?.readyState,
  currentTime:Number.isFinite(v?.currentTime)?v.currentTime:null,error:v?.error?.code || null,
  muted:v?.muted,volume:v?.volume,sessions:playbackSessions.size,
  generation:seekGeneration,active:navigator.userActivation.hasBeenActive,
  pendingMetadata:mediaCleanups.length,events:window.__nvrEdgeEvents || [],
  text:document.body.innerText.slice(-3000)};
})()"""


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def cancelled_interception(error):
    text = str(error).lower()
    return "invalid interceptionid" in text or "invalid interception id" in text


def native_event(state, name, path):
    return any(e.get("name") == name and e.get("trusted") is True and e.get("path") == path
               for e in state.get("events", []))


def check_playing(state, path, minimum=1.2):
    require(state.get("path") == path and state.get("playingIntent") is True and state.get("paused") is False
            and state.get("error") is None and type(state.get("currentTime")) in (int, float)
            and state["currentTime"] > minimum, "native playback did not reach expected source/clock")
    require(native_event(state, "loadedmetadata", path) and native_event(state, "playing", path),
            "native trusted metadata/playing events missing")


def check_held(state, expected_intent):
    require(state.get("readyState") == 0 and state.get("paused") is True and state.get("sessions") == 0
            and state.get("pendingMetadata") == 1 and state.get("playingIntent") is expected_intent,
            "metadata hold did not preserve pending playback intent")
    require(not any(e.get("name") in ("loadedmetadata", "playing") for e in state.get("events", [])),
            "media advanced before held request was released")


def check_denied(state, rejection):
    require(state.get("path") == AUDIO and state.get("active") is False and state.get("muted") is False
            and state.get("volume") == 1 and state.get("readyState", 0) >= 1 and state.get("error") is None
            and state.get("sessions") == 1 and state.get("paused") is True and state.get("playingIntent") is False,
            "autoplay refusal was not observed in a fresh unactivated audible session")
    require("Playback could not start" in state.get("text", "") and rejection == "NotAllowedError",
            "native autoplay denial or application retry message absent")


async def evaluate(cdp, expression):
    result = await cdp.send("Runtime.evaluate", {"expression": expression, "returnByValue": True,
                                                "awaitPromise": True, "userGesture": False})
    require("exceptionDetails" not in result, "browser edge expression failed: " + str(result.get("exceptionDetails", {}))[:600])
    return result.get("result", {}).get("value")


async def wait(cdp, expression, seconds=15):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = await evaluate(cdp, expression)
        if value:
            return value
        await asyncio.sleep(0.05)
    raise AssertionError("native playback edge condition timed out: " + expression[:180])


async def frame(cdp):
    result = await evaluate(cdp, r"""new Promise(resolve=>{
      const timer=setTimeout(()=>resolve({timeout:true}),5000);
      videoPlayers[0].requestVideoFrameCallback((_,m)=>{
       clearTimeout(timer);resolve({frames:m.presentedFrames,width:m.width,height:m.height});
      });
    })""")
    require(isinstance(result, dict) and type(result.get("frames")) is int and result["frames"] > 0 and result.get("width") == 256 and result.get("height") == 144,
            "native decoded-frame witness absent")
    return result


class MediaHold:
    """Hold one request before real server delivery; never fulfill/replace bytes."""
    def __init__(self, cdp, origin):
        self.cdp, self.origin = cdp, origin
        self.queue = asyncio.Queue(maxsize=1)
        self.tasks, self.errors = set(), []
        self.armed, self.pending = True, None
        self.trace = []

    def dispatch(self, event):
        task = asyncio.create_task(self.handle(event))
        self.tasks.add(task)
        def joined(done):
            self.tasks.discard(done)
            if not done.cancelled() and done.exception():
                self.errors.append(type(done.exception()).__name__ + ": " + str(done.exception())[:500])
        task.add_done_callback(joined)

    async def handle(self, event):
        request_id = event["requestId"]
        path = urlsplit(event["request"]["url"]).path
        if self.armed and path == FIRST and event.get("resourceType") == "Media":
            self.armed, self.pending = False, request_id
            self.trace.append({"action": "held", "path": path})
            self.queue.put_nowait(time.monotonic())
            return
        await self.continue_request(request_id)

    async def continue_request(self, request_id):
        try:
            await self.cdp.send("Fetch.continueRequest", {"requestId": request_id})
            return "continued"
        except Exception as error:
            if cancelled_interception(error):
                return "already-cancelled-by-browser"
            raise

    async def start(self):
        self.cdp.on("Fetch.requestPaused", self.dispatch)
        await self.cdp.send("Network.enable")
        await self.cdp.send("Network.setCacheDisabled", {"cacheDisabled": True})
        await self.cdp.send("Fetch.enable", {"patterns": [{"urlPattern": self.origin + "/recordings/id_browser/*",
                                                          "requestStage": "Request"}]})

    async def release(self):
        if self.pending is not None:
            request_id, self.pending = self.pending, None
            outcome = await self.continue_request(request_id)
            self.trace.append({"action": outcome, "path": FIRST})
            return outcome

    async def close(self):
        try:
            await self.release()
        finally:
            self.cdp.remove_listener("Fetch.requestPaused", self.dispatch)
            try:
                await self.cdp.send("Fetch.disable")
            finally:
                if self.tasks:
                    await asyncio.wait_for(asyncio.gather(*list(self.tasks), return_exceptions=True), 5)
        require(not self.errors, "request-hold handler failed: " + str(self.errors))


async def prepare_page(context, origin, legacy, diagnostics, activated_setup=True):
    page = await context.new_page()
    cdp = await context.new_cdp_session(page)
    await cdp.send("Page.addScriptToEvaluateOnNewDocument", {"source": EVENT_SCRIPT})
    await diagnostics.attach(context, page)
    try:
        response = await page.goto(origin + "/", wait_until="load")
        require(response is not None and response.status == 200 and page.url == origin + "/",
                "edge page did not load authenticated application")
        if activated_setup:
            await legacy.application_ready(page, origin)
        else:
            await wait(cdp, "typeof allCameras!=='undefined' && allCameras.some(c=>c.id==='id_browser') && !isUpdatingGrid && timelineAbort===null && loadDataTimeoutId===null")
            await evaluate(cdp, "document.getElementById('timeline-date-picker').value='2026-11-10';document.getElementById('timeline-date-picker').dispatchEvent(new Event('change',{bubbles:true}));true")
            await wait(cdp, "timelineSegments.some(s=>s.camera_id==='id_browser' && s.start_time==='2026-11-10T12:00:00Z') && !isUpdatingGrid && timelineAbort===null && loadDataTimeoutId===null && videoPlayers.length===1")
            require(await evaluate(cdp, "navigator.userActivation.hasBeenActive") is False,
                    "autoplay page unexpectedly acquired user activation")
        return page, cdp
    except BaseException:
        await page.close()
        raise


async def metadata_case(context, origin, legacy, diagnostics, report, retarget=False):
    page, cdp = await prepare_page(context, origin, legacy, diagnostics)
    hold = MediaHold(cdp, origin)
    page_errors = []
    page.on("pageerror", lambda error: page_errors.append(str(error)[:500]) if len(page_errors) < 20 else None)
    try:
        await hold.start()
        await evaluate(cdp, "videoPlayers[0].muted=true;seekToTime(new Date('2026-11-10T12:00:01Z'));true")
        held_at = await asyncio.wait_for(hold.queue.get(), 10)
        initial = await evaluate(cdp, STATE)
        check_held(initial, True)
        if retarget:
            generation = initial["generation"]
            await evaluate(cdp, "seekToTime(new Date('2026-11-10T12:00:09Z'));true")
            await wait(cdp, "videoPlayers[0]?.currentSrc.includes('20261110_120008') && videoPlayers[0].currentTime>1.2 && !videoPlayers[0].paused && isPlaying")
            before_release = await evaluate(cdp, STATE)
            check_playing(before_release, SECOND)
            require(before_release["generation"] > generation, "retarget did not invalidate old generation")
            report["old_request_release"] = await hold.release()
            await asyncio.sleep(0.4)
            after = await evaluate(cdp, STATE)
            check_playing(after, SECOND)
            require(after["generation"] == before_release["generation"] and after["sessions"] == 1,
                    "old request affected replacement session")
            report.update({"before_old_release": before_release, "after_old_release": after})
        else:
            await page.locator("#play-pause").click()
            check_held(await evaluate(cdp, STATE), False)
            await asyncio.sleep(0.4)
            check_held(await evaluate(cdp, STATE), False)
            await page.locator("#play-pause").click()
            check_held(await evaluate(cdp, STATE), True)
            report["held_seconds"] = time.monotonic() - held_at
            require(report["held_seconds"] >= 0.4, "metadata hold window not established")
            await hold.release()
            await wait(cdp, "videoPlayers[0]?.currentTime>1.2 && !videoPlayers[0].paused && isPlaying && !videoPlayers[0].error")
            after = await evaluate(cdp, STATE)
            check_playing(after, FIRST)
            report["after_release"] = after
        report["decoded_frame"] = await frame(cdp)
        require(not page_errors, "edge page errors: " + str(page_errors))
    finally:
        try:
            report["final_state"] = await evaluate(cdp, STATE)
        finally:
            try:
                await hold.close()
            finally:
                report["request_trace"] = hold.trace
                report["handler_errors"] = hold.errors
                report["page_errors"] = page_errors
                await page.close()


async def paused_end_case(context, origin, legacy, diagnostics, report):
    page, cdp = await prepare_page(context, origin, legacy, diagnostics)
    try:
        await evaluate(cdp, "videoPlayers[0].muted=true;seekToTime(new Date('2026-11-10T12:00:01Z'));true")
        await wait(cdp, "videoPlayers[0]?.currentTime>1.2 && !videoPlayers[0].paused && isPlaying")
        await page.locator("#play-pause").click()
        await evaluate(cdp, "videoPlayers[0].currentTime=videoPlayers[0].duration;true")
        await wait(cdp, "videoPlayers[0]?.ended && videoPlayers[0].paused && !isPlaying")
        at_end = await evaluate(cdp, STATE)
        require(at_end["path"] == FIRST and at_end["error"] is None, "paused end selected wrong source")
        await asyncio.sleep(0.4)
        still = await evaluate(cdp, STATE)
        require(still["path"] == FIRST and still["ended"] is True and still["playingIntent"] is False
                and still["generation"] == at_end["generation"], "paused end advanced without explicit resume")
        await page.locator("#play-pause").click()
        await wait(cdp, "videoPlayers[0]?.currentSrc.includes('20261110_120008') && videoPlayers[0].currentTime>0.2 && !videoPlayers[0].paused && isPlaying")
        resumed = await evaluate(cdp, STATE)
        check_playing(resumed, SECOND, minimum=0.2)
        require(resumed["generation"] > at_end["generation"], "resume did not establish successor generation")
        report.update({"paused_end": at_end, "before_resume": still, "resumed": resumed, "decoded_frame": await frame(cdp)})
    finally:
        try:
            report["final_state"] = await evaluate(cdp, STATE)
        finally:
            await page.close()


async def autoplay_case(context, origin, legacy, diagnostics, report):
    page, cdp = await prepare_page(context, origin, legacy, diagnostics, activated_setup=False)
    try:
        await evaluate(cdp, "videoPlayers[0].muted=false;videoPlayers[0].volume=1;seekToTime(new Date('2026-11-10T12:30:01Z'));true")
        await wait(cdp, "videoPlayers[0]?.readyState>=1 && playbackSessions.size===1 && !isPlaying && videoPlayers[0].paused && document.body.innerText.includes('Playback could not start')")
        # Native witness without a play override; CDP never sets userGesture here.
        rejection = await evaluate(cdp, "videoPlayers[0].play().then(()=>{videoPlayers[0].pause();return 'fulfilled';},error=>error.name)")
        denied = await evaluate(cdp, STATE)
        check_denied(denied, rejection)
        await page.locator("#play-pause").click()
        await wait(cdp, "videoPlayers[0]?.currentTime>1.2 && !videoPlayers[0].paused && isPlaying && !videoPlayers[0].error")
        resumed = await evaluate(cdp, STATE)
        check_playing(resumed, AUDIO)
        require(resumed["active"] is True and resumed["muted"] is False, "trusted retry did not start audible playback")
        report.update({"denied": denied, "native_rejection": rejection, "resumed": resumed, "decoded_frame": await frame(cdp)})
    finally:
        try:
            report["final_state"] = await evaluate(cdp, STATE)
        finally:
            await page.close()
