# Repository layout (issue #16)

Status: migration in progress. This page is updated to the final tree and move
inventory in the last batch.

## Target layout

- `cmd/simplenvr/` - all Go sources and Go tests (`package main`, unchanged)
- `web/templates/`, `web/static/` - HTML templates and static assets
- `tests/frontend/`, `tests/runtime/`, `tests/ci/` - Node suites, runtime acceptance scripts, CI helpers and their unit tests
- `tests/fixtures/`, `tests/evidence/` - unchanged
- `docs/` - documentation, including `docs/compose/` for Compose examples
- `.github/` - workflows and scripts (stay in place)
- Root: `go.mod`, `go.sum`, `Dockerfile`, `README.md`, `LICENSE.md`, `DISCLAIMER.md`

Application behavior, runtime paths (`/recordings`, `/certs`, `/app/templates`,
`/app/static`) and test assertions do not change.
