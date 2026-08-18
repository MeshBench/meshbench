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

## Order of work

1. P0 visual set: opacity slider, continuous ramp, nearest filtering,
   cell-size line on the legend.
2. P0 viewport raster button, via the same `coverage.map` verb taking an
   optional box.
3. P1: resolution cap 4096, supersampled viewport tier, station range
   cull.
4. Follow-up: z13 precision DEM tier.
