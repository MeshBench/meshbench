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
		losses, err := dev.CoverageGridLoss(grid, coverage.GridLossParams{
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
	fspl := terrain.FSPLdB(distKm, r.FreqMHz)
	const slack = 8
	out := st.TxPowerDBm + slack + o.RemoteGainDBi - fspl - o.RemoteSensitivityDBm
	in := o.RemoteTxPowerDBm + o.RemoteGainDBi + slack - fspl - st.SensitivityDBm
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
	for y := 0; y < sr.Height; y++ {
		for x := 0; x < sr.Width; x++ {
			i := y*sr.Width + x
			c := sr.Cells[i]
			if c.NoData {
				continue
			}
			// 12 dB of grace over the floor: a building can only subtract,
			// so a cell this far gone cannot come back.
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
}
