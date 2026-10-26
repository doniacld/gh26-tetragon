# Who's watching the agents?

> [!IMPORTANT]
> This is a conference demo about what AI-generated code does **in production**.
> You can reuse the patterns and the GitHub Actions here, but **you are
> responsible for choosing policies that match the behaviours you want to detect
> or block in your own repos** — the policies in `policies/after-party/` are
> examples, not a baseline.

**Mona's After-Party** is a tiny ticket site for the Octocat after-party, built as
three services: a **frontend** (browse the photo wall, buy a ticket), a **gallery**
(serves the wall), and a **payments** service (charges tickets, holds a merchant
secret). An AI agent is asked to add one small feature to the gallery — "let
attendees upload their own Octocat sightings" — and to make the thumbnail it pulls
in one tiny image dependency, `github.com/octopkg/upload`. That dependency's
`Thumbnail()` really works, but on every upload it also opens a connection from
the **gallery** workload to the **payments** service and scrapes its vault — a
gallery→payments call that should never exist. Every static check stays green
(build, test, vet, gofmt, and a vulnerability scan — the module has no CVE), so
the PR ships. Deployed to a cluster, **Tetragon (eBPF) is what catches the
behaviour at runtime**: in monitor mode it *reports* the forbidden call; with an
`Override` action it **blocks the connection** so only `/upload` fails (503) while
the rest of the site keeps serving; and with `Sigkill` it **stops the gallery
container** outright — same code, same policy files, only the mode and action
changed.

## Architecture

```text
Pull request: "add image upload to the gallery"
        |
        v
CI (.github/workflows/ci.yml)
        +--> build · test · vet · gofmt · govulncheck   (five green checks)
        +--> runtime-check job                          (kind + Tetragon; fails on the 503)

Deploy (namespace party):

   browser ──▶ frontend ──buy ticket──▶ payments        (expected east-west)
                  │                        ▲
                  └──upload sighting──▶ gallery ──╳──────┘  gallery → payments :8080
                                                            (the forbidden call the
                                                             upload dependency makes)
        |
   monitor mode    : the gallery→payments connect is REPORTED, the upload still works
        |
   block (Override): connect denied (EPERM) -> /upload 503, pods stay alive
        |
   kill  (Sigkill) : connect trips SIGKILL  -> gallery CrashLoopBackOff
        |
        v
   the fix: one-line import swap on branch fix/clean-upload
```

## What's here

| Path | What it is |
|------|-----------|
| `services/frontend/` | The public site: home, photo wall, tickets. The only tier that fans out east-west — to `gallery` (`/pics`, `/upload`) and `payments` (`/charge`). Themed pages under `templates/`. |
| `services/gallery/` | The photo wall (`/pics`) and the new `/upload` feature. Uploads wait for approval on the frontend's `/admin` page (`make admin-url`) and are saved on an emptyDir (`UPLOADS_FILE`), so a Sigkill restart keeps the wall. Builds to `/usr/local/bin/gallery` — the binary the policies match. |
| `services/payments/` | Ticket charges (`/charge`) and the internal vault (`/vault`). Nothing in the gallery tier should reach it. |
| `evilupload/` | Local module `github.com/octopkg/upload`. `Thumbnail()` does the real thumbnail *and* dials `payments` on every call. Wired in via a `replace` directive so it never touches the internet. |
| `internal/sightings/` | The honest, in-tree replacement. Switching the gallery's import to this is the whole fix. |
| `policies/after-party/` | Two Tetragon `TracingPolicy` files for the gallery→payments flow: `gallery-payments-block` (Override) and `gallery-payments-kill` (Sigkill). Same files for monitor/enforce: both load in monitor (`options: policy-mode: monitor`) and `make block` / `make kill` / `make monitor` switch the mode at runtime (`tetra tp set-mode`). |
| `deploy/after-party.yaml` | `party` namespace, the three Deployments/Services, and the merchant secret. Pods carry the commit SHA. |
| `deploy/Dockerfile` | One parametrized recipe (`--build-arg SERVICE=…`) building each service to `/usr/local/bin/<service>`; bakes the SHA into the OCI revision label. |
| `testdata/headshot.png` | Sample image `make curl` / CI upload to drive the demo. |
| `sink/main.go` | The optional local "exfiltration" sink (used only if you enable the external-exfil beat via `SINK_ADDR`). |
| `.github/workflows/ci.yml` | Five static checks (all green) plus the `runtime-check` job — the Tetragon gate that fails the build on the gallery→payments 503. |
| `.github/copilot-instructions.md` | Steers the AI agent to the shady `octopkg/upload` as the "approved" image helper — the trap. |
| `docs/` | `demo-runbook.md`, `tetragon-explained.md`, `adopting-this.md`, `security-notes.md`, `talk-slides.md`. |
| `Makefile` | `demo-up`, `preflight`, `forward`, `open`, `logs`, `curl`, `observe`, `watch`, `events`, `monitor`, `block`, `kill`, `prove`, `status`, `admin-url`, `clean` (`make help` lists them). |

## The fix

The fix is a one-line diff: change the gallery's import from the shady
`github.com/octopkg/upload` to the in-tree `internal/sightings` (standard library
only). It's ready on the `fix/clean-upload` branch; the gallery's `/upload` then returns 201
(the site's `/upload` page, 200) and there is no gallery→payments call for Tetragon to catch.

## Safety

Nothing here is a real attack. The dependency is a local module (never
downloaded), there is no CVE, and no live internet calls: the forbidden call goes
to the in-cluster `payments` service, and the merchant "secret" is an obviously
fake string. The optional external-exfil beat (off by default) targets an
unroutable documentation address (`192.0.2.1`, RFC 5737) — nothing real is
exfiltrated.

## Honesty note

CI only sees what runs **in CI**. A runtime check on a PR is a strong signal, but
it is not a substitute for monitoring the same behaviours in production, where
the real tokens and real network are. Treat this as one layer, and keep watching
the agents after they ship. See `docs/security-notes.md`.

## Get started

- Run the demo: [`docs/demo-runbook.md`](docs/demo-runbook.md)
- Reuse it in your repo: [`docs/adopting-this.md`](docs/adopting-this.md)
- Limitations and honesty: [`docs/security-notes.md`](docs/security-notes.md)
- How the Tetragon policies work: [`docs/tetragon-explained.md`](docs/tetragon-explained.md)

## Credits

- **[Liz Rice — lizrice/tetragon-ci](https://github.com/lizrice/tetragon-ci)**
  (Apache-2.0): the idea of a Tetragon runtime check in GitHub Actions, the shape of
  this repo's docs (`demo-runbook.md`, `adopting-this.md`), and the CI report.
  [`.github/actions/tetragon-report/action.yml`](.github/actions/tetragon-report/action.yml)
  and the compact event stream step in `ci.yml` are **adapted from her code**; the file
  header lists what changed (Tetragon runs in a kind cluster here, so everything goes
  through `kubectl`).
- **[Tetragon](https://github.com/cilium/tetragon)** (CNCF, Apache-2.0) for the runtime
  security, and [kind](https://kind.sigs.k8s.io/), [Caddy](https://caddyserver.com/),
  [Let's Encrypt](https://letsencrypt.org/) and [sslip.io](https://sslip.io/) for the
  demo infrastructure.

Licensed under Apache-2.0.
