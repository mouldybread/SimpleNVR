# SimpleNVR

SimpleNVR is a lightweight, self-hosted network video recorder. It uses FFmpeg
to record RTSP streams and provides a browser interface for camera management,
timeline playback, event review, and MP4 exports. It is designed to work with
go2rtc and can receive events from Home Assistant.

## Quick start: local Docker test

The commands below build from this checkout and use a separate test volume.
They do not mount an existing recordings directory or publish an image.

```sh
docker build -t simplenvr:local .
docker run --rm -it \
  --mount type=volume,src=simplenvr-local-test,dst=/recordings \
  simplenvr:local auth set-password admin
docker run --rm --name simplenvr-local \
  -p 127.0.0.1:8443:443 \
  --mount type=volume,src=simplenvr-local-test,dst=/recordings \
  -v /certs \
  simplenvr:local
```

Open `https://localhost:8443/auth/login` on the machine running Docker and sign
in as `admin`. On first boot, SimpleNVR generates a self-signed HTTPS certificate
if none is supplied, so expect a browser warning. The password prompt does not
echo input. Use at least 15 characters. There are no default credentials or
unauthenticated setup page. The server and the password setup command must use
the same recordings volume.

The named test volume retains configuration, credentials, and test recordings
between runs. The anonymous certificate volume in this example is disposable.
Do not attach your existing recordings to this test instance.

## Recording, playback, and export

- Add cameras manually by RTSP URL or scan a go2rtc instance from Camera Settings.
- Configure continuous recording, event-based recording with a prebuffer, or no
  recording. Camera registration and recorder start are separate operations.
- Browse available recordings by camera and date, seek on the timeline, and
  review event markers. Do not assume gapless playback or synchronized
  advancement across cameras. See [timeline behavior](docs/audit/TIMELINE_API.md).
- Select cameras and a time range in Downloads to prepare monitored MP4 exports.
  See [export behavior](docs/audit/EXPORT_UI.md) for the supported controls and
  cut semantics.
- Live preview supports the documented go2rtc topology: the camera's RTSP URL
  points to the go2rtc host, its HTTP API is on port `1984`, and the SimpleNVR
  camera name matches the go2rtc stream name. Direct-camera preview and a custom
  go2rtc API port are not supported by this proxy. See
  [live-proxy boundaries](docs/LIVE_PROXY_BOUNDARIES.md).

Camera removal can delete published recordings. It can also refuse if retained
evidence or an unexpected directory entry is present. A failed or uncertain
response does not guarantee that nothing changed: refresh and review the camera
and recordings before retrying. See
[camera deletion safety](docs/audit/CAMERA_DELETION_SAFETY.md).

## Authentication and Home Assistant

Browser access requires a session. The `/auth/login` page is the entry point;
the dashboard and recording files are not public endpoints. Keep HTTPS and
appropriate network access controls in place. If you reverse proxy SimpleNVR,
preserve the browser-facing Host header.

For Home Assistant, create an event-only token using the same recordings volume
as your server. For the local example above:

```sh
docker run --rm \
  --mount type=volume,src=simplenvr-local-test,dst=/recordings \
  simplenvr:local auth rotate-event-token
```

Store the token in Home Assistant secrets. Send `Authorization: Bearer TOKEN`
with a `POST` to `/api/events/trigger`. The token is only for that endpoint; it
does not grant access to the dashboard or recordings. Use a certificate your Home
Assistant instance trusts. See the [authentication guide](docs/AUTHENTICATION.md)
for password resets, token rotation, and credential storage.

## Storage and deployment

`/recordings` holds application state and media, including camera configuration
and login credentials. `/certs` holds TLS material. Back up configuration and
credentials securely, and keep test and production storage separate.

For intended deployment, use a dedicated, trusted local recordings filesystem.
Verify the mount before container startup, monitor free space and retained
staging, and stop the container and verify it exited before storage maintenance.
A free-space check is not a reservation or proof that the intended mount is
present. The runtime requires compatible Linux procfs and no-replace rename
behavior. See [storage deployment boundaries](docs/STORAGE_DEPLOYMENT_BOUNDARIES.md)
for the supported profile and scratch-filesystem check.

Interrupted events can leave staging files for inspection. SimpleNVR does not
automatically reassemble them or expire staging. A file that exists or plays
is not proof that all requested footage was captured. Synthetic CI and a local
smoke test do not qualify live cameras or deployment storage.

## Development and releases

The Docker build includes the Go application, frontend assets, and FFmpeg.
Go unit and integration tests live alongside the packages they exercise;
browser and runtime fixture scripts live in `tests/`. See the
[Safety verification workflow](.github/workflows/safety-ci.yml) for the CI
checks and their dependencies.

A push of a `v*` tag triggers validation of the tagged commit before the
[Docker publish workflow](.github/workflows/docker-publish.yml) can publish an
image to GitHub Container Registry. A local Docker build does not publish.

## Project records

- [Recording-safety audit and historical evidence](docs/audit/README.md)
- [Disclaimer and limitations](DISCLAIMER.md)
- [License](LICENSE.md)
