"""Hand a freshly built MeshCore image to MeshBench.

Drop this beside MeshCore's own extra scripts and add it to an environment:

    extra_scripts = post:meshbench.py

After a successful build the image is handed to a running MeshBench through its
control socket, which files it under the board and role the environment name
implies. Building and testing then costs one keypress rather than a copy, a
rename and a click.

Idiomatic here rather than invented: MeshCore already post-processes builds with
merge-bin.py, create-uf2.py and arch/stm32/build_hex.py, so this is one more of
the same shape.

Environment names carry the board and the role, which is what makes the copy
unambiguous:

    Tbeam_SX1262_repeater          -> board Tbeam_SX1262, role simple_repeater
    Heltec_v3_companion_radio_usb  -> board Heltec_v3,    role companion_radio

Nothing here reaches the network and nothing is uploaded. If MeshBench is not
running the hand-over is skipped and the build still succeeds, because a build
should not fail because a simulator is closed.
"""
import os
import re

Import("env")  # noqa: F821  - provided by SCons, this is a PlatformIO script

# The role suffixes MeshCore publishes, longest first: an alternation matches the
# first branch that fits rather than the best one, and companion_radio_usb would
# otherwise match as companion.
ROLES = [
    ("companion_radio_ble", "companion_radio", "ble"),
    ("companion_radio_usb", "companion_radio", "usb"),
    ("companion_radio", "companion_radio", ""),
    ("room_server", "simple_room_server", ""),
    ("repeater", "simple_repeater", ""),
]


def split_env(name):
    """Board and role from an environment name, or None if it names no role."""
    for suffix, role, transport in ROLES:
        m = re.search(r"^(.*)_" + suffix + r"$", name, re.IGNORECASE)
        if m:
            return m.group(1), role, transport
    return None


def hand_over(path, board, role, version):
    """Give it to a running workbench, which files it in its own cache.

    Best effort: a build should not fail because a simulator is closed. The
    import verb already knows where images live and what a label looks like, so
    copying the file here as well would be a second opinion about a layout that
    is not ours.
    """
    # Through the client rather than by opening a socket here. This built the
    # path by hand - XDG_RUNTIME_DIR or /run/user/<uid> - which is a Linux
    # sentence, and os.getuid() does not exist on Windows at all: a firmware
    # developer building there hit an AttributeError before anything could
    # even fail to connect.
    try:
        from meshbench import Workbench
    except ImportError:
        # Not installed is a perfectly ordinary state for somebody who only
        # wanted to build firmware. pip install meshbench turns this on.
        return False
    try:
        with Workbench.attach() as wb:
            wb.call("firmware.import", {
                "path": path, "board": board, "role": role,
                "version": version})
        return True
    except Exception:  # noqa: BLE001 - a build must not fail over this
        return False


def after_build(source, target, env):
    name = env["PIOENV"]
    parts = split_env(name)
    if parts is None:
        print("meshbench: %s names no role, nothing to hand over" % name)
        return
    board, role, transport = parts

    # Prefer the merged image: a bare application has no bootloader and starts
    # nothing, which is a confusing thing to hand a simulator.
    build_dir = env.subst("$BUILD_DIR")
    candidates = ["firmware-merged.bin", "firmware.uf2", "firmware.bin"]
    src = next((os.path.join(build_dir, c) for c in candidates
                if os.path.exists(os.path.join(build_dir, c))), None)
    if src is None:
        print("meshbench: no image in %s" % build_dir)
        return

    # Named for the branch it came from, so two local builds are told apart in
    # the library rather than overwriting each other.
    version = "local-" + git_ref()

    if hand_over(src, board, role, version):
        print("meshbench: %s handed over as %s %s %s"
              % (os.path.basename(src), board, role, version))
    else:
        print("meshbench: workbench not running, nothing handed over")


def git_ref():
    """The current branch or short commit, for naming a local build."""
    for cmd in ("git rev-parse --abbrev-ref HEAD", "git rev-parse --short HEAD"):
        try:
            out = os.popen(cmd + " 2>/dev/null").read().strip()
            if out and out != "HEAD":
                return re.sub(r"[^A-Za-z0-9._-]", "-", out)
        except OSError:
            pass
    return "build"


env.AddPostAction("buildprog", after_build)  # noqa: F821
