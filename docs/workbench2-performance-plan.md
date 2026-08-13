# Workbench 2: the performance plan

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

## Next, in order

5. **Kill the bounding-box grid; profile on the tiles, maths on the GPU.**
   The grid is the wrong shape: it rasterises ground between nodes that no
   profile crosses, which is why it downloaded tiles workbench 1 never needed
   and why a country needs a grid too big to bind. Replace it: run the
   free-space cull first (as the CPU warm already does), gather profiles for
   the surviving pairs from the same hot TileStore the CPU uses - all cores,
   no new downloads, byte-identical heights to the CPU twin - and ship only
   the packed profiles to the kernel for the Bullington maths. Kills the
   grid cache, the cell-size refusals and the binding limits in one move,
   and works at any span.

6. **Persist the matrix across sessions.** The fingerprint from (2) is also a
   file name. A fixture opened twice on the same machine with the same
   calibration should warm from disk in milliseconds, exactly as terrain
   tiles already persist. Invalidation is the fingerprint, so a moved node or
   a changed excess loss misses cleanly.

7. **Tile prefetch for the study area, said out loud.** First contact with a
   new fixture downloads tiles; today that cost surfaces wherever the first
   profile happens to need them. Prefetch the boundary box through the
   existing TileStore.Prefetch with its progress in the jobs strip, so the
   network cost is paid once, visibly, up front.

8. **Event log and trails, incremental.** The tick rebuilds trails by
   filtering the whole event log and copies the tail for the tables; both
   walk the full slice every tick, so ticks slow as a run ages. Keep a ring
   for the trail window and publish deltas.

9. **Profile the tick under 311 firmware processes.** After (5)-(8) the
   remaining cost is the engine's own step: measure where a tick goes on the
   big fixture - pprof, on elite - and take what it says. No optimisation
   before measurement; the last three "obvious" causes tonight were each
   wrong until run.

## The rule that comes out of tonight

Every long operation must either announce itself in the jobs strip or not be
long. A wait with nothing on screen was reported as a crash three separate
times tonight, and each time the work was healthy - the application just had
not said so.
