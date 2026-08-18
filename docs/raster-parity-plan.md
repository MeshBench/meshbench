# Raster parity plan: what HopReach's map does that ours does not

Measured against the HopReach source (`~/Documents/projects/mccoverage`,
the project behind the ScotMesh coverage map), not its screenshots. Its
raster stack earned its readability through specific decisions; this is
the list, what we already have, and the order to close the rest.

## The gap, item by item

| HopReach | MeshBench today | Verdict |
|---|---|---|
| Opacity slider on the legend, 0-100%, live | Alpha baked into the raster image, fixed | **P0** - draw-time `paint.PushOpacity`, slider beside the layer panel |
| Continuous orange-to-green ramp by margin (transparent below the floor) | Four discrete colour bands | **P0** - continuous ramp, one-way cells keep their amber, legend becomes a gradient bar |
| Nearest-neighbour scaling when zoomed, so real cells read crisp | Linear filtering smears cells into gradients | **P0** - `paint.FilterNearest` on the coverage image |
| Bounded generation (per-region rasters) | Boundary or network box only | **P0** - "raster this view" button: the current viewport becomes the box |
| Precision tier: 6000 px wide raster | 240 default, 1024 cap | **P1** - cap to 4096; the viewport button makes big widths meaningful |
| Supersample 2x then downsample, so edges anti-alias | Raw cells | **P1** - supersample the viewport tier |
| Precision DEM at z13 (~10.5 m/px) under the finer raster | Terrain store is single-zoom z12 (~30 m) | **Follow-up** - honest note meanwhile: raster detail beyond ~30 m/px is interpolation, not information |
| `max_range_km` station cull | GPU prices every station over every cell | **P1** - skip stations that cannot reach the box |
| Calibrated-position second dataset | No calibration source in the workbench | Not planned |
| Per-region rasters per scope | Different product shape | Not planned |

## Design notes

- **Opacity at draw time, not bake time.** HopReach re-alphas the PNG in
  the client; Gio gives us `paint.PushOpacity`, so one slider changes the
  layer live with no repaint of the raster itself.
- **The ramp keeps its honesty.** HopReach's orange-to-green is margin
  0..green-threshold; ours will run the same span but one-way cells stay
  amber (their asymmetry is a different fact, not a weaker margin) and
  no-data stays transparent, never coloured.
- **The viewport button changes what "resolution" buys.** 4096 cells over
  a nation is still ~500 m/cell; 4096 over one glen is finer than the DEM.
  The button plus the existing `coverage.resolution` knob covers both ends,
  and the legend should say the cell size in metres so nobody has to guess.
- **DEM z13 is the real precision unlock** and touches the terrain store's
  single-zoom assumption; it is its own piece of work, recorded, not
  smuggled in here.

## Why a "small" viewport raster is slow, and the GPU plan

A viewport is smaller in area, not in cells: the same box at 1024 cells
is ~960k cells against 45k for the national run at 240 - twenty-one
times the work, paid per station. The resolution knob is the bill.

What the buildings-priced viewport run taught, each step verified live:
the obstruction query's bounding box froze it outright (fixed:
corridor, then a job-level bucket index); the same town was re-tested
per cell (fixed: per-station azimuth sectors, equivalence-tested);
mid-path roofs price at a decibel across tens of km (bounded: near-end
pricing, exact where it prices); free space cannot cull LoRa stations
(fixed: the horizon bulge itself is the knife edge - ~330 m at 150 km -
so range culling is physics, not a config number); and the per-station
CPU margin pass was single-threaded (fixed: parallel rows).

**The next real step is finishing the fold on the GPU.** Today the
kernel prices losses and everything after - margins, buildings, fold -
is CPU per station, with a readback per station. The plan: the kernel
takes the station's budget terms and writes min-margin per cell; a
second small kernel folds it into a persistent best/serving buffer on
the device; buildings stay CPU but apply only to fold survivors near
towns; one readback at the end instead of hundreds. That removes the
per-station CPU passes entirely and should put a 1M-cell viewport
under a minute.

## Order of work

1. P0 visual set: opacity slider, continuous ramp, nearest filtering,
   cell-size line on the legend.
2. P0 viewport raster button, via the same `coverage.map` verb taking an
   optional box.
3. P1: resolution cap 4096, supersampled viewport tier, station range
   cull.
4. Follow-up: z13 precision DEM tier.
