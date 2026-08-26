> **Working note, last true on 12 August 2026.** Kept for the thinking in it, not maintained as a description of the code. Where this disagrees with the tree, the tree is right; the authority is the wb2 design-language skill, and `internal/ui/`.

# The Gio workbench

A second interface to the same simulator, built to replace the imgui one. Both
ship. Nothing is deleted until the two have been compared feature by feature,
and that comparison is the reason both are in the same build.

    meshbench-workbench -fixture fixtures/fixture-scotland-ireland-strict.json

## Why there are two

The imgui workbench docks everything, because docking was the only arrangement
it had. A dock is the right answer when two things are read together and the
wrong answer when one of them wants to be on the other monitor. In this one a
panel that wants to be a window *is* one: its own frame loop, its own size, the
compositor's own decorations, and enough width to show columns the docked
version had to clip.

## How it is put together

**One goroutine owns the world.** Verbs are messages to it; the renderer reads
an immutable snapshot and never the world. That is why two windows drawing one
network need no coordination beyond both reading the same value, and why the
engine advances on its own ticker rather than on frames - a headless run and a
watched run are the same run.

**Every gesture is a verb.** Clicking a node, dragging it, asking for coverage:
each goes through the same named action a script would use. The shortcut sheet
lists those names for that reason.

**The renderer knows nothing about radio.** Margins, coverage rasters and
hillshades are computed in the store and arrive as values or as images. A
renderer that has to know what a decibel is will eventually disagree with the
panel printing the number.

## What it does that the old one does not

- Links drawn by the margin the engine measured, in three bands, with a mark on
  links more than 3 dB lopsided - the case where a mast is heard by a handheld
  it cannot answer.
- Labels placed so they do not overlap, in priority order, stably across frames.
- A hillshade of the same DEM the path losses are cut against, so the elevation
  attribution is true of the picture as well as of the answer.
- Panels in real windows.
- A sweep over arms and seeds, drawn as a matrix, which answers "did that
  survive reseeding" at a glance.

## Two things it is honest about

**A map that cannot draw everything says so.** With no terrain tiles cached the
model has no profile to cut a path against, so links close at distances they
would not over real ground - 12,924 of them on the shipped fixture. The map
draws the strongest 2,500 and says how many of how many, because a map showing a
fraction of the links without saying so is worse than a slow one.

**No data and no result are different claims.** A coverage cell the terrain
could not answer for is transparent and counted, not drawn as absence. A sweep
cell that was not run is NaN, not zero. An empty waterfall says which of the
several reasons it is.

## Comparing the two

Run them side by side, panel by panel, and check each one does what it has to.
What is worth checking by hand
rather than by screenshot: whether menus and view tabs respond, whether dragging
a node moves it, whether a popped-out window keeps drawing while the main one
is busy. Rendering correctly and responding correctly are different claims, and
only the first can be checked from a screenshot.

## Flags worth knowing

    -view run            open a view directly
    -play                start the simulation immediately
    -pop-out Scoreboard  open a panel in its own window
    -panel Map           draw only that panel, filling the window
    -fps                 report frames per second and the cost of each frame

The last one exists because "it feels slow" is not a measurement. It reports the
time this process spends building and submitting a frame, which is the number
the 16.7 ms budget is about, rather than delivered frames per second, which is
capped by the display and shaped by the compositor.
