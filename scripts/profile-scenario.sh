#!/bin/bash
# profile-scenario.sh BIN HOME [DISPLAY_NUMBER]: the same use of an isolated app and daemon
# on Xvfb, measured after each part: what docs/performance.md's table is made of. BIN is a
# build of cmd/enowx. HOME is a new folder, whose path should be short (a socket lives
# there). It leaves them running, and prints their PIDs last; run it with nothing else
# of the host's: the daemon and sessions all live in HOME. Linux only.
set -u
bin=$1; home=$2; xnum=${3:-94}
reader=$(cat <<'PY'
import json, sys
p = json.load(sys.stdin)
M = 1 << 20
for k in ("daemon", "app"):
    x = p.get(k)
    if not x:
        continue
    m, g = x["memory"], x["go"]
    print("%-24s %-6s rss=%5.0f pss=%5.0f dirty=%5.0f heap=%6.1f free=%6.1f released=%6.1f goHeld=%6.1f sys=%5.0f gcs=%d" % (
        sys.argv[1], k, m["rss"] / M, m.get("pss", 0) / M, m.get("privateDirty", 0) / M, g["heapAlloc"] / M,
        (g["heapIdle"] - g["heapReleased"]) / M, g["heapReleased"] / M, (g["sys"] - g["heapReleased"]) / M, g["sys"] / M, g["numGC"]))
PY
)
cli() { env -u ENOWX_SESSION -u ENOWX_BIN HOME="$home" ENOWX_HOME="$home/.enowx-app" "$bin" "$@"; }
snap() { # label: one line for each process, in MB
	cli profile --json 2>/dev/null | python3 -c "$reader" "$1"
}
walk() { # step...: the steps of ui/devdriver.go, no picture drawn
	local out="$home/walk.done" steps
	steps=$(printf '"%s",' "$@")
	printf '{"out":"%s","noPicture":true,"steps":[%s]}' "$out" "${steps%,}" >"$home/.enowx-app/devshot.tmp"
	mv "$home/.enowx-app/devshot.tmp" "$home/.enowx-app/devshot.json"
	for _ in $(seq 1 30); do [ -f "$out.txt" ] && break; sleep 1; done
	unlink "$out.txt" 2>/dev/null
}
mkdir -p "$home/.enowx-app/settings" "$home/bin" "$home/proj"
echo '{"done":"2026-11-01T00:00:00Z","version":99}' >"$home/.enowx-app/settings/onboarding.json"
echo '{"onboardedDevice":true}' >"$home/.enowx-app/app.json"
printf '#!/bin/sh\nexec sleep 100000\n' >"$home/bin/claude"; chmod +x "$home/bin/claude"
Xvfb ":$xnum" -screen 0 1600x1000x24 -nolisten tcp >/dev/null 2>&1 &
xvfb=$!
sleep 1
(cd "$home" && exec env -u ENOWX_SESSION -u ENOWX_BIN -u WAYLAND_DISPLAY ENOWX_DEV=1 DISPLAY=":$xnum" HOME="$home" ENOWX_HOME="$home/.enowx-app" XDG_RUNTIME_DIR="$home" PATH="$home/bin:$PATH" "$bin") >"$home/app.out" 2>&1 &
app=$!
sleep 30
snap fresh
# Three agents, stand-ins with a conversation of 30 MB each.
folder="$home/.claude/projects/$(echo "$home/proj" | sed 's/[^A-Za-z0-9-]/-/g')"
mkdir -p "$folder"
python3 - "$folder" <<'PY'
import json,sys
words=" ".join(["lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor"]*40)
for i in (1,2,3):
    size,n=0,0
    with open(f"{sys.argv[1]}/00000000-0000-4000-8000-00000000000{i}.jsonl","w") as f:
        while size<30<<20:
            n+=1
            for r in ({"type":"user","timestamp":"2026-11-01T06:00:00Z","message":{"role":"user","content":f"Do step {n}: {words[:400]}"}},
                      {"type":"assistant","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":f"## Step {n}\n\n{words}\n"}]}},
                      {"type":"assistant","message":{"content":[{"type":"tool_use","id":f"t{n}","name":"Bash","input":{"command":"go test ./pkg"}}]}},
                      {"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":f"t{n}","content":words*5}]}}):
                s=json.dumps(r)+"\n"; f.write(s); size+=len(s)
PY
for i in 1 2 3; do
	cli spawn --agent claude --project "$home/proj" --name "c$i" hello >/dev/null 2>&1
	touch "$folder/00000000-0000-4000-8000-00000000000$i.jsonl"
	sleep 3
done
ids=""
for i in $(seq 1 20); do ids="$ids $(cli spawn --agent shell --project "$home/proj" --name "s$i" 2>&1 | tail -1)"; done
sleep 25
snap 20-sessions
for id in $(echo "$ids" | cut -d' ' -f2-6); do cli send "$id" "seq 1 150000 | sed 's/\$/ the quick brown fox jumps over the lazy dog/'" >/dev/null; done
sleep 30
snap 5-full-scrollbacks
# Five canvases of six designs, 300 KB of HTML each with about 2,000 elements.
python3 -c '
print("<!doctype html><html><body>")
for i in range(1900): print(f"<div class=\"row\" id=\"r{i}\"><h2>Card {i}</h2><p>The quick brown fox jumps over the lazy dog, on a page large enough to count.</p></div>")
print("</body></html>")' >"$home/page.html"
for i in 1 2 3 4 5; do
	id=$(cli canvas new "canvas $i" --kind mockup --frame f1:1280x800 --frame f2:1280x800 --frame f3:1280x800 --frame f4:1280x800 --frame f5:1280x800 --frame f6:1280x800 2>&1 | tail -1 | awk '{print $1}')
	for f in f1 f2 f3 f4 f5 f6; do cli canvas write "$id" --frame "$f" --html - <"$home/page.html" >/dev/null 2>&1; done
done
snap canvases-made
for page in notes canvas settings usage sync hosts vault; do walk "page:$page"; sleep 2; snap "page-$page"; done
walk page: session:c1 view:Chat; sleep 5; snap chat-1
walk session:c2 view:Chat; sleep 3; walk session:c3 view:Chat; sleep 3; snap chat-3
for i in 1 2 3 4 5 6; do walk "session:s$i"; done; snap six-terminals-shown
walk page:canvas; sleep 5
cli canvas goto "$(cli canvas list 2>/dev/null | awk 'NR==2{print $1}')" >/dev/null 2>&1; sleep 15; snap canvas-open
sleep 60; snap idle-1min
sleep 120; snap idle-3min
cli profile --cpu 20 >"$home/profile.txt" 2>&1
grep -E "^cpu:" "$home/profile.txt"
echo "app=$app xvfb=$xvfb daemon=$(cli profile --json 2>/dev/null | python3 -c 'import json,sys;print(json.load(sys.stdin)["daemon"]["pid"])')"
