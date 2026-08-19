// The whole network's coverage, computed directly.
//
// The first attempt rasterised every station over the shared grid and
// combined the results: N full rasters for an answer that only wants the
// best of them per cell. On a national network that is hours. This walks
// the cells once, tries each cell's nearest stations first, and stops the
// moment free-space arithmetic proves nobody farther could serve the cell
// or beat its best - which is almost immediately, almost everywhere.
package coverage

import (
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/geo"
	"github.com/MeshBench/meshbench/internal/propagation"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// gridTerrain lets the sampled height grid stand in for the tile store, so
// a profile walk costs bilinear reads instead of tile lookups.
type gridTerrain struct{ g propagation.HeightGrid }

func (t gridTerrain) ElevationM(lat, lon float64) (float64, bool) { return t.g.At(lat, lon) }

// gainSlackDBi bounds how much a directional antenna could add over the
// free-space cull's assumption. Generous on purpose: the cull may only skip
// stations that could not serve the cell under any pointing.
const gainSlackDBi = 8

// BestServer fills r with each cell's best two-way link from any station,
// and returns the Combined statistics over the same pass. extraLossDB, when
// not nil, prices whatever else stands on the path - buildings - and is
// called only for the paths that survive the free-space cull. progress is
// called per finished row.
func BestServer(g propagation.HeightGrid, stations []Endpoint, r *Raster, o Options,
	extraLossDB func(station int, cellLat, cellLon, txAslM, rxAslM, distM float64) float64,
	progress func(done, total int)) *Combined {
	if o.ProfileStepM <= 0 {
		o.ProfileStepM = 30
	}
	n := r.Width * r.Height
	r.Cells = make([]Cell, n)
	c := &Combined{
		Raster: Raster{South: r.South, North: r.North, West: r.West, East: r.East,
			Width: r.Width, Height: r.Height, FreqMHz: r.FreqMHz},
		BestMarginDB: make([]float64, n),
		BestNode:     make([]int, n),
		ServingCount: make([]int, n),
	}
	c.Cells = r.Cells

	grounds := make([]float64, len(stations))
	groundOK := make([]bool, len(stations))
	for i, st := range stations {
		grounds[i], groundOK[i] = g.At(st.Lat, st.Lon)
	}
	gt := gridTerrain{g}

	var done atomic.Int64
	var wg sync.WaitGroup
	rows := make(chan int)
	workers := runtime.NumCPU()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Scratch per worker: distances and order, reused per cell.
			type cand struct {
				i  int
				km float64
			}
			cands := make([]cand, 0, len(stations))
			for y := range rows {
				for x := 0; x < r.Width; x++ {
					idx := y*r.Width + x
					lat, lon := r.LatLonAt(x, y)
					remoteGround, ok := g.At(lat, lon)
					if !ok {
						r.Cells[idx] = Cell{NoData: true}
						c.BestMarginDB[idx] = math.NaN()
						c.BestNode[idx] = -1
						continue
					}
					cands = cands[:0]
					for i, st := range stations {
						if !groundOK[i] {
							continue
						}
						cands = append(cands, cand{i, geo.DistanceKm(st.Lat, st.Lon, lat, lon)})
					}
					// Nearest first; insertion sort - the list is small and
					// mostly the same order cell to cell.
					for i := 1; i < len(cands); i++ {
						for j := i; j > 0 && cands[j].km < cands[j-1].km; j-- {
							cands[j], cands[j-1] = cands[j-1], cands[j]
						}
					}
					best := Cell{NoData: true}
					bestIdx, bestMin := -1, math.Inf(-1)
					serving := 0
					for rank, cd := range cands {
						st := stations[cd.i]
						if cd.km <= 0 {
							cell := Cell{}
							if 0 > bestMin {
								best, bestIdx, bestMin = cell, cd.i, 0
							}
							serving++
							continue
						}
						fspl := terrain.FSPLdB(cd.km, r.FreqMHz)
						optOut := st.TxPowerDBm + gainSlackDBi + o.RemoteGainDBi - fspl - o.RemoteSensitivityDBm
						optIn := o.RemoteTxPowerDBm + o.RemoteGainDBi + gainSlackDBi - fspl - st.SensitivityDBm
						opt := math.Min(optOut, optIn)
						// The cull: free space is the best any path can do,
						// and the list is nearest-first, so once even free
						// space cannot serve this cell or beat its best,
						// nobody farther can either. The nearest station is
						// always evaluated, so a gap cell still carries an
						// honest negative margin rather than silence.
						if rank > 0 && opt < 0 && opt < bestMin {
							break
						}
						profile, okP := sampleProfile(gt, st.Lat, st.Lon, lat, lon, cd.km, o.ProfileStepM)
						if !okP {
							continue
						}
						loss := fspl + terrain.MultiEdgeLossDB(profile, st.HeightAGLm, o.RemoteHeightAGLm, r.FreqMHz)
						if extraLossDB != nil {
							loss += extraLossDB(cd.i, lat, lon,
								grounds[cd.i]+st.HeightAGLm, remoteGround+o.RemoteHeightAGLm,
								cd.km*1000)
						}
						cell := cellFromLoss(st, grounds[cd.i], remoteGround, lat, lon, cd.km, loss, o)
						if cell.Workable() {
							serving++
						}
						m := math.Min(cell.OutboundMarginDB, cell.InboundMarginDB)
						if m > bestMin {
							best, bestIdx, bestMin = cell, cd.i, m
						}
					}
					r.Cells[idx] = best
					c.BestNode[idx] = bestIdx
					c.ServingCount[idx] = serving
					if bestIdx < 0 {
						c.BestMarginDB[idx] = math.NaN()
					} else {
						c.BestMarginDB[idx] = bestMin
					}
				}
				if progress != nil {
					progress(int(done.Add(1)), r.Height)
				}
			}
		}()
	}
	for y := 0; y < r.Height; y++ {
		rows <- y
	}
	close(rows)
	wg.Wait()
	return c
}
