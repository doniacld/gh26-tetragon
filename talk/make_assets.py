"""Render the code snippets and site screenshots used by the deck into talk/assets/.

Run from the repo root: python3 talk/make_assets.py (needs Go + Google Chrome).
"""
import html, os, re, subprocess, sys, tempfile, time, pathlib

ROOT = pathlib.Path(__file__).resolve().parent.parent
OUT = ROOT / "talk" / "assets"
CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
OUT.mkdir(parents=True, exist_ok=True)

def shot(url, out, w, h):
    # Chrome sometimes exits non-zero on teardown after writing the file, so
    # judge success by the PNG, not the exit code.
    out = pathlib.Path(out)
    out.unlink(missing_ok=True)
    with tempfile.TemporaryDirectory() as prof:
        try:
            subprocess.run([CHROME, "--headless=new", f"--user-data-dir={prof}", "--disable-gpu",
                            "--hide-scrollbars", "--force-device-scale-factor=2",
                            f"--window-size={w},{h}", f"--screenshot={out}", url],
                           capture_output=True, timeout=60)
        except subprocess.TimeoutExpired:
            pass
    if not out.exists():
        sys.exit(f"screenshot failed: {url}")

KW = r"\b(func|return|if|err|nil|defer|import|package|var|const)\b"
def hl(line):
    out, i = "", 0
    for m in re.finditer(r'(//.*$)|("[^"]*")|' + KW, line):
        out += html.escape(line[i:m.start()])
        cls = "c" if m.group(1) else "s" if m.group(2) else "k"
        out += f'<span class="{cls}">{html.escape(m.group(0))}</span>'
        i = m.end()
    return out + html.escape(line[i:])

def code(name, title, lines, first, mark=(), diff=False):
    rows = []
    for n, l in enumerate(lines, first):
        cls = ""
        if diff and l.startswith("-"): cls = "del"
        elif diff and l.startswith("+"): cls = "add"
        elif n in mark: cls = "mark"
        num = "" if diff else str(n)
        rows.append(f'<div class="l {cls}"><span class="n">{num}</span>{hl(l) or "&nbsp;"}</div>')
    page = f"""<html><head><meta charset="utf-8"><style>
body{{margin:0;background:#0d1117;font:20px/1.55 Menlo,monospace;color:#e6edf3}}
.t{{padding:14px 24px;color:#7d8590;font:600 16px -apple-system,sans-serif;border-bottom:1px solid #30363d;background:#161b22}}
.b{{padding:16px 0}} .l{{padding:0 24px;white-space:pre}} .n{{display:inline-block;width:44px;color:#484f58}}
.k{{color:#ff7b72}} .s{{color:#a5d6ff}} .c{{color:#8b949e}}
.mark{{background:rgba(248,81,73,.20);box-shadow:inset 4px 0 #f85149}}
.del{{background:rgba(248,81,73,.18)}} .add{{background:rgba(63,185,80,.18)}}
</style></head><body><div class="t">{html.escape(title)}</div><div class="b">{''.join(rows)}</div></body></html>"""
    f = OUT / f"{name}.html"; f.write_text(page)
    shot(f.as_uri(), OUT / f"{name}.png", 1100, 90 + 31 * len(lines) + 40)
    f.unlink()

def src(path, a, b):
    return (ROOT / path).read_text().splitlines()[a - 1:b]

# Code snippets.
g = (ROOT / "services/gallery/main.go").read_text().splitlines()
imp = next(i for i, l in enumerate(g, 1) if l.strip() == "import (")
end = next(i for i, l in enumerate(g, 1) if i > imp and l.strip() == ")")
code("code-import", "services/gallery/main.go", g[imp - 1:end], imp,
     mark=[i for i, l in enumerate(g, 1) if "octopkg/upload" in l and l.startswith("\t")])
e = (ROOT / "evilupload/upload.go").read_text().splitlines()
ph = next(i for i, l in enumerate(e, 1) if l.startswith("func phoneHome"))
code("code-phonehome", "github.com/octopkg/upload · upload.go", e[ph - 1:ph + 18], ph,
     mark=[i for i, l in enumerate(e, 1) if "/vault" in l and "client.Get" in l])
d = subprocess.run(["git", "diff", "after-party", "fix/clean-upload", "--", "services/gallery/main.go"],
                   cwd=ROOT, capture_output=True, text=True).stdout.splitlines()
hunk = [l for l in d[d.index(next(l for l in d if l.startswith("@@"))) + 1:]]
code("code-diff", "git diff after-party fix/clean-upload -- services/gallery/main.go", hunk, 1, diff=True)

# Terminal panes from real demo output saved in talk/captures/.
def term(name, panes, width=1600, stack=False):
    """panes: [(title, capture file, [substrings to highlight])], side by side or stacked."""
    cols = []
    for title, f, marks in panes:
        rows = []
        for l in (ROOT / "talk" / "captures" / f).read_text().splitlines():
            cls = "cmd" if l.startswith("$ ") else "mark" if any(m in l for m in marks) else ""
            rows.append(f'<div class="l {cls}">{html.escape(l) or "&nbsp;"}</div>')
        cols.append(f'<div class="p"><div class="t">{html.escape(title)}</div>{"".join(rows)}</div>')
    page = f"""<html><head><meta charset="utf-8"><style>
body{{margin:0;background:#0d1117;font:19px/1.6 Menlo,monospace;color:#e6edf3;display:flex;gap:2px;flex-direction:{'column' if stack else 'row'}}}
.p{{flex:1;background:#0d1117;padding:0 0 18px}}
.t{{padding:10px 20px;margin-bottom:10px;color:#7d8590;font:600 15px -apple-system,sans-serif;background:#161b22;border-bottom:1px solid #30363d}}
.l{{padding:0 20px;white-space:pre}} .cmd{{color:#3fb950}}
.mark{{color:#ffa198;background:rgba(248,81,73,.15);box-shadow:inset 4px 0 #f85149}}
</style></head><body>{"".join(cols)}</body></html>"""
    sizes = [len((ROOT / "talk" / "captures" / f).read_text().splitlines()) for _, f, _ in panes]
    h = sum(70 + 31 * n for n in sizes) if stack else 70 + 31 * max(sizes)
    f = OUT / f"{name}.html"; f.write_text(page)
    shot(f.as_uri(), OUT / f"{name}.png", width, h + 20)
    f.unlink()

term("term-monitor", [("make watch  ·  Tetragon", "watch.txt", ["gallery ->"]),
                      ("make logs SVC=payments", "payments.txt", ["/vault"])], width=1300, stack=True)
term("term-block", [("make block · make curl · make prove · make status", "block.txt", ["-> 503", "enforce   1"])], width=1100)
term("term-kill", [("make kill · make curl · kubectl get pods", "kill.txt", ["gallery-", "-> 503"])], width=1100)

# Site screenshots: run the three services locally.
bins = pathlib.Path(tempfile.mkdtemp())
subprocess.run(["go", "build", "-o", str(bins) + "/", "./services/..."], cwd=ROOT, check=True)
def run(svc, port, **env):
    return subprocess.Popen([str(bins / svc)], env={**os.environ, "PORT": str(port), **env},
                            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
pay = run("payments", 9103)
gal = run("gallery", 9102, PAYMENTS_ADDR="localhost:9103")
fe = run("frontend", 9101, GALLERY_ADDR="localhost:9102", PAYMENTS_ADDR="localhost:9103")
time.sleep(1.5)
base = "http://localhost:9101"
try:
    shot(base + "/", OUT / "site-home.png", 1280, 720)
    subprocess.run(["curl", "-s", "-o", "/dev/null", "-F", f"sighting=@{ROOT}/testdata/headshot.png",
                    base + "/upload"], check=True)
    shot(base + "/gallery", OUT / "site-wall.png", 1280, 1500)
    # Receipt and blocked pages are POST results: save the HTML, screenshot the file.
    for name, args in [("site-receipt", ["-X", "POST", base + "/buy"])]:
        h = subprocess.run(["curl", "-s"] + args, capture_output=True, text=True).stdout
        f = bins / f"{name}.html"; f.write_text(h); shot(f.as_uri(), OUT / f"{name}.png", 1280, 800)
    pay.kill(); time.sleep(0.5)  # payments unreachable = what the gallery sees under Override
    h = subprocess.run(["curl", "-s", "-F", f"sighting=@{ROOT}/testdata/headshot.png", base + "/upload"],
                       capture_output=True, text=True).stdout
    f = bins / "blocked.html"; f.write_text(h); shot(f.as_uri(), OUT / "site-blocked.png", 1280, 720)
finally:
    for p in (pay, gal, fe):
        p.kill()
print("assets written to", OUT)
