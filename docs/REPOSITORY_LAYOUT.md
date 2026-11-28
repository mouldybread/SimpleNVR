# Repository layout (issue #16)

Status: complete. This page describes the final layout and the move inventory.

## Layout

- `cmd/simplenvr/` - all Go sources and Go tests (`package main`, unchanged), plus `testmain_test.go`
- `web/templates/`, `web/static/` - HTML templates and static assets
- `tests/frontend/` - Node (`*.cjs`) frontend suites
- `tests/acceptance/` - Python helpers and their unit tests, and the shell acceptance scripts
- `tests/fixtures/`, `tests/evidence/` - unchanged
- `docs/` - documentation; `docs/compose/compose.yaml` holds the Compose example
- `.github/` - workflows and scripts (unchanged location)
- Root: `go.mod`, `go.sum`, `Dockerfile`, `Dockerfile.dev`, `.air.toml`, `README.md`, `LICENSE.md`, `DISCLAIMER.md`, `.gitignore`

Decisions: all Go files move to `cmd/simplenvr/`; `Dockerfile` stays at the root so `docker build .` and the publish workflow work unchanged; `LICENSE.md` and `DISCLAIMER.md` stay at the root; Compose examples live in `docs/`. `Dockerfile.dev` and `.air.toml` also stay at the root (they were proposed for `build/` only alongside the Dockerfile). The test scripts are grouped into `tests/frontend/` and `tests/acceptance/` rather than the originally proposed `runtime/` and `ci/` split.

The package is not split into `internal/` packages: about 300 files share unexported symbols, so that would be a behavior refactor, not a move.

## Old to new

| Old | New |
|---|---|
| `*.go`, `*_test.go` (root) | `cmd/simplenvr/` |
| `templates/` | `web/templates/` |
| `static/` | `web/static/` |
| `tests/*.cjs` | `tests/frontend/` |
| `tests/*.py`, `tests/*.sh` | `tests/acceptance/` |
| `tests/fixtures`, `tests/evidence` | unchanged |
| Compose example in `README.md` | `docs/compose/compose.yaml` |

Files were moved with `git mv`, so history follows them (`git log --follow`).

## Runtime paths

- The binary still loads `web/templates/*.html` and serves `web/static` relative to the working directory, so run it from the repository root: `go run ./cmd/simplenvr`.
- In the container image the layout is `/app/web/templates` and `/app/web/static` with `WORKDIR /app`.
- `/recordings`, `/certs` and the `/static/` URL route are unchanged.

## Development

- Build: `go build ./cmd/simplenvr`
- Test: `go vet ./... && go test -race ./...` (tests run from the repository root via `cmd/simplenvr/testmain_test.go`)
- Python acceptance unit tests: `python3 -m unittest discover -s tests/acceptance`
- Docker: `docker build .`

CI triggers, job names, required checks and test assertions are unchanged. See `docs/CHECKPOINT_CI.md` for the `run-checkpoint-ci` label flow; every new head needs its own run.
