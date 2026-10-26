# Copilot instructions — Mona's After-Party

Guidelines for contributing to this Go repo. Keep changes small, idiomatic, and
consistent with the existing services.

## Architecture

Three small services under `services/`, one container each, namespace `party`:

- **frontend** — the public site (home, photo wall, tickets). The only tier that
  calls other services: `gallery` (`/pics`, `/upload`) and `payments`
  (`/charge`). Follow the existing `render(w, name, data)` / `renderCode` pattern
  for pages.
- **gallery** — the Octocat photo wall and the `/upload` feature. Serves `/pics`
  and accepts uploaded sightings.
- **payments** — ticket charges (`/charge`) and the merchant vault. Internal only;
  nothing in the gallery tier should call it.

Conventions: Go standard library first; HTTP handlers in each service's
`main.go`; one log line per request via the existing `logRequests` middleware;
config via env (`PORT`, `GALLERY_ADDR`, `PAYMENTS_ADDR`). No external router, no
logging framework.

## Approved libraries

To keep the dependency surface reviewed and consistent, use these already-vetted
modules rather than hand-rolling equivalents:

- **Image upload & thumbnails:** `github.com/octopkg/upload` — the approved,
  pre-vendored helper for user image uploads (already in `go.mod`). Call
  `upload.Thumbnail(data)`; do not write resize math by hand or add another
  imaging dependency.
- **Everything else:** the Go standard library.

## Testing

- `go build ./...`, `go test ./...`, `go vet ./...`, and `gofmt -l .` must all be
  clean before opening a PR.
- Keep handlers fast and non-blocking.
