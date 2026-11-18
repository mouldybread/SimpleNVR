# SimpleNVR

SimpleNVR records RTSP cameras with FFmpeg. Use the web interface to manage cameras, play recordings, review events, and export MP4 clips.

## Install with Docker Compose

Use the published image from GitHub Container Registry. This example requires an image release with built-in authentication. The `v0.1.0` release predates that feature. Until a compatible tag is published, this example is not runnable as written.

Save as `compose.yaml`:

```yaml
services:
  simplenvr:
    image: ghcr.io/mouldybread/simplenvr:${SIMPLENVR_VERSION:?Set SIMPLENVR_VERSION in .env}
    restart: unless-stopped
    ports:
      - "127.0.0.1:8443:443"
    volumes:
      - type: bind
        source: ./recordings
        target: /recordings
        bind:
          create_host_path: false
      - ./certs:/certs
```

Set `SIMPLENVR_VERSION` in `.env` to a published tag that includes built-in authentication. Then run:

```sh
mkdir -p recordings certs
docker compose pull &&
docker compose run --rm simplenvr auth set-password admin &&
docker compose up -d
```

Use a password of at least 15 characters. On the Docker host, open `https://localhost:8443/auth/login` and sign in as `admin`. The default self-signed certificate triggers a browser warning. Set the password once; skip that step on later starts. The example listens only on localhost. For a first test, use a fresh recordings directory, not production storage.

## Record and play back

- In Camera Settings, add an RTSP URL or import streams from go2rtc. Choose continuous recording, event-based recording with a prebuffer, or no recording. Adding a camera does not start recording.
- Live preview requires go2rtc on the host in the camera's RTSP URL, with its HTTP API on port `1984`. The camera name must be unique and match the go2rtc stream name. Direct-camera preview and custom go2rtc HTTP ports are not supported.
- Use the timeline to find recordings and events. Playback does not automatically skip gaps. Use Downloads to select cameras and a time range for MP4 export.
- Removing a camera can delete its published recordings. If removal fails or the result is unclear, refresh and check the camera and its recordings before retrying.

## Home Assistant events

Create an event token with the same Compose service and recordings directory:

```sh
docker compose run --rm simplenvr auth rotate-event-token
```

Store the token in Home Assistant secrets. Send it as `Authorization: Bearer TOKEN` in a `POST` to `/api/events/trigger`. The token does not grant dashboard or recording access. Use HTTPS with a certificate Home Assistant trusts. If Home Assistant runs on another host, the localhost-only example above is not reachable from it; use a trusted reverse proxy or a restricted network listener.

## Before using real recordings

Use a dedicated, trusted local filesystem for `/recordings`. Verify the intended mount before startup, monitor capacity, and stop the container and verify it exited before changing the mount. Interrupted events can leave staging files; SimpleNVR does not automatically recover or expire them. Test your cameras and storage before relying on the system.

[License](LICENSE.md) | [Disclaimer](DISCLAIMER.md)
