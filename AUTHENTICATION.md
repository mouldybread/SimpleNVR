# Built-in authentication (fix branch)

This is an intentional breaking deployment change. There are no default credentials and no unauthenticated setup page. Do not deploy until CI and the remaining recording/security audit fixes are complete.

## Bootstrap

Build the fix-branch image first. Initialize credentials using a one-shot container with the same recordings dataset mounted as the server. For example, replace /YOUR/RECORDINGS with your actual dataset directory:

```sh
docker build -t simplenvr-safety .
docker run --rm -it -v /YOUR/RECORDINGS:/recordings simplenvr-safety auth set-password admin
```

The CLI prompts without echo and confirms the password. It also accepts a single password line on stdin for secret-management automation. Do not pass passwords in command arguments. Passwords must have at least 15 Unicode characters and at most 1024 bytes. Spaces are preserved. Hashing uses Argon2id with 64 MiB memory, 3 iterations, 1 lane and a random 16-byte salt.

Credentials default to /recordings/.auth/credentials.json, mode 0600. NVR_AUTH_FILE can select a different persistent path. Mount that same location for both bootstrap and server containers. Back up the credential file securely. The server refuses to start if it is missing, invalid, symlinked or accessible to group/other users.

## Browser sessions

Visit /auth/login over HTTPS. Sessions are server-side in memory, have an 8-hour absolute lifetime and 20-minute idle timeout, and disappear on restart. Login renews the session ID. The __Host-simplenvr cookie is Secure, HttpOnly, SameSite=Lax, Path=/, with no Domain.

Visit /auth/logout and submit the sign-out form to revoke the current session. Password reset uses the same CLI command. Credential-file changes invalidate previously issued sessions on their next request without a restart. Existing requests/streams are not forcibly terminated by logout or password reset in this patch; cancellation/revocation of in-flight streams remains part of the pending live-proxy/lifecycle work.

Login attempts have a shared one-attempt-per-second limit and at most two concurrent password hashes. This bounds work, but distributed attackers can consume the shared allowance; stronger network-level limits/MFA remain future hardening. No proxy identity headers are trusted.

State-changing API endpoints require POST. Go CrossOriginProtection rejects unsafe cross-origin browser requests; no cross-origin allowlist is configured. Preserve the browser-facing Host header when reverse proxying. Browser credentials are removed before invoking downstream application handlers so the existing stream proxy cannot forward session cookies or authorization headers to cameras/go2rtc.

## Home Assistant

Create/rotate the event-only token:

```sh
docker run --rm -v /YOUR/RECORDINGS:/recordings simplenvr-safety auth rotate-event-token
```

The token is printed once. Store it in Home Assistant secrets, not in Git. Only its SHA-256 digest is persisted. Send Authorization: Bearer TOKEN to POST /api/events/trigger. This token is rejected everywhere else. Rotation also invalidates browser sessions because the credential-file version changes. Existing unauthenticated integrations need updating.

## Dependency verification

Go 1.26 is required by the selected current cryptography dependency and includes standard-library CSRF protection. Library versions are pinned in go.mod. This initial branch patch bootstraps go.sum with go mod tidy in CI/builds, verified against Go's configured checksum database; checksums have not been fabricated. Review and commit the generated go.sum from CI before merging, then require readonly module builds. CI prints the generated lockfile for that follow-up. A green run alone does not complete the wider audit or establish absence of vulnerabilities.
