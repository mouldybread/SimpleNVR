# Browser first-run setup

Start SimpleNVR normally with a fresh recordings volume. Initializing credentials on the CLI is optional. When the credential file is absent, open `https://HOST:PORT/auth/setup` (or open the root page, which redirects there). Accept the self-signed certificate only for your own trusted test installation.

Read the one-time setup code from `docker logs CONTAINER` or `docker compose logs simplenvr`. Enter that code, a username, and a password twice. Passwords must have at least 15 characters, at most 1024 UTF-8 bytes, and no line breaks. Treat the logs as sensitive. The code is random per process start; restarting before completion invalidates the previous code. There is no automatic setup-code expiry within one process.

During setup no camera loading, recording, recovery, event handling, or dashboard/media access runs. Setup attempts are serialized and rate-limited globally to one per second. All traffic uses HTTPS and setup POSTs have cross-origin protection.

Once credentials are created, the setup listener drains and normal startup continues on the same port, without needing a Docker restart policy. Use the completion page's sign-in link; there may be a short startup delay. Setup does not automatically sign you in. After normal startup the setup route cannot create or replace credentials. Existing malformed or unreadable credentials fail normal authentication startup instead of reopening setup.

Credentials are Argon2id hashed with the same parameters as CLI initialization and published with mode 0600 without replacing an existing file. The parent directory is created with mode 0700 when absent. Initial publication requires hard-link support on the credentials filesystem; unsupported storage fails rather than falling back to an overwriting write.

The existing `auth set-password USERNAME` command remains available for scripted initialization and password resets. Deleting the credential file enables first-run setup on the next process start; protect the recordings volume accordingly.

Verification: the new tests have not been compiled or run by the assistant. Real-browser setup, normal-startup handover, and concurrent initialization still require CI/runtime verification.
