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

It photographs a window of the workbench and nothing else. The active window
during a nine-second settle is whatever the person at the keyboard last
clicked, and this wrote two of those to disk under the names of panels before
it was tightened: a remote-desktop session, then a browser. The test is now
that the window belongs to the process we launched - not that it carries a
particular title, because half these steps are about another of our own
windows and a popped-out panel is titled after the panel. A missing picture is
the failure mode, which is the right one.
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


def _kdotool(*args):
    """One kdotool call, or "" if it is not installed or says nothing."""
    if not shutil.which("kdotool"):
        return ""
    try:
        out = subprocess.run(["kdotool", *args], capture_output=True,
                             text=True, timeout=10)
    except subprocess.SubprocessError:
        return ""
    return out.stdout.strip()


def ours(pid):
    """Every process id the capture may belong to: the workbench and its own.

    A popped-out panel is a window of the same process, so one pid covers the
    main window, the node windows and the board view alike.
    """
    out = {pid}
    try:
        kids = subprocess.run(["pgrep", "-P", str(pid)], capture_output=True,
                              text=True, timeout=5).stdout.split()
        out |= {int(k) for k in kids}
    except (subprocess.SubprocessError, ValueError):
        pass
    return out


def capture(out, pid):
    """One picture of a window belonging to *our* workbench, or none at all.

    It used to photograph whatever had focus. The intent was always
    window-only - a fullscreen grab takes the rest of the desktop with it, and
    what is on the rest of the desktop is nobody's business - but the active
    window during a nine-second settle is whatever the person at the keyboard
    last clicked. On one run that was a remote-desktop session and on the next
    a browser, both written to disk under the name of a panel.

    The test is ownership, not the title. Half these steps are *about* another
    of our own windows - a popped-out panel is titled after the panel, a node
    window after the node - so insisting on the main window's title would
    photograph the wrong one of ours, which is the same fault wearing our own
    colours. A pid cannot be borrowed by a browser that happens to have the
    project open.
    """
    win = _kdotool("getactivewindow")
    if not win:
        print("  no active window to photograph (is kdotool installed?)",
              file=sys.stderr)
        return False
    owner = _kdotool("getwindowpid", win)
    if not owner.isdigit() or int(owner) not in ours(pid):
        name = _kdotool("getwindowname", win)
        print(f"  the active window is not ours ({name!r}); not photographing it",
              file=sys.stderr)
        return False

    if shutil.which("spectacle"):
        ok = subprocess.call(
            ["spectacle", "-a", "--new-instance", "-b", "-n", "-o", out],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) == 0
    elif shutil.which("grim"):
        # grim with no geometry photographs the whole screen, which is the one
        # thing this must not do. Only the geometry form is allowed.
        geom = _kdotool("getwindowgeometry", "--shell", win)
        box = dict(l.split("=", 1) for l in geom.splitlines() if "=" in l)
        try:
            x, y = int(box["X"]), int(box["Y"])
            w, h = int(box["WIDTH"]), int(box["HEIGHT"])
        except (KeyError, ValueError):
            print("  grim needs the window geometry and kdotool did not give"
                  " it; refusing to photograph the whole screen", file=sys.stderr)
            return False
        ok = subprocess.call(["grim", "-g", f"{x},{y} {w}x{h}", out],
                             stderr=subprocess.DEVNULL) == 0
    else:
        sys.exit("no window capture tool found: install spectacle or grim")

    after = _kdotool("getactivewindow")
    if ok and after and after != win:
        os.remove(out)
        print("  focus moved during the grab; the picture was discarded",
              file=sys.stderr)
        return False
    return ok


# What every launch writes to stderr and nobody needs to hear about.
QUIET = ("session log:", "control socket:", "closing:")


def tail(path):
    """What the workbench said for itself, minus its own startup chatter."""
    try:
        with open(path, "rb") as f:
            said = f.read().decode(errors="replace")
    except OSError:
        return ""
    keep = [l for l in said.splitlines()
            if l.strip() and not l.startswith(QUIET)]
    return " / ".join(keep)[-400:]


def run(step, binary, fixture, outdir):
    out = os.path.join(outdir, step["name"] + ".png")
    # A step may name its own fixture. The board view needs a node running a
    # board image and the default fixture has none, so all three board steps
    # were refused - and, before the stderr check below, refused silently.
    cmd = [binary, "workbench",
           "-fixture", step.get("fixture", fixture)] + step["flags"]
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
    # Into a file rather than a pipe: a refusal has to be readable while the
    # window is still up, and a pipe can only be drained once the process ends.
    # A step may need a setting switched on. The Energy panel is behind
    # MESHBENCH_ENERGY until its model is trusted, and panelMenus lists it
    # unconditionally - so the manifest asked for a picture of a panel a
    # default build does not register, and the two steps could never pass.
    # Switching it on for those steps photographs the real panel, which is what
    # the manifest is for.
    env = None
    if step.get("env"):
        env = dict(os.environ)
        env.update(step["env"])
    errf = os.path.join(outdir, step["name"] + ".stderr")
    with open(errf, "wb") as e:
        proc = subprocess.Popen(cmd, stdout=subprocess.DEVNULL, stderr=e, env=env)
    try:
        time.sleep(settle)
        if proc.poll() is not None:
            print("  the workbench exited before it could be photographed:",
                  tail(errf) or "no output", file=sys.stderr)
            return False
        # A flag the workbench refused leaves the window up and perfectly
        # usable, so this used to photograph it and call the step a success.
        # Three board-view steps named a node no fixture has and reported green
        # for months on a picture of a plain workbench. The refusal is the
        # loudest signal there is; discarding it was the whole fault.
        if said := tail(errf):
            print("  the workbench refused something:", said, file=sys.stderr)
            return False
        ok = capture(out, proc.pid)
        if step.get("then"):
            # A step needing a click cannot be finished by a script: the
            # window is left up and the operator is told what to do with it.
            print("  left open -", step["then"])
            input("  press enter once done> ")
            ok = capture(out, proc.pid)
        return ok
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("buckets", nargs="*", help="only these buckets")
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
            print(f"{b:14} {s['name']:26} {' '.join(s['flags'])}")
        print(f"\n{len(chosen)} steps")
        return

    if not os.path.exists(a.binary):
        sys.exit(f"no binary at {a.binary}: go build -o meshbench ./cmd/meshbench")
    os.makedirs(a.out, exist_ok=True)

    failed = []
    skipped = []
    for i, (b, s) in enumerate(chosen, 1):
        print(f"[{i}/{len(chosen)}] {b}/{s['name']} - {s['what']}")
        # A step that says what it is waiting for is not a step that failed.
        # Saying so beats both a red run nobody can act on and a picture of
        # something else.
        if s.get("needs"):
            print("  skipped - needs", s["needs"])
            skipped.append(s["name"])
            continue
        if not run(s, a.binary, a.fixture, a.out):
            failed.append(s["name"])
    took = len(chosen) - len(failed) - len(skipped)
    print(f"\n{took} of {len(chosen)} captured into {a.out}")
    if skipped:
        print("skipped, and why is in steps.json:", ", ".join(skipped))
    if failed:
        print("no picture for:", ", ".join(failed))
        sys.exit(1)


if __name__ == "__main__":
    main()
