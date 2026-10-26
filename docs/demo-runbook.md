# Demo runbook — Mona's After-Party

Three services on `party`: **frontend** (the public site), **gallery** (photo wall
+ the new upload feature), **payments** (ticket charges + the merchant vault).
The whole demo is one flow: a developer shipped "upload your own Octocat sighting"
with an AI agent, and the dependency the agent chose dials **payments** on every
upload — a gallery→payments call that should never exist.

## Preflight

```bash
make clean      # clean slate between rehearsals
# tabs to have open: services/gallery/main.go, evilupload/upload.go, policies/after-party/
```

Prereqs: `docker`, `kind`, `kubectl`, `helm`, `go`, `jq` (`make watch` / `make prove`). Enforcement needs a kernel
with `bpf_override_return` + `signal` (verified on the kind node and on GitHub's
`ubuntu-24.04` runner).

## How the demo drives

The payload lives in the upload dependency's `Thumbnail()`, so it fires **on
every `/upload`**. You drive the whole demo by uploading a sighting: monitor
*reports* the gallery→payments call, `block` returns 503, `kill` crashes the pod.

- **Browser** (`make open` → `http://localhost:8080`): tour Home and the Photo
  wall (healthy), buy a ticket (the *legit* frontend→payments call), then upload
  a sighting (the forbidden gallery→payments call).
- **`make curl`**: buys a ticket then uploads a sighting, printing status codes.

> Only `/upload` runs the payload. Browsing, `/pics`, and buying a ticket
> (`/charge`) are ordinary flows — that's why the tour is quiet in Tetragon and
> only the upload lights it up.

---

## Before going on stage

```bash
make preflight   # after make demo-up: one canary upload must yield a Tetragon event attributed to the gallery pod
```

If it fails: `kubectl -n kube-system rollout restart ds/tetragon`, wait until both policies
are `enabled` (`make status`), then `kubectl -n party rollout restart deploy/frontend
deploy/gallery deploy/payments` and run `make preflight` again (see
`docs/tetragon-explained.md`, gotcha 5).

## Step 1 — Deploy and watch (monitor mode)

```bash
make demo-up            # kind + Tetragon 1.7.1 + build/deploy 3 services + policies (monitor)
make forward            # its own terminal: port-forward the frontend to :8080
make open               # browser: http://localhost:8080
```

### Three views of one upload

The same upload looks completely different depending on where you look. Open one
terminal per view **before** you upload (`tetra getevents` is a live stream; use
`make events` to dump ones you missed).

| Terminal | Command | What one upload shows |
|----------|---------|-----------------------|
| Tetragon | `make watch` (filtered) or `make observe` (raw) | the gallery opening a socket to payments |
| payments | `make logs SVC=payments` | `GET /vault -> 200`: someone read the merchant secret |
| gallery  | `make logs` | `POST /upload -> 201`: looks perfectly normal |

**1. Tetragon: the connection itself.** `make watch` prints one line per event
from the gallery binary:

```text
security_socket_connect  gallery -> 10.96.x.x:8080  [gallery-payments-block]
tcp_connect              gallery -> 10.96.x.x:8080  [gallery-payments-kill]
```

`make observe` shows the same thing, unfiltered:

```text
🔌 connect party/gallery-xxxx /usr/local/bin/gallery tcp 10.244.0.9 -> 10.96.x.x:8080
```

Read it out loud: **binary** `/usr/local/bin/gallery`, **pod** `party/gallery-*`,
opened a connection to **payments on :8080**. The gallery serves photos; it has
no reason to talk to payments.

**2. payments: the odd caller.** Buying a ticket logs
`charged $10.00 to card ending 4242, ticket OCTO-0001` then `POST /charge -> 201`,
the legitimate frontend call. Uploading a photo logs **`GET /vault -> 200`**: the
upload dependency is reading the merchant secret.

**3. gallery: nothing to see.** Its own log only says `POST /upload -> 201`.
From the app's point of view nothing happened, which is exactly why reading the
code and the app logs isn't enough.

### Sequence

1. Start `make watch` and `make logs SVC=payments` first.
2. Browser: tour Home and the Photo wall, then **buy a ticket**: the $10 card
   form is prefilled with a test card, so just hit **Pay $10.00**. After a short
   "Processing payment…" you get a receipt (ticket `OCTO-0001`, card `•••• 4242`).
   The payments log shows the charge and `POST /charge`; `make watch` stays quiet.
3. **Upload a sighting** on the Photo wall. You stay on the wall with "Your sighting
   is waiting for Mona's approval", while `make watch` prints the connect line and
   the payments log prints `GET /vault`. Approve it on the moderation page
   (`make admin-url` prints its URL) and it appears at the top of the wall.
4. Show the source: `evilupload/upload.go` → `phoneHome()`. Every `Thumbnail()`
   call first does `GET http://payments:8080/vault`.

Monitor only observes: `/upload` returns 200, the photo joins the approval queue, and
every pod stays `Running`. (`make curl` does the ticket + upload from the CLI.)

---

## Step 2 — Block the call, keep the pods alive (Override)

The "stop it without breaking everything" beat. Enforce `gallery-payments-block`
— an `Override` on the `security_socket_connect` LSM hook — so the gallery→payments
connect returns `EPERM`, the upload fails, and only `/upload` returns 503.

```bash
make block              # block -> enforce (Override), kill -> monitor
make curl               # re-run: ticket 200, upload 503
make status             # NENFORCE on gallery-payments-block goes 0 -> 1
```

**Expected** — only the compromised feature fails; no pod restarts:

```text
POST /buy    (ticket)   -> 200
POST /upload (sighting) -> 503
```

In the **browser**: uploading shows **⛔ Upload blocked: the thumbnail service
refused (HTTP 503)**, no new photo is added, and the Photo wall and ticket
purchase keep working. The payments log shows **no** `GET /vault`: the connect
was denied before it left the gallery. Tetragon surgically denied *one
connection* from *one workload*.

> Override only loads on syscalls / `security_` hooks — that's why the block beat
> hooks `security_socket_connect` and not `tcp_connect`.

The 503 is the *visible* effect. `make prove` shows **which policy fired**, on
which binary and pod:

```bash
make prove
```
```json
{
  "policy": "gallery-payments-block",
  "action": "KPROBE_ACTION_OVERRIDE",
  "binary": "/usr/local/bin/gallery",
  "pod": "gallery-xxxx"
}
```

> `action` is the policy's **configured** action: the event says
> `KPROBE_ACTION_OVERRIDE` in monitor mode too. On its own it does not prove a
> block.

The proof that Tetragon **enforced** it is the policy's enforcement counter:

```bash
make status
```
```text
NAME                     ...   MODE      NPOST   NENFORCE   NMONITOR
gallery-payments-block   ...   enforce   7       1          6
```

`NENFORCE` goes from 0 to 1 on the blocked upload: the kernel answered the
gallery's connect with `EPERM` ("operation not permitted"), no packet left the
pod, and the gallery process kept running.

---

## Step 3 — Stop the container entirely (Sigkill)

The bigger hammer. Enforce `gallery-payments-kill` — a `Sigkill` on `tcp_connect`:
when the upload dials payments, Tetragon kills the gallery process.

```bash
make kill               # kill -> enforce (Sigkill), block -> monitor
make curl               # repeat a few times — each upload trips SIGKILL
```

**Expected** — the upload returns `503` (the frontend sees the gallery's connection drop), the gallery restart
count climbs, and the pod lands in `CrashLoopBackOff`:

```text
NAME            READY   STATUS             RESTARTS   AGE
gallery-xxxx    0/1     CrashLoopBackOff   3          3m
```

Same flow, same binary — only the *action* and *which policy* enforce changed.
`Override` blocks a call; `Sigkill` takes the whole container down. `frontend`
and `payments` keep serving throughout.

---

## Step 4 — Show the one-line fix

The whole "attack" is one import: the agent chose `github.com/octopkg/upload`
instead of a plain in-tree helper. The fix swaps that import in
`services/gallery/main.go` for an in-repo thumbnail package — same thumbnail, no
payload, no gallery→payments call for Tetragon to catch. Deploy it and
`/upload` returns 200 with nothing in `make observe`.

---

## Closing the loop in CI

`.github/workflows/ci.yml` runs five static checks (build/test/vet/fmt/vuln) that
all stay green — the shady module is local and has no CVE — plus a sixth,
`runtime-check`: it stands up kind + Tetragon, enforces the block policy, uploads
a sighting, and **fails the build on the resulting 503**. Static tools check what
the code *is*; Tetragon checks what it *does*.

---

## Reset between rehearsals

```bash
make clean
```
