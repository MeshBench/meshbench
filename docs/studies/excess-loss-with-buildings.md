# Buildings bought 0.7 dB of the excess-loss term, and cost a third of the links

Measured 2 September 2026 against ScotMesh: 451 imported nodes, a 24 hour
observation window, calculated RF mode, both arms in one session so they saw
the same nodes and very nearly the same observations.

Data: `excess-loss-with-buildings.json`

**Read `excess-loss-buildings-saturated.md` after this one.** The 0.70 dB
below was measured against a building rule that summed a knife edge and a wall
per crossed footprint with no combination rule, which is the fault this run
found and reported rather than fixed. Re-run against a rule that saturates,
the shrink is gone: the term moves by 0.04 dB the other way, and the
environment removes 27.0% of the matrix rather than 37.6%. Everything below stands as what was
measured on the night; the reading of it that survives is the one this run
gave itself, at the end of "The finding".

## The finding

`ExcessPathLossDB` exists because the bare-earth model has no buildings. Load
real footprints and the fitted term should shrink, and the size of the shrink
is what buildings bought. It shrinks by **0.70 dB**, from 29.77 dB to 29.07 dB,
which is 2.4% of the term.

Over the same import, at the same 25.1 dB starting term, the environment
removed **1,956 of 5,204 links**, 37.6% of the matrix, and took 24
observations out of the fit because their pair no longer has a measured link
at all. At each arm's own converged term the gap is 44. Buildings are doing a
great deal to the model. Almost none of it reaches the term.

The reason is where the two things act. The term is fitted on the median
residual of observations that were actually heard, and an observation was heard
because its path was open. Buildings price a path that crosses a town into the
ground, and a path priced into the ground is not one anybody reports hearing:
it leaves the link matrix, its observations become unmatched, and it stops
voting. What is left in the fit is the population buildings barely touched. So
the term measures what the model cannot explain about paths that work, and
adding the environment mostly removes paths from the fit rather than explaining
them.

## What was fitted against

**Footprints.** Microsoft Global ML Building Footprints, the 2026-02-03
release, resolved through the published `dataset-links.csv` index: 139 files at
level-9 quadkey granularity covering 54.0 to 59.2 N and 7.2 to 1.6 W, which is
Scotland plus the Northern Irish, Manx and northern English ground ScotMesh
nodes stand on. 476 MB gzipped, 4,487,597 features, 1.89 GB as GeoJSONL.

`go run ./tools/envgen -region uk` turned that into 41,458 z14 tiles, 527 MB,
with **0 features skipped**. The path index over the fleet's own bounding box
holds 2,753,713 of those buildings.

**The dataset publishes no heights in the United Kingdom.** Every feature
carries `"height": -1.0` and `"confidence": -1.0`, so every building takes
envgen's stated 6 m default at confidence 0.3, and every material falls to the
`uk` regional default. That is a real limit on how the result can be read: this
is a fit against 4.5 million 6 m boxes of default material, not against
surveyed building stock. An OSM merge would narrow it where OSM has surveyed a
building, and only there.

**Observations.** `validate.fetch` against the live CoreScope deployment for
the window, then `validate.calibrate`, repeated until the fit stopped moving,
which is the protocol the standing figure was reached by. Censoring is the same
in both arms because it lives in the code rather than in a parameter: a
prediction past the modem's +15 dB reporting ceiling says "at least this
optimistic", and a bound does not vote.

## The two arms

| | bare earth | buildings |
|---|---|---|
| links at 25.1 dB | 5,204 | 3,248 |
| converged term | **29.77 dB** | **29.07 dB** |
| rounds to converge | 2 | 2 |
| observations matched | 1,957 | 1,913 |
| censored, not voting | 637 | 611 |
| voting | 1,320 | 1,302 |
| unmatched | 135 | 180 |
| links at the converged term | 4,150 | 2,875 |

Both arms start from 25.1 dB, take one large step and one small one, then
repeat the same number to fourteen decimal places, which is what convergence
looks like here.

## Buildings really are priced

Not inferred from the term moving. Three independent readings:

- **envgen** reports 4,487,597 buildings into 41,458 tiles with nothing
  skipped, and the tile tree is 527 MB on disk.
- **The engine's own index**, opened over the fleet box, holds 2,753,713
  buildings. On `fixtures/fixture-scotland-strict.json`, of the 961 node pairs
  under 25 km, 921 cross at least one footprint and 641 are priced above
  0.01 dB. Buildings found in a box over Glasgow: 83,293; Edinburgh: 30,287;
  Dundee: 23,972; over Rannoch Moor: 108.
- **The link matrix changes**, by 1,956 links out of 5,204 at the same term,
  and the median residual moves with it.

Two earlier attempts at this measurement produced arms whose medians agreed to
sixteen significant figures. Neither was a null result; both were the harness
being fooled, and both faults are worth knowing about because they are silent:

- `rf.environment` persists the directory into the operator's preferences, so a
  later session opens with buildings already loaded and an arm labelled bare
  earth is nothing of the kind.
- The measured matrix is cached on disk under a fingerprint that did not
  include the environment, so a bare-earth session restored a building-priced
  matrix, found it already covered every pair, skipped the warm entirely and
  reported itself measured. That is fixed here: the environment is in the
  fingerprint and `matrixVersion` moves to 3, so no file written before it can
  be mistaken for either.

The run recorded here sets `rf.environment {on: false}` explicitly, and uses a
cache and a config of its own.

## What the environment does to a path, and why it is not a small effect

`environ.priceObstructions` charges a knife edge per rooftop the ray must clear,
plus one wall of material loss per building the ray passes through, and sums
them with no combination rule. A 23.1 km path across Glasgow crosses 114
footprints and prices at 2,235 dB. Across the fixture's sub-25 km pairs the
median priced loss is 123.7 dB and the 90th percentile 732 dB.

Terrain does not behave that way: `terrain.MultiEdgeLossDB` is a Bullington
construction with the P.526 correction, so a ridge line is one obstacle rather
than one per DEM sample. Buildings have no equivalent, so a town is charged
once per building rather than once as a town. The engine and the coverage
raster also disagree about it: coverage prices only the near-end footprints
(`PathLossNearEndsDB`), the engine prices every crossing, and
`internal/rf/environ/pathloss.go` opens by saying the two must charge a roof
the same. They do not.

None of that is fixed here, and it decides how the 0.70 dB should be read: the
environment is not making urban paths a little worse, it is removing them, and
a term fitted on what survives cannot see the difference. It has since been
fixed, and the re-fit in `excess-loss-buildings-saturated.md` shows the reading
was right: with a combination rule the 0.70 dB does not survive.

## What this does not settle

**The standing 25.1 dB does not reproduce on this night's data.** Bare earth,
same protocol, same censoring, same network: it converges to 29.77 dB, 4.7 dB
above the recorded figure. That is a difference in the observations rather than
in the arms. This fit used 1,320 voting observations where the recorded one
used 357, and it is stable in the window it was taken over:

| window | converged | matched | censored | voting |
|---|---|---|---|---|
| 6 hours | 29.775 dB | 529 | 165 | 364 |
| 24 hours | 29.765 dB | 1,958 | 637 | 1,321 |
| 72 hours | 29.765 dB | 1,959 | 638 | 1,321 |

One hundredth of a decibel between a window with 364 votes and one with 1,321.

The 72 hour row is not independent evidence: `validate.fetch` reads the last
4,000 packets, and a day of ScotMesh already fills that, so 24 and 72 hours ask
for the same traffic. Two windows on one evening agree; several evenings have
not been tried.

**The constant is left at 25.1 dB.** Moving it to either number measured here
would be wrong, for a different reason each way. 29.07 dB was fitted with an
environment loaded, and the constant is what a session with no environment
gets, so it would undercharge every bare-earth run, which are almost all of
them. 29.77 dB was fitted on one evening, where the figure it would replace was
reached over repeated rounds on more than one, and this measurement has just
shown how far the matched population can move underneath a fit. A network with
observations of its own already refines the term through `validate.fetch` then
`validate.calibrate`; what would justify moving the default is that protocol
repeated across several days, which is a measurement rather than a decision.
