# Who's watching the agents? — slide deck

Generated from the PowerPoint deck by `talk/deck_to_markdown.py`: the deck is the source
of truth, edit it and re-run the script. Each slide lists its timing (start · duration),
what's on screen, and the script from the speaker notes.

## 1 · Who’s watching the agent? What AI code does in production

**Timing:** 0:00 · 0:20

**On screen:** Donia Chaiehloudj | @doniacld · Senior Technical Community Advocate · Isovalent @Cisco

> "AI agents are writing and merging code in our repos right now. The tests pass, the PR is green, it ships. My question today is simple: once that code is running in production, who's actually watching what it does?"

---

## 2 · I am a software engineer with a focus on cloud native, networking, runtime security and eBPF.

**Timing:** 0:20 · 0:15  Quick intro, keep it short: the demo is the star.

**On screen:** Donia Chaiehloudj · Sr. technical community advocate / Isovalent @cisco

---

## 3 · Mona's After-Party

**Timing:** 0:35 · 0:20

> "Here's our app: a little site for the Octocat after-party. Three services: the site, a photo wall, and a payments service to buy a ticket. Everything I show today runs on my laptop in a kind cluster."

---

## 4 · The new normal

**Timing:** 0:55 · 0:25

**TODO  screenshot: issue #6 + the Copilot Agent mode chat that implemented it**

> "This is normal now. I hand the ticket to an agent, it writes the code, I open the PR. A reviewer skims it, sees green checks, hits merge. The agent is fast and mostly right, which is exactly why we stop reading carefully."

---

## 5 · “Let attendees upload their own Octocat sightings.”

**Timing:** 1:20 · 0:15

> "Here's the task we gave the agent: let attendees upload their own Octocat sightings to the photo wall. Utterly boring. The kind of change you approve between meetings."

---

## 6 · LIVE DEMO · GRAB YOUR PHONE

**Timing:** 1:35 · 0:40  (leave the QR up while people scan)

**On screen:** 32-190-46-246.sslip.io · Join the party 📱

> "And you're the attendees. This is the real site, live, right now. Scan this and upload an Octocat sighting: anything Octocat-ish, a sticker, a hoodie, your badge. I approve every photo before it hits the wall, and the location data is stripped from your pictures. Keep it on screen: we'll come back to it."
> Backstage: admin page open on your phone (admin-url.txt). Approve a few as they come in.

---

## 7 · What the agent wrote

**Timing:** 2:15 · 0:35

**On screen:** Read the file, make a thumbnail, pin it to the wall. · import ( · services/gallery/main.go

> "And this is what it wrote. Read the file, make a thumbnail, add it to the wall. For the thumbnail it pulled in one small dependency: octopkg/upload. Our Copilot instructions even recommend it as the approved image helper. Looks great. Would you block this PR?"

---

## 8 · CI is green

**Timing:** 2:50 · 0:25

**TODO  screenshot: the 5 green static checks (build · test · vet · fmt · vuln) on the Copilot PR, PR #4 as fallback (crop out runtime-check, it comes back later)**

> "The pipeline agrees. It builds, tests pass, go vet and gofmt are happy. And govulncheck is green too: there is no CVE here. Static analysis has nothing to grab onto. So it ships."

---

## 9 · What could go wrong?

**Timing:** 3:15 · 0:15

> "Green across the board, merged, deployed. And you've been using it for the last few minutes. So what does this code actually do when it runs?"

---

## 10 · eBPF Observability and Runtime Security Agent.

**Timing:** 3:30 · 0:35

**On screen:** Tetragon: Observing runtime behaviour

> "This is Tetragon, a CNCF project built on eBPF. It runs in the Linux kernel and sees what every process does: what executes, what files get read, what connections open, and crucially which workload did it. It doesn't care what the code says. It sees what it does."

---

## 11 · Demo: watch it

**Timing:** 4:05 · 2:15 · DEMO 1 (monitor) · the audience has been uploading since slide 6

Screen: browser (wall) left · right: make watch | make logs SVC=payments | command pane
1. Wall: approve a few photos live (admin page on your phone). "Look, that's you."
2. make events | wc -l  →  "N uploads so far." Point at make watch as new uploads arrive: "security_socket_connect gallery -> …:8080".
3. Payments pane: a wall of "GET /vault -> 200". "Every single one of your uploads just read the payment service's merchant secret."
4. Buy a ticket ($10): "charged $10.00 … POST /charge -> 201", make watch quiet. "Frontend → payments: expected."
> "The gallery serves photos; it has no reason to talk to payments. Your uploads worked, the site looks fine: without this we'd never know. So where did this come from?"

---

## 12 · Tetragon events

**Timing:** 6:20 · 0:10  SKIP if the live demo worked (fallback / recap only)

**On screen:** One upload: the gallery binary dials payments:8080 and reads /vault

> "Two views of one upload. Tetragon: the gallery binary opened a connection to payments on 8080. Payments: someone called GET /vault. The gallery's own log just says POST /upload 201."

---

## 13 · There it is

**Timing:** 6:30 · 0:40

**On screen:** On every upload, the thumbnail helper dials payments and reads its vault. · func phoneHome() error { · github.com/octopkg/upload

> "Here it is, inside that little upload dependency. The thumbnail is real; that's why it passed review. But on every upload, the same function dials the payments service and reads its vault. No exploit, no CVE, in a file nobody opened. We didn't find this by reading it. We found it by watching the behaviour."

---

## 14 · Same code

**Timing:** 7:10 · 0:20

**On screen:** Developer · “I just added a photo upload.” · Security · “The gallery workload opens a socket to payments.” · Two views

> "This is the gap in one slide. The developer sees a feature that looks fine. Tetragon shows the security engineer the flow that feature actually makes. Same code, two completely different pictures."

---

## 15 · Turn the boundary into policy

**Timing:** 7:30 · 0:30

**On screen:** Match the gallery binary connecting to :8080. · metadata: · policies/after-party/gallery-payments-block.yaml

> "One TracingPolicy. Hook the connect path, match only the gallery binary, only when it dials port 8080, payments. The action is Override: the connect returns EPERM. Everything else the gallery does is untouched."

---

## 16 · Demo: block it

**Timing:** 8:00 · 1:45 · DEMO 2 (block · Override) · the audience uploads again

**On screen:** upload again now 📱

1. make block  (wait ~2 s)
2. "Everyone: upload another photo now." (QR on this slide.) Their phones show "Upload service currently down. Please try again later." (HTTP 503). Payments pane: NO more GET /vault.
3. "Photo wall still up? Try buying a ticket: still works." Only the compromised call fails.
4. make prove → "This is the policy that fired: gallery-payments-block, on the gallery binary."
5. make status → NENFORCE climbing with every one of your uploads. "That counter is Tetragon actually enforcing it: the kernel answered 'operation not permitted' (EPERM) to each of your uploads' connect. No pod restart, no outage."

---

## 17 · Tetragon blocked the upload

**Timing:** 9:45 · 0:10  SKIP if the live demo worked (fallback / recap only)

> "Upload: 503. Ticket: 200. make prove shows the policy that fired: gallery-payments-block, on /usr/local/bin/gallery. And make status is the proof it was enforced: the NENFORCE counter went from 0 to 1."
Careful: the OVERRIDE label is the policy's configured action. It also shows up in monitor mode, so it is not, on its own, proof of a block.

---

## 18 · Demo: or stop it

**Timing:** 9:55 · 1:00 · DEMO 3 (kill · Sigkill) · presenter only: "phones down"

> 0. The kill restarts the gallery container but the wall survives (uploads are saved on an emptyDir). The rollout restart after the talk deletes the pod and empties it: take a photo of the wall first if you want it.
1. make kill  (it then watches pods)
2. Upload ×3 yourself (or make curl ×3): each upload returns 503 and the gallery RESTARTS count climbs. frontend + payments stay Running.
> "Sometimes you don't want surgical, you want it gone. Same behaviour caught; I only changed what Tetragon does."
After the talk: make monitor + kubectl rollout restart deploy/gallery.

---

## 19 · Tetragon stopped the gallery

**Timing:** 10:55 · 0:10  SKIP if the live demo worked (fallback / recap only)

> "Every forbidden connect kills the process: gallery restarts climb, frontend and payments never blink."

---

## 20 · -  "github.com/octopkg/upload"

**Timing:** 11:05 · 0:35

**On screen:** The fix is one line · git diff after-party fix/clean-upload -- services/gallery/main.go

> "And the fix? One line. Swap the sketchy dependency for an in-house package that makes the same thumbnail with none of the payload. The lesson isn't 'this library is bad'. It's that we couldn't tell from the code, and we could tell instantly from the behaviour."

---

## 21 · Tetragon in GitHub Actions

**Timing:** 11:40 · 0:50

**TODO  screenshots side by side: PR #4 (5 green + runtime-check RED) | PR #5 (all 6 GREEN)**

> "You can pull this left, into CI. Alongside the five static checks, a sixth job stands up kind and Tetragon, uploads a sighting, and fails the build when the gallery→payments call shows up. On the agent's PR: five green, one red. On the fix: all six green. That's the check that would have caught our agent."

---

## 22 · The honest part

**Timing:** 12:30 · 0:50

**On screen:** CI only sees what runs in CI. · Production is where the real secrets and the real network are. · Your policies are your responsibility. · Tetragon gives you the eyes. To prevent the flow, add identity-aware network policy with Cilium. · Watch production too

> "Two honest caveats. A runtime check in CI is a great signal, but production is where the real secrets and the real network are, so watch there too. And this policy is an example: you decide which flows matter. Tetragon gives you the eyes; the judgement is yours. And if you want to prevent the flow, not just catch it, that's identity-aware network policy with Cilium."

---

## 23 · Who's watching the agents?

**Timing:** 13:20 · 0:30  (then ~1:00 buffer)

**On screen:** github.com/doniacld/gh26-tetragon

> "AI will keep writing more of our code, faster than we can read it. Static checks stay essential, but they read the code. Runtime is where you see what it does. Thanks for being my users today. So: who's watching the agents? Now you can. Everything I showed is in this repo."
The repo must be PUBLIC before the talk for the QR code to work.

---

## 24 · (visual only)

**Timing:** 13:50 · Thank you.

---

## 25 · (visual only)

**Timing:** 14:00 · Survey QR (template slide).

---

## Appendix: demo cheat sheet (not a slide)

Keep this open on a second screen. Full detail in [`demo-runbook.md`](demo-runbook.md).

| Beat | Command | Point at |
|------|---------|----------|
| Setup (T-30 min) | `make demo-up` + `make forward` + `make open` | site loads, 3 pods Running |
| Panes | `make watch` · `make logs SVC=payments` · command pane | start them before uploading |
| 0 · healthy | browser: buy a ticket ($10, prefilled card) | `charged $10.00 … POST /charge`, `make watch` quiet |
| 1 · monitor | browser: upload a sighting | photo on the wall, `make watch` connect line, payments `GET /vault` |
| 2 · block | `make block` → upload → `make prove` → `make status` | 503, no `GET /vault`, policy that fired, NENFORCE 0 → 1 |
| 3 · kill | `make kill` → upload ×3 (or `make curl`) | uploads 503, gallery RESTARTS climb, frontend/payments Running |
| Fix | `git diff after-party fix/clean-upload -- services/gallery/main.go` | one import line |
| Reset | `make monitor` + `kubectl --context kind-agents-demo -n party rollout restart deploy/gallery deploy/payments` | clean wall, ticket counter back to OCTO-0001 |

Rehearsal reminders:
- `tetra getevents` is a live stream: start `make watch` *before* uploading (`make events` dumps earlier ones).
- Give `make block` / `make kill` a couple of seconds to propagate before uploading.
- `KPROBE_ACTION_OVERRIDE` in `make prove` is the policy's configured action (it shows in monitor mode too):
  the proof of the block is the 503 + `make status` NENFORCE.
- `Override` only loads on syscalls / `security_` hooks, hence `security_socket_connect` for the block
  beat and `tcp_connect` for the kill beat.
