# SimpleNVR

SimpleNVR records RTSP cameras with FFmpeg. Use the web interface to manage cameras, play recordings, review events, and export MP4 clips.

## Install with Docker Compose

Use the image from GitHub Container Registry. The example defaults to `v0.2.2`, which includes built-in authentication and server-prepared exports. An image's release workflow must finish publishing before it can be pulled.

The example Compose file is [docs/compose/compose.yaml](docs/compose/compose.yaml). Copy it into your install directory as `compose.yaml` before running the commands below, because its `./recordings`, `./exports` and `./certs` paths are relative to the Compose file.

Optionally set `SIMPLENVR_VERSION` in `.env` to another published version that supports these features. Unlike the old example, this Compose file actually uses that setting.

For a new installation, run:

```sh
mkdir -p recordings certs exports
chmod 700 exports
docker compose pull &&
docker compose run --rm simplenvr auth set-password admin &&
docker compose up -d
```

Use a password of at least 15 characters. On the Docker host, open `https://localhost:8443/auth/login` and sign in as `admin`. The default self-signed certificate triggers a browser warning. Set the password once; skip that step on later starts. The example listens only on localhost. For a first test, use a fresh recordings directory, not production storage.

The `exports` directory holds temporary prepared video files. Keep it private, writable by the container user, and on disk-backed storage with enough free space. It must be outside `recordings` and every camera directory, with no symlinks in the configured container path. Do not use a tmpfs or the container's writable layer for large exports. Without `NVR_EXPORT_SCRATCH_DIR`, prepared video exports are unavailable. See [large-media exports](docs/LARGE_MEDIA_EXPORTS.md) for limits and settings.

### Upgrading to v0.2.2

Keep your existing recordings, certificates, and credentials. Add the `/exports` mount and `NVR_EXPORT_SCRATCH_DIR` setting to your existing Compose service, create the host export directory with suitable permissions, then pull the image and recreate the service. Do not reset the password as part of a routine upgrade. Prepared export jobs are lost on restart; recording files are not export scratch storage.

### Recordings from versions before day folders

Earlier versions wrote recordings directly into each camera folder. Newer versions write them into day folders (see [Recording storage, retention and disk usage](#recording-storage-retention-and-disk-usage)). Recordings left directly in a camera folder stay on disk but are not shown by the timeline or exports, and nothing migrates them. They are still counted in Disk Usage, are still expired by the retention cleanup using the date at the start of the file name (or the file's modification time when the name has no usable date; very short names are kept), and are removed together with the camera.

## Session timeouts

A session ends after 20 minutes without a request, or 8 hours after sign-in, whichever comes first. To change either limit, click the lock button in the dashboard header, enter the new values in minutes, and choose Save and restart (click twice to confirm). SimpleNVR saves the values to `settings.json` in the same directory as the credentials file (`/recordings/.auth/settings.json` by default), shuts down gracefully, and relies on the container's restart policy to start it again. Keep `restart: unless-stopped` (or `always`) in your Compose file: with `on-failure` or no policy the container stays stopped until you start it. The restart stops live views and exports, pauses recording until SimpleNVR is back, and ends all current sessions. Nothing restarts if the saved values already match the running ones.

The idle timeout must be between 1 minute and 24 hours, and the maximum session length between 5 minutes and 30 days. The idle timeout cannot exceed the maximum session length. If `settings.json` is damaged or out of range, SimpleNVR refuses to start until you fix or delete it.

## Record and play back

- In Camera Settings, add an RTSP URL or import streams from go2rtc. Choose continuous recording, event-based recording with a prebuffer, or no recording. Adding a camera does not start recording.
- Live preview requires go2rtc on the host in the camera's RTSP URL, with its HTTP API on port `1984`. The camera name must be unique and match the go2rtc stream name. Direct-camera preview and custom go2rtc HTTP ports are not supported.
- Use the timeline to find recordings and events. Playback does not automatically skip gaps. Use Export range to export video; see below.
- Known continuous-recording failures appear as "Recording failed" or "Recording failed — retrying", with the recording setting and reason shown separately. "Recording enabled" is a setting, not proof that footage is being captured.
- Removing a camera can delete its published recordings. Removal is refused, with an error and the camera left registered and stopped, when the camera's folder holds anything SimpleNVR does not recognise as published recordings or empty staging folders, for example leftover capture files or event staging that is not empty. If removal fails or the result is unclear, refresh and check the camera and its recordings before retrying.

## Recording storage, retention and disk usage

- **Layout.** Published recordings are stored as `/recordings/<camera>/<YYYYMMDD>/<start>_E<end>_<label>.mp4`. The folder is the UTC date on which the recording started, not your local date, and the file name carries the UTC start and end. A recording cut short is published with `_partial` before `.mp4`. A recording that crosses midnight stays in the folder of its start day. See [recording layout](docs/RECORDING_LAYOUT.md).
- **Finding footage.** The timeline and exports choose recordings by reading file names, not by opening the media: a recording is included when it starts before the end of the requested range and ends after its start. Because the end time comes from the name, a wrongly stamped name gives a wrong range, not an error. Files whose names are not valid stamped names are left out and counted. A recording with a valid name that turns out to be unusable (empty, not a regular file, or gone) makes an export or download fail, because they need complete coverage. The timeline instead leaves that recording out, so it shows as a gap, and the playback page warns how many were left out. If a folder cannot be read completely within its limits, any of these requests fails with a 503 or 504 response instead of returning a smaller result.
- **Retention.** The retention setting (1 to 365 days per camera; the default shown is 7) is applied by a cleanup that runs every 5 minutes. A recording expires only when its stamped end is before the cutoff; one ending exactly at the cutoff is kept. Day folders after the cutoff day are not opened, so an expired file that is misplaced or misnamed in a newer folder is cleaned up later, once that folder is old enough to be inspected. See [retention selection](docs/RETENTION_SELECTION.md).
- **Disk usage.** The Disk Usage figure for each camera adds up its published recordings, loose `.mp4` files in the camera folder and the `.mp4` files in its recording buffers. It does not count symbolic links, other file types or `.mp4` files in any other folder. "Unavailable" means the folders could not be read completely or a limit was exceeded; it is never a smaller number. After a restart the first page load has to read every day folder once, and a camera that takes longer than about 10 seconds shows "Unavailable" until you refresh. See [disk usage](docs/DISK_USAGE.md).
- **Playback across clips.** Playback continues from one clip into the next only when the next clip starts exactly when the previous one ended. Stamped times are wall-clock times and often differ slightly, so playback can stop at a clip boundary. This is a known limit.

## Export video

Select the cameras to export in the Cameras selector, move the playhead to the time you want, and click Export range in the playback toolbar. An export always covers every camera currently selected for playback (up to 8); deselect a camera to leave it out. Set the start and end by dragging the two handles over the timeline or with the Earlier and Later buttons, then choose a mode with the Copy / Accurate toggle:

- Copy (default) preserves the encoded media without re-encoding. A single complete recording is copied byte for byte; compatible complete recordings can be joined without re-encoding. For part of a recording, SimpleNVR extends the range outward to the nearest safe cut (at most 30 seconds at each end) without asking, and shows the saved range once the job is created. Audio packet boundaries can reach slightly past the video range. If no safe cut is available within 30 seconds, or the video cannot be copied safely, the export is refused and you can choose Accurate instead. An unsupported copy is refused, never silently converted to Accurate.
- Accurate re-encodes supported video and audio to cut within frame and packet limits at the selected times. It is not lossless and can require more processing.

Choose Queue exports for selected cameras. SimpleNVR queues one job per camera and prepares them on the server in order. The Export queue lists every export from your session. When one is ready, click Download to save it through your browser. Release a prepared file, or use Clear all, to free server space; an already-started transfer may continue. The page cannot verify that your browser saved the file to disk.

Prepared files expire, normally 30 minutes after preparation finishes, and do not survive a restart. Exports require complete recording coverage: gaps are never skipped silently. Source recordings are chosen by their file names, as described under [Recording storage, retention and disk usage](#recording-storage-retention-and-disk-usage). A Copy export that has to cut inside a single recording file longer than about two hours is refused; choose Accurate instead. See [export rendering](docs/EXPORT_RENDERING.md) for supported media and timing limitations.

## Home Assistant events

Create an event token with the same Compose service and recordings directory:

```sh
docker compose run --rm simplenvr auth rotate-event-token
```

Store the token in Home Assistant secrets. Send it as `Authorization: Bearer TOKEN` in a `POST` to `/api/events/trigger`. The token does not grant dashboard or recording access. Use HTTPS with a certificate Home Assistant trusts. If Home Assistant runs on another host, the localhost-only example above is not reachable from it; use a trusted reverse proxy or a restricted network listener.

## Before using real recordings

Use a dedicated, trusted local filesystem for `/recordings`. Verify the intended mount before startup, monitor capacity, and stop the container and verify it exited before changing the mount. Interrupted events can leave staging files; SimpleNVR does not automatically recover or expire them. Test your cameras and storage before relying on the system.

[License](LICENSE.md) | [Disclaimer](DISCLAIMER.md)
