"""Rewrite the Universe 2026 template deck for the After-Party story.

Reads the original deck (never modified) and writes a v2 copy next to it:
    python3 -m venv ~/.cache/deckvenv && ~/.cache/deckvenv/bin/pip install python-pptx segno
    ~/.cache/deckvenv/bin/python talk/build_template_deck.py [path/to/U26_whos_watching_agent.pptx]

Reuses the speaker's own slides (title, bio, Tetragon, thank you, survey), adds
new slides from the template's layouts, drops the v1 (speaker portal) slides and
the template's instruction/example slides, and writes the script + timings into
every slide's speaker notes. Images come from talk/assets (make_assets.py).
"""
import pathlib
import re
import sys

import segno
from PIL import Image
from pptx import Presentation
from pptx.dml.color import RGBColor
from pptx.util import Inches, Pt

HERE = pathlib.Path(__file__).resolve().parent
ASSETS = HERE / "assets"
ROOT = HERE.parent
SRC = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else
                   "~/Documents/Talks/github-universe-26/U26_whos_watching_agent.pptx").expanduser()
OUT = SRC.with_name(SRC.stem + "_v2.pptx")
REPO = "github.com/doniacld/gh26-tetragon"

BAD, GOOD, DIM = RGBColor(0xFF, 0x81, 0x82), RGBColor(0x5F, 0xED, 0x83), RGBColor(0x8B, 0x94, 0x9E)

prs = Presentation(SRC)
orig = list(prs.slides)  # 0-based: orig[0] is slide 1
LAYOUTS = {l.name: l for l in orig[0].slide_layout.slide_master.slide_layouts}


def notes(s, text):
    s.notes_slide.notes_text_frame.text = text.strip()


def ph(s, idx):
    return next(p for p in s.placeholders if p.placeholder_format.idx == idx)


def drop(shape):
    shape._element.getparent().remove(shape._element)


def drop_named(s, name):
    for sh in list(s.shapes):
        if sh.name == name:
            drop(sh)


def new(layout, keep=()):
    """Add a slide from a template layout; remove placeholders not listed in keep."""
    s = prs.slides.add_slide(LAYOUTS[layout])
    for p in list(s.placeholders):
        if p.placeholder_format.idx not in keep:
            drop(p)
    return s


def fit(s, name, x, y, w, h):
    """Add an image scaled to fit the box, centred."""
    iw, ih = Image.open(ASSETS / name).size
    k = min(w / iw, h / ih)
    pw, phh = int(iw * k), int(ih * k)
    return s.shapes.add_picture(str(ASSETS / name), x + (w - pw) // 2, y + (h - phh) // 2, pw, phh)


def code(placeholder, lines, size, marks=()):
    """Write code lines into a template code placeholder; marks = {substring: colour}."""
    tf = placeholder.text_frame
    tf.text = ""
    for i, line in enumerate(lines):
        p = tf.paragraphs[0] if i == 0 else tf.add_paragraph()
        r = p.add_run()
        r.text = line.replace("\t", "  ") or " "
        r.font.size = Pt(size)
        for m, colour in dict(marks).items():
            if m in line:
                r.font.color.rgb = colour
        if line.lstrip().startswith(("//", "#")):
            r.font.color.rgb = DIM


def large(text, note, footer="@doniacld"):
    s = new("Large statement", keep=(11, 14))
    ph(s, 11).text_frame.text = text
    ph(s, 14).text_frame.text = footer
    notes(s, note)
    return s


def two(header, title, s1, d1, s2, d2, note):
    s = new("Two statements", keep=(32, 39, 22, 23, 36, 37, 14))
    for idx, t in [(32, header), (39, title), (22, s1), (23, d1), (36, s2), (37, d2), (14, "@doniacld")]:
        ph(s, idx).text_frame.text = t
    notes(s, note)
    return s


def code_right(title, text, code_title, lines, size, marks, note):
    s = new("Code right side", keep=(11, 12, 10, 13))
    ph(s, 11).text_frame.text = title
    ph(s, 12).text_frame.text = text
    ph(s, 13).text_frame.text = code_title
    code(ph(s, 10), lines, size, marks)
    notes(s, note)
    return s


def todo(s, shape_name, text):
    for sh in s.shapes:
        if sh.name == shape_name:
            sh.text_frame.text = "TODO  " + text


# ---------------------------------------------------------------- reused slides
title, bio, portal, normal, ci, wrong = orig[0], orig[1], orig[2], orig[3], orig[4], orig[5]
tetragon, events, blocked, actions, thanks, survey = orig[9], orig[11], orig[14], orig[15], orig[16], orig[17]

notes(title, """
0:00 · 0:20
"AI agents are writing and merging code in our repos right now. The tests pass, the PR is green, it ships. My question today is simple: once that code is running in production, who's actually watching what it does?"
""")
notes(bio, "0:20 · 0:20  Quick intro, keep it short: the demo is the star.")

ph(portal, 48).text_frame.text = "Mona's After-Party"
drop_named(portal, "TextBox 3")
ph(portal, 40).insert_picture(str(ASSETS / "site-home.png"))
notes(portal, """
0:40 · 0:30
"Here's our app: a little site for the Octocat after-party. Three services: the site, a photo wall, and a payments service to buy a ticket. Everything I show today runs on my laptop in a kind cluster."
""")

todo(normal, "TextBox 3", "screenshot: issue #6 + the Copilot Agent mode chat that implemented it")
notes(normal, """
1:10 · 0:30
"This is normal now. I hand the ticket to an agent, it writes the code, I open the PR. A reviewer skims it, sees green checks, hits merge. The agent is fast and mostly right, which is exactly why we stop reading carefully."
""")

todo(ci, "TextBox 4", "screenshot: the 5 green static checks (build · test · vet · fmt · vuln) on the Copilot PR, PR #4 as fallback (crop out runtime-check, it comes back later)")
notes(ci, """
2:40 · 0:30
"The pipeline agrees. It builds, tests pass, go vet and gofmt are happy. And govulncheck is green too: there is no CVE here. Static analysis has nothing to grab onto. So it ships."
""")

notes(wrong, """
3:10 · 0:15
"Green across the board, merged, deployed. Everybody moves on. So what does this code actually do when it runs?"
""")

notes(tetragon, """
3:25 · 0:40
"This is Tetragon, a CNCF project built on eBPF. It runs in the Linux kernel and sees what every process does: what executes, what files get read, what connections open, and crucially which workload did it. It doesn't care what the code says. It sees what it does."
""")

# Tetragon events: swap the v1 terminal for the real After-Party capture.
drop_named(events, "Picture 9")
fit(events, "term-monitor.png", Inches(0.4), Inches(2.2), Inches(12.5), Inches(3.0))
for sh in events.shapes:
    if sh.name == "TextBox 11":
        sh.text_frame.paragraphs[0].runs[0].text = "One upload: the gallery binary dials payments:8080 and reads /vault"
        for r in sh.text_frame.paragraphs[0].runs[1:]:
            r.text = ""
        sh.left, sh.top, sh.width = Inches(0.4), Inches(5.6), Inches(12.5)
notes(events, """
6:20 · 0:20  (recap of demo 1; also the fallback if the live demo fails)
"Two views of one upload. Tetragon: the gallery binary opened a connection to payments on 8080. Payments: someone called GET /vault. The gallery's own log just says POST /upload 201."
""")

ph(blocked, 48).text_frame.text = "Tetragon blocked the upload"
drop(ph(blocked, 37))
fit(blocked, "site-blocked.png", Inches(0.4), Inches(2.3), Inches(6.1), Inches(4.6))
fit(blocked, "term-block.png", Inches(6.8), Inches(2.3), Inches(6.1), Inches(4.6))
notes(blocked, """
10:00 · 0:20  (recap of demo 2; fallback if the live demo fails)
"Upload: 503. Ticket: 200. make prove shows the policy that fired: gallery-payments-block, on /usr/local/bin/gallery. And make status is the proof it was enforced: the NENFORCE counter went from 0 to 1."
Careful: the OVERRIDE label is the policy's configured action. It also shows up in monitor mode, so it is not, on its own, proof of a block.
""")

todo(actions, "TextBox 5", "screenshots side by side: PR #4 (5 green + runtime-check RED) | PR #5 (all 6 GREEN)")
notes(actions, """
12:10 · 0:50
"You can pull this left, into CI. Alongside the five static checks, a sixth job stands up kind and Tetragon, uploads a sighting, and fails the build when the gallery→payments call shows up. On the agent's PR: five green, one red. On the fix: all six green. That's the check that would have caught our agent."
""")

notes(thanks, "14:20 · Thank you.")
notes(survey, "14:30 · Survey QR (template slide).")

# ------------------------------------------------------------------- new slides
task = large("“Let attendees upload their own Octocat sightings.”", """
1:40 · 0:20
"Here's the task we gave the agent: let people upload their own sightings to the photo wall. Utterly boring. The kind of change you approve between meetings."
""")

g = (ROOT / "services/gallery/main.go").read_text().splitlines()
a = g.index("import (")
agent = code_right(
    "What the agent wrote",
    "Read the file, make a thumbnail, pin it to the wall.\n\nOne new dependency: the image helper our Copilot instructions recommend.",
    "services/gallery/main.go", g[a:g.index(")", a) + 1], 16, {"octopkg/upload": BAD}, """
2:00 · 0:40
"And this is what it wrote. Read the file, make a thumbnail, add it to the wall. For the thumbnail it pulled in one small dependency: octopkg/upload. Our Copilot instructions even recommend it as the approved image helper. Looks great. Would you block this PR?"
""")

demo1 = large("Demo: watch it", """
4:05 · 2:15 · DEMO 1 (monitor)
Screen: browser left · right: make watch | make logs SVC=payments | command pane
1. Home → Photo wall. "Three services, a photo wall, a ticket shop."
2. Get a ticket → Pay $10.00 → receipt OCTO-0001. Payments pane: "charged $10.00 … POST /charge -> 201". make watch stays quiet. "Frontend → payments: expected."
3. Photo wall → upload a sighting. It lands on the wall.
4. make watch: "security_socket_connect gallery -> …:8080". Payments: "GET /vault -> 200".
"Look at this. The gallery just opened a connection to payments, and read the vault. Gallery serves photos; it has no reason to touch payments. The upload worked, the site looks fine: without this we'd never know. So where did this come from?"
""")

e = (ROOT / "evilupload/upload.go").read_text().splitlines()
p0 = next(i for i, l in enumerate(e) if l.startswith("func phoneHome"))
body = [re.sub(r"\s+//.*$", "", l) for l in e[p0:p0 + 19] if l.strip() and not l.strip().startswith("//")]
there = code_right(
    "There it is",
    "On every upload, the thumbnail helper dials payments and reads its vault.\n\nA few ordinary lines of Go. No exploit. No CVE.",
    "github.com/octopkg/upload", body, 13, {"/vault": BAD}, """
6:40 · 0:40
"Here it is, inside that little upload dependency. The thumbnail is real; that's why it passed review. But on every upload, the same function dials the payments service and reads its vault. No exploit, no CVE, in a file nobody opened. We didn't find this by reading it. We found it by watching the behaviour."
""")

views = two("Same code", "Two views", "Developer", "“I just added a photo upload.”",
            "Security", "“The gallery workload opens a socket to payments.”", """
7:20 · 0:25
"This is the gap in one slide. The developer sees a feature that looks fine. Tetragon shows the security engineer the flow that feature actually makes. Same code, two completely different pictures."
""")

y = (ROOT / "policies/after-party/gallery-payments-block.yaml").read_text().splitlines()
y = [re.sub(r"\s+#.*$", "", l) for l in y
     if l.strip() and not l.strip().startswith("#") and not l.startswith(("apiVersion", "kind"))]
policy = code_right(
    "Turn the boundary into policy",
    "Match the gallery binary connecting to :8080.\n\nOverride: the connect fails with EPERM.\n\nSwap the action to Sigkill to stop the container instead.",
    "policies/after-party/gallery-payments-block.yaml", y, 11,
    {"Override": BAD, "/gallery": GOOD, '"8080"': GOOD}, """
7:45 · 0:30
"One TracingPolicy. Hook the connect path, match only the gallery binary, only when it dials port 8080, payments. The action is Override: the connect returns EPERM. Everything else the gallery does is untouched."
""")

demo2 = large("Demo: block it", """
8:15 · 1:45 · DEMO 2 (block · Override)
1. make block  (wait ~2 s)
2. Upload a sighting → red "Upload service currently down. Please try again later." (HTTP 503), no new photo. Payments pane: NO GET /vault.
3. Photo wall, buy a ticket: both still work.
"Same code, same policy. I just switched it to enforce. Upload… blocked. The wall is up, tickets still sell. Only the compromised call failed."
4. make prove → "This is the policy that fired: gallery-payments-block, on the gallery binary."
5. make status → NENFORCE = 1. "And this counter is Tetragon telling us it actually enforced it: the kernel answered 'operation not permitted' (EPERM) to the gallery's connect. No pod restart, no outage."
""")

demo3 = large("Demo: or stop it", """
10:15 · 1:00 · DEMO 3 (kill · Sigkill)
1. make kill  (it then watches pods)
2. Upload ×3 (or make curl ×3 in another pane): each upload returns 503 and the gallery RESTARTS count climbs. frontend + payments stay Running.
"Sometimes you don't want surgical, you want it gone. Now the upload dials payments and Tetragon kills the gallery process. Same behaviour caught; I only changed what Tetragon does."
After the talk: make monitor + kubectl rollout restart deploy/gallery.
""")

killed = new("Title and image stacked", keep=(48, 14))
ph(killed, 48).text_frame.text = "Tetragon stopped the gallery"
ph(killed, 14).text_frame.text = ""
fit(killed, "term-kill.png", Inches(0.4), Inches(2.2), Inches(12.5), Inches(4.8))
notes(killed, """
11:15 · 0:15  (recap of demo 3; fallback if the live demo fails)
"Every forbidden connect kills the process: gallery restarts climb, frontend and payments never blink."
""")

fix = new("Developer code layout", keep=(10, 11, 12))
ph(fix, 11).text_frame.text = "The fix is one line"
ph(fix, 12).text_frame.text = "git diff after-party fix/clean-upload -- services/gallery/main.go"
code(ph(fix, 10), ['-\t"github.com/octopkg/upload"',
                   '+\tupload "github.com/doniacld/gh26-tetragon/internal/sightings"'],
     16, {"-\t": BAD, "+\t": GOOD})
notes(fix, """
11:30 · 0:40
"And the fix? One line. Swap the sketchy dependency for an in-house package that makes the same thumbnail with none of the payload. The lesson isn't 'this library is bad'. It's that we couldn't tell from the code, and we could tell instantly from the behaviour."
""")

honest = two("The honest part", "Watch production too", "CI only sees what runs in CI.",
             "Production is where the real secrets and the real network are.",
             "Your policies are your responsibility.",
             "Tetragon gives you the eyes. To prevent the flow, add identity-aware network policy with Cilium.", """
13:00 · 0:50
"Two honest caveats. A runtime check in CI is a great signal, but production is where the real secrets and the real network are, so watch there too. And this policy is an example: you decide which flows matter. Tetragon gives you the eyes; the judgement is yours. And if you want to prevent the flow, not just catch it, that's identity-aware network policy with Cilium."
""")

close = large("Who's watching the agents?\nYou can. At runtime.", f"""
13:50 · 0:30
"AI will keep writing more of our code, faster than we can read it. Static checks stay essential, but they read the code. Runtime is where you see what it does. So: who's watching the agents? Now you can. Everything I showed is in this repo."
The repo must be PUBLIC before the talk for the QR code to work.
""", footer=REPO)
segno.make("https://" + REPO, error="m").save(str(ASSETS / "qr-repo.png"), scale=12, border=2,
                                             dark="#0d1117", light="#e6edf3")
close.shapes.add_picture(str(ASSETS / "qr-repo.png"), Inches(11.0), Inches(4.9), Inches(1.9), Inches(1.9))

# ------------------------------------------------------------ order and cleanup
order = [title, bio, portal, normal, task, agent, ci, wrong, tetragon, demo1, events, there,
         views, policy, demo2, blocked, demo3, killed, fix, actions, honest, close, thanks, survey]

lst = prs.slides._sldIdLst
by_id = {s.slide_id: el for s, el in zip(prs.slides, list(lst))}
wanted = [s.slide_id for s in order]  # resolve ids while every slide is still listed
for sid, el in by_id.items():
    if sid not in wanted:
        prs.part.drop_rel(el.rId)
for el in list(lst):
    lst.remove(el)
for sid in wanted:
    lst.append(by_id[sid])

prs.save(OUT)
print(f"wrote {OUT} ({len(order)} slides)")
