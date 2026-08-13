# Workbench 2: the performance plan

All nine items have landed and been measured; the numbers are inline below.

Written after the GPU warm went wrong three ways in one evening, and after the
question that reframed it: why was workbench 1 never slow? The answer is the
plan. Workbench 1 is fast because it never does work it can avoid: its warm
walks node-to-node profiles over tiles already hot in memory, a free-space cull
skips most pairs before any terrain is read, and nothing it computes reaches
past the ground its profiles actually cross. Workbench 2 got slow wherever it
departed from that shape. So the plan is mostly to stop departing.

## Landed

1. **Warm on every rebuild, visibly.** The stall-on-first-send was an unwarmed
   cache paying for the whole matrix on whichever thread sent the first
   packet. A rebuild now marks the engine cold, the tick warms it, play
   declines to start until it is done, and the button says so with a flame and
   a percentage.

2. **The link matrix survives rebuilds that do not change it.** Reset, a new
   seed and a run-kind switch remake the engine but move no node, so the
   matrix they re-measured was identical. Everything a path loss depends on -
   positions, heights, powers, frequency, excess loss - is fingerprinted, and
   an unchanged fingerprint carries the measured cache into the new engine.
   The commonest rebuilds now warm in nothing.

3. **Rasterisation is parallel and reports progress.** One core did 67
   million elevation lookups while eleven watched, silently.

4. **The GPU speaks when it declines**, sizes its grid to what the card will
   actually bind (the device's limits, not the adapter's advertisement), and
   the two stacked progress throttles that hid the first 32,000 pairs of CPU
   progress are one throttle.

5. **The bounding-box grid is dead; profiles on the tiles, maths on the
   GPU.** Free-space cull first, profiles for the survivors gathered from
   the hot TileStore on every core, only the packed profiles shipped to the
   kernel. With the 10 GB tile cache holding the working set, the 311-node
   fixture's warm measured 8.2 seconds end to end on elite - 48,205 pairs,
   down from nine minutes.

6. **The matrix persists across sessions.** Saved under its geometry
   fingerprint when a warm completes, loaded before anything warms, pruned
   at 24. Measured on elite: the first launch of the national fixture warms
   in 8 seconds, every launch after it has its 4,086 links 1.1 seconds after
   start with no warm at all.

7. **Tile prefetch for the study area, said out loud.** terrain.prefetch
   estimates first - "fetching 412 of 500 tiles, roughly 25 MB" - then runs
   as a job in the strip. On elite the 311 fixture answers instantly:
   13,965 tiles, all cached.

8. **Event log and trails, incremental.** EventsSince binary-searches from a
   timestamp and EventsTail copies only the tail, so ticks no longer walk
   the whole log as a run ages.

9. **The tick, measured under 311 firmware processes.** 30.0 simulated
   seconds in 30.0 wall seconds - 1.00x real time with 309 processes up -
   so there is nothing to profile: the engine keeps pace exactly.

## The rule that comes out of tonight

Every long operation must either announce itself in the jobs strip or not be
long. A wait with nothing on screen was reported as a crash three separate
times tonight, and each time the work was healthy - the application just had
not said so.
