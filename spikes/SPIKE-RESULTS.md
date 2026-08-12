# The pre-P0 spike, run

Section 20 of the redesign plan: the waterfall as a per-frame texture, and the
map stressed at real scale, on the actual 60Hz display and real hardware (AMD
RX 5700 XT, RADV, confirmed - not software rendering). Network is the shipped
`fixture-scotland-ireland-strict`: **311 nodes, 1,223 links** at the same
proximity rule the working Plan-view spike used. The plan's "~2,000 links" was
a guess made before this fixture was loaded; 1,223 is what the real geography
gives, and it is reported rather than adjusted to match.

A basemap was added before measuring anything: the first pass had none, and a
map that composites nothing under itself is not the real workload. It is a
48-tile checkerboard standing in for cached OSM tiles - the same shape of work
as `tiles.go`, sized for a half-day spike rather than a tile cache on disk.

Three modes, auto-cycled every 8 seconds so the numbers are reproducible
without an input tool, each measured over a clean window (the counter resets
at every mode switch, after the first run mixed frames from two modes into one
misleading reading):

| mode | what | fps, steady |
|---|---|---|
| 1 - naive | 1,223 separate stroked draw calls plus 311 separate filled circles, rebuilt every frame, over the basemap | **~24** |
| 3 - batched | the same scene as one stroked path and one filled path - two draw calls instead of 1,534 | **~35** |
| 2 - waterfall | a pre-rendered ring of 45 frames, so the number is pure texture-upload and composite cost, isolated from the CPU-side cost of generating a synthetic spectrogram | **~60**, the display ceiling |

## Reading it straight

**The waterfall clears without qualification.** Once the measurement is honest
about what it is testing - upload and composite, not trigonometry - it sits at
the monitor's own refresh rate. Section 5.5's "waterfall is a texture upload
and likely fine" was right.

**The map does not clear 60fps, even after one round of the obvious
optimisation.** Batching linked draw calls into one path is the standard first
move and it recovered about 50% of the shortfall - 24 to 35 - which is real
evidence Gio's rasteriser is not the wall. But 35 is not 60, and the plan's own
condition in section 20 was "if both hold." The map did not.

This does not mean stop. It means the map is a real risk line rather than a
cleared checkbox, and section 10.14's frame budget stays open into P3 with two
specific, untried levers rather than a vague "make it faster":

- **Viewport culling.** This spike draws every link in the network regardless
  of whether it is on screen. The real map, at a normal zoom, would not - most
  of a national network is off the visible edge at once. Untested here on
  purpose: it is a P3 question, not a pre-P0 one.
- **Path caching across static frames.** Most frames in the real application
  the camera has not moved and the network has not changed. A cached compiled
  path, reissued only when something invalidates it, turns most frames into a
  cheap redraw rather than 1,223 fresh strokes. This spike rebuilds the path
  every frame on purpose, to measure the worst case.

Both are ordinary engineering, not toolkit risk. Neither was in scope for a
half-day spike whose job was to answer "is the rasteriser the wall" - and the
batched-versus-naive delta answers that question: no, it is not close to being
the wall on its own, there is real headroom left by culling and caching that
this spike deliberately did not spend.

## What changes in the plan

- **10.14 stays open**, reworded from "measured" to "measured, with culling and
  path caching as the two levers if 3.stress-test in P3 does not clear 60fps
  outright" - because it will not, on this evidence, without them.
- **Section 20's gate is met on the waterfall and open on the map.** P0 starts
  regardless: the map risk is real but bounded, with two named, ordinary fixes
  rather than an open question about whether Gio can do this at all.
- **A P3 sub-task added**: implement viewport culling and static-frame path
  caching before the frame-budget test in 13.5 is judged pass or fail. Without
  them, 13.5 would fail on the evidence above.
