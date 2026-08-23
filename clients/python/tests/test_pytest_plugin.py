"""The plugin, exercised as pytest will exercise it.

pytester runs a real pytest in a subdirectory, which is the only way to check
that a fixture is discovered rather than merely importable - and being
discovered without a conftest is the whole point of shipping it as an entry
point.
"""

from __future__ import annotations

import os
import shutil

import pytest

pytest_plugins = ["pytester"]


@pytest.fixture(autouse=True)
def _needs_binary():
    if not (os.environ.get("MESHBENCH_BINARY") or shutil.which("meshcoresim")):
        pytest.skip("no meshcoresim binary")


def test_the_meshbench_fixture_is_discovered(pytester: pytest.Pytester):
    pytester.makepyfile(
        """
        def test_a_mesh_is_there(meshbench):
            meshbench.project.open("fife-strict")
            assert len(meshbench.nodes) == 58
            assert meshbench.is_headless
        """
    )
    result = pytester.runpytest_subprocess(
        # No -p: the entry point is what has to work. Registering it by hand
        # would test that the module imports, which is not the claim.
        f"--meshbench-binary={os.environ.get('MESHBENCH_BINARY', 'meshcoresim')}",
    )
    result.assert_outcomes(passed=1)


def test_the_scenario_is_cleared_between_tests(pytester: pytest.Pytester):
    """Session-scoped reuse must not leak one test's network into the next.

    Two tests, the first of which places a node. The second must not see it.
    """
    pytester.makepyfile(
        """
        def test_places_one(meshbench):
            meshbench.project.new()
            meshbench.nodes.place("LeftBehind", lat=56, lon=-3)
            assert "LeftBehind" in meshbench.nodes

        def test_does_not_see_it(meshbench):
            assert "LeftBehind" not in meshbench.nodes
        """
    )
    result = pytester.runpytest_subprocess(
        # No -p: the entry point is what has to work. Registering it by hand
        # would test that the module imports, which is not the claim.
        f"--meshbench-binary={os.environ.get('MESHBENCH_BINARY', 'meshcoresim')}",
    )
    result.assert_outcomes(passed=2)


def test_a_failure_carries_the_provenance(pytester: pytest.Pytester):
    """The honesty rule, where it matters most.

    Somebody reading a failed assertion about a mesh is deciding whether their
    firmware change broke something. They need to know the run had no
    multipath, no body loss and no oscillator error before concluding anything,
    and they must not have to have asked for that.
    """
    pytester.makepyfile(
        """
        def test_deliberately_fails(meshbench):
            meshbench.project.open("fife-strict")
            assert len(meshbench.nodes) == 1, "not really"
        """
    )
    result = pytester.runpytest_subprocess(
        # No -p: the entry point is what has to work. Registering it by hand
        # would test that the module imports, which is not the claim.
        f"--meshbench-binary={os.environ.get('MESHBENCH_BINARY', 'meshcoresim')}",
    )
    result.assert_outcomes(failed=1)
    result.stdout.fnmatch_lines(["*best case*"])
