# A saturating building rule buys nothing off the excess-loss term, and the 0.7 dB was the deletions

Measured 2 September 2026 against ScotMesh: 455 imported nodes, a 24 hour
observation window, calculated RF mode, both arms in one session so they saw
the same nodes and the same observations.

Data: `excess-loss-buildings-saturated.json`. It re-runs the protocol of
`excess-loss-with-buildings.md` against a building model that saturates
instead of accumulating, and the two should be read together: that run
measured the fit, found the pricing bug, and said in as many words that the
0.70 dB it reported could not be separated from what the bug deleted.

## The model that changed

A crossed footprint used to cost a knife edge plus a wall, summed with no
combination rule, so a 23.1 km path across Glasgow crossing 114 buildings
priced at 2,235 dB. It now prices as one obstacle: the leading rooftop takes
its full ITU-R P.526 knife edge, the rows behind it add 18 log10 of their
number, which is the settled field Walfisch and Bertoni derived and the COST
231 multi-screen term carries, and the walls are the alternative route through
rather than over, with the interior depth charged as well so a tower block does
not cost only what its own two walls cost, combined in power rather than added.
Rooftops are priced within 3 km of either end, which is where a few metres of
building carry Fresnel weight; anything over 20 m is priced wherever it stands,
because that argument is about houses. Making the aperture a rule of the price
rather than of one caller's search is what stops the engine and the coverage
raster answering differently. They now agree to zero decibels across all 961
sub-25 km pairs of the fixture, where the worst pair used to disagree by
2.2 dB.

## The finding

**Buildings buy nothing.** The term went from 29.475 dB bare earth to
29.515 dB with footprints loaded: it grew by 0.041 dB. The previous run,
under the unbounded rule, measured a 0.70 dB shrink.

**And they delete a quarter of the matrix rather than a third.** At the same
25.1 dB starting term the environment removed 1,420 of 5,264 links, 27.0%,
against 1,956 of 5,204 and 37.6% before.

So the direction the first run guessed at is the direction this one found, and
further than it guessed: the 0.70 dB shrink was not buildings explaining
paths, it was buildings deleting them. Take the deletions away and the shrink
goes with them.

## The two arms

| | bare earth | buildings |
|---|---|---|
| links at 25.1 dB | 5,264 | 3,844 |
| converged term | **29.475 dB** | **29.515 dB** |
| rounds to converge | 3 | 2 |
| observations matched, first round | 1,874 | 1,864 |
| censored, not voting, first round | 723 | 615 |
| voting, first round | 1,151 | 1,249 |
| unmatched, first round | 131 | 141 |
| links at the converged term | 4,246 | 3,252 |

## Why the term grew rather than shrank

The 0.041 dB is not a shrink turned inside out, it is the censoring moving.

A prediction past the modem's +15 dB reporting ceiling says "at least this
optimistic" and does not vote. Bare earth, 723 of 1,874 matched observations
sat above that ceiling. Loading buildings brought 108 of them back under it,
so the voting population grew from 1,151 to 1,249 rather than shrinking. What
came back in are paths the model was most optimistic about, and they arrive
carrying large positive residuals, which pulls the median up by more than the
building loss pulls it down. Net, 0.04 dB.

That is a better outcome than a shrink would have been. Under the old rule an
urban path left the matrix and stopped saying anything; under this one it
stays, stops being censored, and votes.

## What a path costs now

Over `fixtures/fixture-scotland-strict.json`, against the same 2,753,713
buildings in the fleet box:

| | before | now |
|---|---|---|
| pairs under 25 km priced above 0.01 dB | 641 | 613 |
| median priced loss | 123.7 dB | 27.8 dB |
| 90th percentile | 732.1 dB | 46.5 dB |
| worst | 2,234.9 dB | 57.4 dB |
| worst engine/coverage disagreement | 2.2 dB | 0.0 dB |

The 23.1 km Glasgow path that priced at 2,234.9 dB prices at 33.9 dB. Seven of
its 114 crossings fall inside the aperture, so on that path it is the aperture
rather than the saturation doing most of the work; on the fixture's dense
short pairs, where crossings cluster at both ends, it is the other way round.

## What this does not settle

**One night, one network, as before.** The bare-earth arm converged to
29.475 dB where the previous evening's converged to 29.775 dB, on the same
protocol against the same deployment, so 0.3 dB is the run-to-run spread of
this measurement and neither number is the constant. The default stays at
25.1 dB for the reasons the earlier study gives.

**Every building is still 6 m.** Microsoft's footprints publish no height in
the United Kingdom, so the leading rooftop's knife edge is computed against a
default rather than a survey. The multi-screen term counts rows and does not
ask how tall they are, which is why it was chosen, but the term that decides
whether a town shadows a path at all still rests on that 6 m.

**The aperture is a modelling choice, not a measurement.** Three kilometres is
argued from Fresnel weight and from needing the raster and the engine to
charge the same crossings, and 20 m is where a footprint stops being a rooftop
and starts being an obstacle wherever it stands. Nothing here measures what a
town of ordinary houses mid-way across a long path really costs, and the model
now says it costs nothing. On this dataset that exemption is untested in the
other direction too: with no published heights, nothing in Scotland is over
20 m.
