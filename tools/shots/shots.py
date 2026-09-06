#!/usr/bin/env python3
"""Drive the workbench once per capture step and keep a picture of each.

    tools/shots/shots.py                 every step
    tools/shots/shots.py panels views    only those buckets
    tools/shots/shots.py --list          what would run, and nothing else

The steps are in steps.json beside this file, and internal/ui/workbench's
shotsteps_test.go checks that list against the application's own panel table -
so a panel added without a step is a red build rather than a picture nobody
took.

Why a script rather than a Go test: a panel's real content needs a real
session behind it - a fixture, terrain, firmware - and a unit test drawing the
panel struct in isolation photographs the layout and nothing about whether the
thing works. This launches the binary the way somebody would.

One window at a time, deliberately. Several workbenches at once will make the
control socket time out, which reads as a hung capture rather than as a
loaded machine.
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
STEPS = os.path.join(HERE, "steps.json")

# Long enough for the window to map, the fixture to warm and the panel to draw
# its first real frame. A capture taken before that is a picture of an empty
# panel, which passes for a broken panel.
SETTLE = 9.0


def capture(out):
    """One window-only picture. A fullscreen grab takes the rest of the desktop
    with it, and what is on the rest of the desktop is nobody's business."""
    if shutil.which("spectacle"):
        return subprocess.call(
            ["spectacle", "-a", "--new-instance", "-b", "-n", "-o", out],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) == 0
    if shutil.which("grim"):
        return subprocess.call(["grim", out],
                               stderr=subprocess.DEVNULL) == 0
    sys.exit("no window capture tool found: install spectacle or grim")


def read_page(step, outdir):
    """A documentation step: open the page and let somebody read it.

    Not automated, and deliberately. What is being checked is whether the page
    is still true of the application - whether a step can be followed, whether
    a screenshot on it is recognisable - and no script can answer that. The
    browser is opened so the reading actually happens, and the picture is of
    what the reader concluded.
    """
    print("  ", step["url"])
    print("  expected:", step["expect"])
    if shutil.which("xdg-open"):
        subprocess.Popen(["xdg-open", step["url"]],
                         stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    input("  press enter once read, having captured anything that disagrees> ")
    return True


def run(step, binary, fixture, outdir):
    if step.get("url"):
        return read_page(step, outdir)
    out = os.path.join(outdir, step["name"] + ".png")
    cmd = [binary, "workbench", "-fixture", fixture] + step["flags"]
    # A panel that is empty without traffic is a panel whose picture cannot be
    # judged: "showing 0 of 0 events" looks exactly like a panel that does not
    # work. Those steps say so, and get the run started and longer to fill.
    #
    # Starting the clock is not enough on its own - it advances time and leaves
    # every counter at zero. The steps that need rows carry -inject too, which
    # originates packets in the engine without waiting for firmware to boot on
    # fifty-eight nodes.
    settle = SETTLE
    if step.get("play"):
        cmd.append("-play")
        settle = SETTLE + 12
    proc = subprocess.Popen(cmd, stdout=subprocess.DEVNULL,
                            stderr=subprocess.PIPE)
    try:
        time.sleep(settle)
        if proc.poll() is not None:
            err = (proc.stderr.read() or b"").decode()[-400:]
            print("  the workbench exited before it could be photographed:",
                  err.strip() or "no output", file=sys.stderr)
            return False
        ok = capture(out)
        if step.get("then"):
            # A step needing a click cannot be finished by a script: the
            # window is left up and the operator is told what to do with it.
            print("  left open -", step["then"])
            input("  press enter once done> ")
            ok = capture(out)
        return ok
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("buckets", nargs="*",
                    help="only these buckets; \"docs\" walks the published "
                         "pages rather than the application")
    ap.add_argument("--list", action="store_true", help="say what would run")
    ap.add_argument("--out", default=os.path.join(ROOT, "shots"))
    ap.add_argument("--binary", default=os.environ.get(
        "MESHBENCH_BINARY", os.path.join(ROOT, "meshbench")))
    ap.add_argument("--fixture", default="fixture-fife-strict")
    a = ap.parse_args()

    steps = json.load(open(STEPS))
    buckets = a.buckets or list(steps)
    chosen = [(b, s) for b in buckets for s in steps.get(b, [])]
    if not chosen:
        sys.exit("no steps: buckets are " + ", ".join(steps))

    if a.list:
        for b, s in chosen:
            how = s.get("url") or " ".join(s["flags"])
            print(f"{b:14} {s['name']:26} {how}")
        print(f"\n{len(chosen)} steps")
        return

    if not os.path.exists(a.binary):
        sys.exit(f"no binary at {a.binary}: go build -o meshbench ./cmd/meshbench")
    os.makedirs(a.out, exist_ok=True)

    failed = []
    for i, (b, s) in enumerate(chosen, 1):
        print(f"[{i}/{len(chosen)}] {b}/{s['name']} - {s['what']}")
        if not run(s, a.binary, a.fixture, a.out):
            failed.append(s["name"])
    print(f"\n{len(chosen) - len(failed)} of {len(chosen)} captured into {a.out}")
    if failed:
        print("no picture for:", ", ".join(failed))
        sys.exit(1)


if __name__ == "__main__":
    main()
