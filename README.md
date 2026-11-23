# SimpleNVR

SimpleNVR records RTSP cameras with FFmpeg. Use the web interface to manage cameras, play recordings, review events, and export MP4 clips.

## Install with Docker Compose

Use the image from GitHub Container Registry. The example defaults to `v0.2.2`, which includes built-in authentication and server-prepared exports. An image's release workflow must finish publishing before it can be pulled.

Save as `compose.yaml`:

```yaml
services:
  simplenvr:
    image: ghcr.io/mouldybread/simplenvr:${SIMPLENVR_VERSION:-v0.2.2}
    restart: unless-stopped
    ports:
      - "127.0.0.1:8443:443"
    environment:
      NVR_EXPORT_SCRATCH_DIR: /exports
    volumes:
      - type: bind
        source: ./recordings
        target: /recordings
        bind:
          create_host_path: false
      - type: bind
        source: ./exports
        target: /exports
        bind:
          create_host_path: false
      - ./certs:/certs
```

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

## Session timeouts

A session ends after 20 minutes without a request, or 8 hours after sign-in, whichever comes first. To change either limit, click the lock button in the dashboard header, enter the new values in minutes, and choose Save and restart (click twice to confirm). SimpleNVR saves the values to `settings.json` in the same directory as the credentials file (`/recordings/.auth/settings.json` by default), shuts down gracefully, and relies on the container's restart policy to start it again. Keep `restart: unless-stopped` (or `always`) in your Compose file: with `on-failure` or no policy the container stays stopped until you start it. The restart stops live views and exports, pauses recording until SimpleNVR is back, and ends all current sessions. Nothing restarts if the saved values already match the running ones.

The idle timeout must be between 1 minute and 24 hours, and the maximum session length between 5 minutes and 30 days. The idle timeout cannot exceed the maximum session length. If `settings.json` is damaged or out of range, SimpleNVR refuses to start until you fix or delete it.

## Record and play back

- In Camera Settings, add an RTSP URL or import streams from go2rtc. Choose continuous recording, event-based recording with a prebuffer, or no recording. Adding a camera does not start recording.
- Live preview requires go2rtc on the host in the camera's RTSP URL, with its HTTP API on port `1984`. The camera name must be unique and match the go2rtc stream name. Direct-camera preview and custom go2rtc HTTP ports are not supported.
- Use the timeline to find recordings and events. Playback does not automatically skip gaps. Use Downloads to export video; see below.
- Known continuous-recording failures appear as "Recording failed" or "Recording failed — retrying", with the recording setting and reason shown separately. "Recording enabled" is a setting, not proof that footage is being captured.
- Removing a camera can delete its published recordings. If removal fails or the result is unclear, refresh and check the camera and its recordings before retrying.

## Export video

In Downloads, choose cameras, adjust the start and end times, and choose a mode:

- Copy (default) preserves the encoded media without re-encoding. A single complete recording is copied byte for byte; compatible complete recordings can be joined without re-encoding. For part of a recording, SimpleNVR extends the range outward to the nearest safe cut (at most 30 seconds at each end) without asking, and shows the saved range once the job is created. Audio packet boundaries can reach slightly past the video range. If no safe cut is available within 30 seconds, or the video cannot be copied safely, the export is refused and you can choose Accurate instead. An unsupported copy is refused, never silently converted to Accurate.
- Accurate re-encodes supported video and audio to cut within frame and packet limits at the selected times. It is not lossless and can require more processing.

Choose Prepare exports on server. SimpleNVR prepares one camera's file at a time. When ready, choose Download prepared video. Once your browser has accepted the download, release the prepared file to continue to the next camera; an already-started transfer may continue. The page cannot verify that your browser saved the file to disk.

Prepared files expire, normally 30 minutes after preparation finishes, and do not survive a restart. Exports require complete recording coverage: gaps are never skipped silently. See [export rendering](docs/EXPORT_RENDERING.md) for supported media and timing limitations.

## Home Assistant events

Create an event token with the same Compose service and recordings directory:

```sh
docker compose run --rm simplenvr auth rotate-event-token
```

Store the token in Home Assistant secrets. Send it as `Authorization: Bearer TOKEN` in a `POST` to `/api/events/trigger`. The token does not grant dashboard or recording access. Use HTTPS with a certificate Home Assistant trusts. If Home Assistant runs on another host, the localhost-only example above is not reachable from it; use a trusted reverse proxy or a restricted network listener.

## Before using real recordings

Use a dedicated, trusted local filesystem for `/recordings`. Verify the intended mount before startup, monitor capacity, and stop the container and verify it exited before changing the mount. Interrupted events can leave staging files; SimpleNVR does not automatically recover or expire them. Test your cameras and storage before relying on the system.

[License](LICENSE.md) | [Disclaimer](DISCLAIMER.md)
