# SimpleNVR

SimpleNVR records RTSP cameras with FFmpeg. Use the web interface to manage cameras, play recordings, review events, and export MP4 clips.

## Try it with Docker

From this checkout, build the image, set an admin password, and start a local test instance:

```sh
docker build -t simplenvr:local . &&
docker run --rm -it -v simplenvr-local-test:/recordings simplenvr:local auth set-password admin &&
docker run --rm --name simplenvr-local -p 127.0.0.1:8443:443 -v simplenvr-local-test:/recordings -v simplenvr-local-certs:/certs simplenvr:local
```

Use a password of at least 15 characters. Open `https://localhost:8443/auth/login` on the Docker host and sign in as `admin`. The default self-signed certificate triggers a browser warning. The named volumes keep your test recordings, settings, login, and certificate between runs. Once the password is set, skip that step on later starts. Do not use your production recordings for this test.

## Record and play back

- In Camera Settings, add an RTSP URL or import streams from go2rtc. Choose continuous recording, event-based recording with a prebuffer, or no recording. Adding a camera does not start recording.
- Live preview requires go2rtc on the host in the camera's RTSP URL, with its HTTP API on port `1984`. The camera name must be unique and match the go2rtc stream name. Direct-camera preview and custom go2rtc HTTP ports are not supported.
- Use the timeline to find recordings and events. Playback does not automatically skip gaps. Use Downloads to select cameras and a time range for MP4 export.
- Removing a camera can delete its published recordings. If removal fails or the result is unclear, refresh and check the camera and its recordings before retrying.

## Home Assistant events

Create an event token with the same image and recordings volume used by the server. For the test instance above:

```sh
docker run --rm -v simplenvr-local-test:/recordings simplenvr:local auth rotate-event-token
```

Store the token in Home Assistant secrets. Send it as `Authorization: Bearer TOKEN` in a `POST` to `/api/events/trigger`. The token does not grant dashboard or recording access. Use HTTPS with a certificate Home Assistant trusts.

## Before using real recordings

Use a dedicated, trusted local filesystem for `/recordings`. Verify the intended mount before startup, monitor capacity, and stop the container before changing the mount. Interrupted events can leave staging files; SimpleNVR does not automatically recover or expire them. Test your cameras and storage before relying on the system.

[License](LICENSE.md) | [Disclaimer](DISCLAIMER.md)
