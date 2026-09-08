"""Checks for the parts of the sweep that decide things.

Run with `python3 -m pytest tools/shots/shots_test.py`. Not the capture
itself - that needs a window - but the two judgements that failed whole
platforms: what counts as a refusal, and where the binary is.
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import shots  # noqa: E402


def write(tmp_path, name, text):
    p = tmp_path / name
    p.write_text(text)
    return str(p)


def test_a_gpu_warning_is_not_a_refusal(tmp_path):
    """A driver wgpu dislikes made it chatty, and every one of those lines was
    read as a refusal - so on a machine with an older Intel driver all 117
    steps failed, each reported as "the workbench refused something", while the
    window was up and perfectly usable."""
    noisy = write(tmp_path, "warn.txt", "\n".join([
        "session log: /tmp/x.log",
        "[wgpu] [Warn] Disabling robustBufferAccess2: IntegratedGpu Intel Driver is outdated",
        "[wgpu] [Warn] Device creation failed: 0x887A0004",
        "xkbcommon: ERROR: unrecognized keysym \"dead_hamza\"",
    ]))
    assert shots.tail(noisy) == ""


def test_a_real_refusal_still_comes_through(tmp_path):
    """The check exists because three board-view steps reported green for
    months on a picture of a plain workbench. Filtering noise must not filter
    the thing it was added for."""
    refused = write(tmp_path, "refuse.txt", "\n".join([
        "session log: /tmp/x.log",
        '-panel "Energy": no such panel. There is: Boards, Map',
    ]))
    assert "no such panel" in shots.tail(refused)


def test_the_binary_is_found_under_the_name_this_platform_builds():
    """go build -o meshbench produces meshbench.exe on Windows, and the path
    checked had no extension - so the script refused before doing anything,
    printing the build instruction that had just been followed."""
    got = shots.defaultBinary()
    assert os.path.basename(got) in ("meshbench", "meshbench.exe")


def test_every_step_has_a_name_and_something_to_judge_it_by():
    """A step with no expectation is a screenshot nobody can judge."""
    import json
    steps = json.load(open(shots.STEPS))
    assert steps, "no buckets at all"
    for bucket, items in steps.items():
        for s in items:
            assert s.get("name"), f"{bucket} has a step with no name"
            assert s.get("expect") or s.get("needs"), \
                f'{bucket}/{s["name"]} has nothing to judge it by'


def png(tmp_path, name, w, h):
    """Just the header: the size check reads nothing past IHDR."""
    p = tmp_path / name
    p.write_bytes(b"\x89PNG\r\n\x1a\n" + (13).to_bytes(4, "big") + b"IHDR"
                  + w.to_bytes(4, "big") + h.to_bytes(4, "big") + b"\x08\x06\x00\x00\x00")
    return str(p)


def test_a_popout_the_size_of_the_workbench_is_the_workbench(tmp_path):
    """The macOS chooser skipped every floating window of ours and fell
    through to the workbench behind it, so 46 of 115 steps filed the main
    window under a popout's or a node window's name, and the sweep said
    complete. A picture of another window cannot be the workbench's size."""
    step = {"name": "popout-events", "flags": ["-pop-out", "Events"]}
    out = png(tmp_path, "popout-events.png", 1500, 963)
    why = shots.wrongWindow(step, out, (1500, 963))
    assert why and "workbench" in why


def test_a_popout_of_its_own_size_passes(tmp_path):
    step = {"name": "node-console", "flags": ["-node-window", "A", "-node-tab", "Console"]}
    out = png(tmp_path, "node-console.png", 820, 652)
    assert shots.wrongWindow(step, out, (1500, 963)) is None


def test_a_step_about_the_workbench_may_be_its_size(tmp_path):
    step = {"name": "panel-events", "flags": ["-panel", "Events"]}
    out = png(tmp_path, "panel-events.png", 1500, 963)
    assert shots.wrongWindow(step, out, (1500, 963)) is None


def test_the_check_needs_a_measured_workbench(tmp_path):
    """No reference, no verdict: the sweep measures the workbench first and
    refuses to start without it, rather than guessing a size."""
    step = {"name": "popout-events", "flags": ["-pop-out", "Events"]}
    out = png(tmp_path, "popout-events.png", 1500, 963)
    assert shots.wrongWindow(step, out, None) is None
