> **Working note, last true on 19 August 2026.** Kept for the thinking in it, not maintained as a description of the code. Where this disagrees with the tree, the tree is right; the authority is the decision itself, which stands.

# ADR-0019: a headless mode, rather than a virtual display

**Status:** accepted, 12 August 2026
**Context:**

## The question

Both testing workstreams need to drive MeshBench from CI, and MeshBench is a
desktop application that wants a display and a GPU. MeshCore ships a
`.devcontainer`, so firmware developers will meet the same wall.

Two options were spiked: run the real binary under Xvfb with software GL, or
give the application a headless mode that serves the same control-socket verbs
without a renderer.

## What the spike found

**Xvfb is not the obstacle.** Xvfb on this machine advertises GLX and Mesa
reports direct rendering through llvmpipe:

    $ DISPLAY=:77 glxinfo -B
    direct rendering: Yes
    Vendor: Mesa (0xffffffff)

**The application starts under it and binds its control socket**, so it gets a
long way. But no verb answered: `session.describe`, `nodes.list` and
`firmware.state` all returned nothing, across repeated attempts and after
waiting. The only line in its log is `Glfw Error 65538: Cannot set swap interval
without a current OpenGL or OpenGL ES context`, which also appears on the
working desktop application and is therefore not the fault.

The cause was not isolated. That is worth saying plainly rather than implying a
verdict the spike did not reach.

## The decision

**Build a headless mode.** Not because Xvfb failed, but because of what the
failure exposed.

**Control verbs are serviced on the frame thread.** That is already documented
behaviour with consequences we have hit: console replies come back empty while a
sweep is driving the engine, because the experiment owns the clock. A CI harness
that drives verbs is therefore hostage to the renderer, and the spike is exactly
that failure mode wearing a different hat: socket bound, frames not running,
verbs silent.

Putting a virtual display underneath does not remove that coupling. It hides it,
and it hides it in the environment where debugging is hardest, which is somebody
else's CI runner at three in the morning.

Three further reasons, in order of how much they matter:

1. **The devcontainer case is the same case.** MeshCore ships one. A firmware
   developer who wants to test a change should not need a display server inside
   their container.
2. **A renderer on a CI runner is pure cost.** llvmpipe drawing frames nobody
   will ever see, on a machine already paying about a core per emulated node.
3. **It is a smaller surface to keep working.** A headless mode exercises the
   engine and the verbs. The Xvfb path additionally depends on Xvfb, Mesa, GLX
   and GLFW agreeing, on every distribution we claim to support.

## What this does not mean

It does not mean the GUI stops being the primary interface. ADR-0005 stands:
this is a native desktop application, not a service. Headless is a second entry
point to the same verbs, not a second product.

It does not mean the Xvfb result is closed. If somebody wants screenshots in CI
later, that path is worth finishing, and the spike script is kept for it.

## Consequences

- The verb layer must be usable without a frame loop. Today it is not, and that
  is the work.
- Every verb needs to behave identically in both modes, or the harness tests
  something users never run. A shared test that drives the same verbs both ways
  is the cheapest guard.
- The scripts from the spike stay at `tools/headless/` so the next person does
  not repeat it.

## Alternative, for the record

Finish the Xvfb path: isolate why verbs do not answer, pin Mesa and Xvfb
versions in CI, and accept the renderer as a dependency of testing. Cheaper this
week, and it leaves the frame-thread coupling in place for everything that comes
after.
