// The whole-map raster on the GPU.
//
// The same shape the link warm uses: the operator's one GPU switch decides,
// the device is opened for the job and closed after, and the CPU pass
// remains the twin that runs everywhere else. The kernel folds each
// station straight into per-cell best and second-best slots on the device,
// so the job pays one readback however many stations price it; buildings
// are priced once at the end, on the surviving cells only, with the
// second-best slot there to catch a shadow deep enough to change the
// winner.
package study

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/rf/gpu"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/study/coverage"
)

// foldBandCells caps one band's cell count: dispatch stays under the
// 65535-workgroup limit and the fold state under a modest slice of device
// memory. Rasters taller than a band run as several, exactly - the fold
// is per-cell, so banding changes nothing but the buffer sizes.
const foldBandCells = 4_000_000

func coverageMapGPU(s *session.Sim, grid propagation.HeightGrid, stations []coverage.Endpoint,
	r *coverage.Raster, o coverage.Options,
	extra func(int, float64, float64, float64, float64, float64) float64,
	progress func(what string, done, total int)) (*coverage.Combined, string, bool) {

	dev, err := gpu.Open()
	if err != nil {
		return nil, "", false
	}
	defer dev.Close()
	// The seam tick: the bar sat silent between the last terrain row and
	// the first station while the device opened, compiled and uploaded -
	// long enough to be reported as a stall.
	if progress != nil {
		progress("coverage: pricing every station on the GPU", 0, 1)
	}
	cg, err := dev.UploadGrid(grid)
	if err != nil {
		return nil, "", false
	}
	defer cg.Release()

	// Stations that can reach the box at all, with their budgets and
	// sampled patterns. The gain table is the kernel's view of the
	// antenna; its CPU twin holds the sampling honest.
	type prepared struct {
		idx    int
		st     coverage.Endpoint
		txAslM float64
		budget propagation.StationBudget
		gains  propagation.GainTable
	}
	var reach []prepared
	for i, st := range stations {
		ground, ok := grid.At(st.Lat, st.Lon)
		if !ok || !stationReaches(st, r, o) {
			continue
		}
		reach = append(reach, prepared{
			idx: i, st: st, txAslM: ground + st.HeightAGLm,
			budget: propagation.StationBudget{
				TxPowerDBm: st.TxPowerDBm, SensitivityDBm: st.SensitivityDBm,
				RemoteTxDBm: o.RemoteTxPowerDBm, RemoteGainDBi: o.RemoteGainDBi,
				RemoteSensitivityDBm: o.RemoteSensitivityDBm, Station: i,
			},
			gains: propagation.SampleGains(st.GainTowardsDBi),
		})
	}

	gw, gh := r.Width, r.Height
	n := gw * gh
	c := &coverage.Combined{
		Raster: coverage.Raster{South: r.South, North: r.North, West: r.West, East: r.East,
			Width: gw, Height: gh, FreqMHz: r.FreqMHz, Cells: make([]coverage.Cell, n)},
		BestMarginDB: make([]float64, n),
		BestNode:     make([]int, n),
		ServingCount: make([]int, n),
	}

	bandRows := gh
	if bandRows*gw > foldBandCells {
		bandRows = foldBandCells / gw
	}
	bands := (gh + bandRows - 1) / bandRows
	totalWork := len(reach)*bands + gh
	workDone := 0
	var tGPU, tBuild time.Duration

	for y0 := 0; y0 < gh; y0 += bandRows {
		y1 := y0 + bandRows
		if y1 > gh {
			y1 = gh
		}
		// The band's box: sliced so its cell centres are exactly the full
		// raster's rows y0..y1 - the fold is banded, the answer is not.
		p := propagation.GridLossParams{
			RasterW: gw, RasterH: y1 - y0,
			South: r.North - (r.North-r.South)*float64(y1)/float64(gh),
			North: r.North - (r.North-r.South)*float64(y0)/float64(gh),
			West:  r.West, East: r.East,
			RemoteHeightM: o.RemoteHeightAGLm, FreqMHz: r.FreqMHz, Steps: 96,
		}
		t0 := time.Now()
		fold, err := cg.NewFold(gw, y1-y0)
		if err != nil {
			return nil, "", false
		}
		for _, pr := range reach {
			p.StLat, p.StLon, p.StAltM = pr.st.Lat, pr.st.Lon, pr.txAslM
			if err := fold.Station(p, pr.budget, pr.gains); err != nil {
				// A device that dies mid-job is a job the CPU finishes.
				fold.Release()
				return nil, "", false
			}
			workDone++
			if progress != nil {
				progress("coverage: pricing every station on the GPU", workDone, totalWork)
			}
		}
		best, second, served, err := fold.Read()
		fold.Release()
		if err != nil {
			return nil, "", false
		}
		tGPU += time.Since(t0)

		t0 = time.Now()
		foldBandCPU(s, c, grid, stations, o, extra, best, second, served, y0, y1,
			func(rows int) {
				if progress != nil {
					progress("coverage: buildings on the survivors",
						len(reach)*bands+y0+rows, totalWork)
				}
			})
		tBuild += time.Since(t0)
	}
	fmt.Fprintf(os.Stderr, "coverage fold: gpu=%s buildings=%s stations=%d/%d bands=%d\n",
		tGPU.Round(time.Millisecond), tBuild.Round(time.Millisecond),
		len(reach), len(stations), bands)
	r.Cells = c.Cells
	return c, fmt.Sprintf("%s (%s)", dev.Name, dev.Backend), true
}

// foldBandCPU turns one band's slots into Combined cells, pricing
// buildings on the cells that survived. The runner-up is re-judged only
// when the winner's building cost drops it below the runner-up's clear
// margin - the one case where a shadow can change who serves a cell.
// Serving counts stay as the kernel counted them, before buildings: a
// count that moves by whether one roof grazes one path would read as
// network fragility, which is a different fact than shadowing.
func foldBandCPU(s *session.Sim, c *coverage.Combined, grid propagation.HeightGrid,
	stations []coverage.Endpoint, o coverage.Options,
	extra func(int, float64, float64, float64, float64, float64) float64,
	best, second []propagation.FoldSlot, served []uint32, y0, y1 int,
	rowsDone func(int)) {

	gw, gh := c.Width, c.Height
	price := func(sl propagation.FoldSlot, lat, lon, rxAsl float64) (float64, float64, int) {
		out, in, win := float64(sl.OutDB), float64(sl.InDB), int(sl.Station)
		if extra == nil || (out < -12 && in < -12) {
			return out, in, win
		}
		st := stations[win]
		stGround, ok := grid.At(st.Lat, st.Lon)
		if !ok {
			return out, in, win
		}
		distM := geo.DistanceKm(st.Lat, st.Lon, lat, lon) * 1000
		if e := extra(win, lat, lon, stGround+st.HeightAGLm, rxAsl, distM); e > 0 {
			out, in = out-e, in-e
		}
		return out, in, win
	}
	var wg sync.WaitGroup
	rows := make(chan int)
	var done int64
	var mu sync.Mutex
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for y := range rows {
				for x := 0; x < gw; x++ {
					gi := y*gw + x
					bi := (y-y0)*gw + x
					b := best[bi]
					if b.Station < 0 {
						c.Cells[gi] = coverage.Cell{NoData: true}
						c.BestMarginDB[gi] = math.NaN()
						c.BestNode[gi] = -1
						continue
					}
					lat := c.North - (c.North-c.South)*(float64(y)+0.5)/float64(gh)
					lon := c.West + (c.East-c.West)*(float64(x)+0.5)/float64(gw)
					ground, _ := grid.At(lat, lon)
					rxAsl := ground + o.RemoteHeightAGLm
					out, in, win := price(b, lat, lon, rxAsl)
					if sec := second[bi]; sec.Station >= 0 &&
						math.Min(out, in) < float64(sec.MinDB) {
						so, si, sw := price(sec, lat, lon, rxAsl)
						if math.Min(so, si) > math.Min(out, in) {
							out, in, win = so, si, sw
						}
					}
					c.Cells[gi] = coverage.Cell{OutboundMarginDB: out, InboundMarginDB: in}
					c.BestMarginDB[gi] = math.Min(out, in)
					c.BestNode[gi] = win
					c.ServingCount[gi] = int(served[bi])
				}
				mu.Lock()
				done++
				rowsDone(int(done))
				mu.Unlock()
			}
		}()
	}
	for y := y0; y < y1; y++ {
		rows <- y
	}
	close(rows)
	wg.Wait()
}

// stationReaches is the box-level cull: the nearest point of the box to
// the station, priced at free space with generous antenna slack. Terrain
// and buildings only ever add loss, so a station that fails here fails
// everywhere inside the box.
func stationReaches(st coverage.Endpoint, r *coverage.Raster, o coverage.Options) bool {
	lat := math.Min(math.Max(st.Lat, r.South), r.North)
	lon := math.Min(math.Max(st.Lon, r.West), r.East)
	distKm := geo.DistanceKm(st.Lat, st.Lon, lat, lon)
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
