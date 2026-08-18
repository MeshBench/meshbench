// The whole-map raster on the GPU.
//
// The same shape the link warm uses: the operator's one GPU switch decides,
// the device is opened for the job and closed after, and the CPU pass
// remains the twin that runs everywhere else. Per station the kernel prices
// the whole grid at once; the margins, the buildings and the best-server
// fold stay on the CPU, where the arithmetic is shared with every other
// consumer.
package session

import (
	"fmt"
	"github.com/MeshBench/meshbench/internal/terrain"
	"math"
	"runtime"
	"sync"

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/gpu"
)

// coverageMapGPU prices every station's grid on the device and folds the
// results. Returns false when there is no device to be had, and the caller
// falls back to the CPU pass.
func (s *Sim) coverageMapGPU(grid coverage.HeightGrid, stations []coverage.Endpoint,
	r *coverage.Raster, o coverage.Options,
	extra func(int, float64, float64, float64, float64, float64) float64,
	progress func(done, total int)) (*coverage.Combined, string, bool) {

	dev, err := gpu.Open()
	if err != nil {
		return nil, "", false
	}
	defer dev.Close()
	// The seam tick: the bar sat silent between the last terrain row and
	// the first station while the device opened, compiled and uploaded -
	// long enough to be reported as a stall.
	if progress != nil {
		progress(0, len(stations))
	}
	cg, err := dev.UploadGrid(grid)
	if err != nil {
		return nil, "", false
	}
	defer cg.Release()

	fold := coverage.NewFold(r.South, r.North, r.West, r.East, r.Width, r.Height, r.FreqMHz)
	for i, st := range stations {
		ground, ok := grid.At(st.Lat, st.Lon)
		if !ok {
			continue
		}
		// A station that cannot reach any corner of the box has nothing to
		// contribute to it - HopReach's max-range cull, priced by free
		// space rather than by a config number.
		if !stationReaches(st, r, o) {
			if progress != nil {
				progress(i+1, len(stations))
			}
			continue
		}
		losses, err := cg.Loss(coverage.GridLossParams{
			StLat: st.Lat, StLon: st.Lon, StAltM: ground + st.HeightAGLm,
			RasterW: r.Width, RasterH: r.Height,
			South: r.South, North: r.North, West: r.West, East: r.East,
			RemoteHeightM: o.RemoteHeightAGLm, FreqMHz: r.FreqMHz,
			Steps: 96,
		})
		if err != nil {
			// A device that dies mid-job is a job the CPU finishes instead.
			return nil, "", false
		}
		sr := &coverage.Raster{South: r.South, North: r.North, West: r.West, East: r.East,
			Width: r.Width, Height: r.Height, FreqMHz: r.FreqMHz}
		if err := coverage.ComputeFromLosses(st, grid, losses, sr, o); err != nil {
			continue
		}
		s.priceBuildingsInto(sr, grid, st, i, o, extra)
		fold.Add(sr, i)
		if progress != nil {
			progress(i+1, len(stations))
		}
	}
	c := fold.Done()
	r.Cells = c.Cells
	return c, fmt.Sprintf("%s (%s)", dev.Name, dev.Backend), true
}

// stationReaches is the box-level cull: the nearest point of the box to
// the station, priced at free space with generous antenna slack. Terrain
// and buildings only ever add loss, so a station that fails here fails
// everywhere inside the box.
func stationReaches(st coverage.Endpoint, r *coverage.Raster, o coverage.Options) bool {
	lat := math.Min(math.Max(st.Lat, r.South), r.North)
	lon := math.Min(math.Max(st.Lon, r.West), r.East)
	distKm := haversineKmSession(st.Lat, st.Lon, lat, lon)
	if distKm <= 0 {
		return true
	}
	// Free space alone barely culls: a LoRa budget reaches a thousand
	// kilometres of vacuum, so every station on two islands "reached"
	// every viewport and the raster paid for all of them. The earth is
	// not a vacuum: at range, the horizon bulge itself is a knife edge no
	// antenna height on this network clears - ~330 m of it at 150 km -
	// and pricing that bulge culls honestly, with no configured range.
	d := distKm * 1000
	bulge := d * d / (8 * 4.0 / 3.0 * 6371000)
	clear := st.HeightAGLm + o.RemoteHeightAGLm + 100 // terrain grace
	loss := terrain.FSPLdB(distKm, r.FreqMHz)
	if bulge > clear {
		d1 := d / 2
		v := terrain.FresnelParameter(bulge-clear, d1, d1, r.FreqMHz)
		loss += terrain.KnifeEdgeDB(v)
	}
	const slack = 8
	out := st.TxPowerDBm + slack + o.RemoteGainDBi - loss - o.RemoteSensitivityDBm
	in := o.RemoteTxPowerDBm + o.RemoteGainDBi + slack - loss - st.SensitivityDBm
	return math.Min(out, in) >= -1
}

// priceBuildingsInto adds the environment's cost to the cells where it
// could matter: a cell already dead by tens of dB stays dead, and pricing
// buildings over open country for it is work with no witness.
func (s *Sim) priceBuildingsInto(sr *coverage.Raster, grid coverage.HeightGrid,
	st coverage.Endpoint, station int, o coverage.Options,
	extra func(int, float64, float64, float64, float64, float64) float64) {
	if extra == nil {
		return
	}
	stGround, ok := grid.At(st.Lat, st.Lon)
	if !ok {
		return
	}
	// Across every core: the corridor query made one cell cheap, and a
	// raster is a hundred thousand of them per station.
	var wg sync.WaitGroup
	rows := make(chan int)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range rows {
				for x := 0; x < sr.Width; x++ {
					i := y*sr.Width + x
					c := sr.Cells[i]
					if c.NoData {
						continue
					}
					// 12 dB of grace over the floor: a building can only
					// subtract, so a cell this far gone cannot come back.
					if c.OutboundMarginDB < -12 && c.InboundMarginDB < -12 {
						continue
					}
					lat, lon := sr.LatLonAt(x, y)
					ground, ok := grid.At(lat, lon)
					if !ok {
						continue
					}
					distM := haversineKmSession(st.Lat, st.Lon, lat, lon) * 1000
					e := extra(station, lat, lon, stGround+st.HeightAGLm,
						ground+o.RemoteHeightAGLm, distM)
					if e <= 0 {
						continue
					}
					c.OutboundMarginDB -= e
					c.InboundMarginDB -= e
					c.PathLossDB += e
					sr.Cells[i] = c
				}
			}
		}()
	}
	for y := 0; y < sr.Height; y++ {
		rows <- y
	}
	close(rows)
	wg.Wait()
}
