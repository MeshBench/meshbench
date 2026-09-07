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
