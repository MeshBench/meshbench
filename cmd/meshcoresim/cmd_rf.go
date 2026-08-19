package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/geo"
	"github.com/MeshBench/meshbench/internal/planning"
	"github.com/MeshBench/meshbench/internal/rf"
	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/sdr"
	"github.com/MeshBench/meshbench/internal/terrain"
)

func runLink(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	store := terrainFlags(fs)
	fromLat := fs.Float64("from-lat", 0, "latitude of the first station")
	fromLon := fs.Float64("from-lon", 0, "longitude of the first station")
	fromH := fs.Float64("from-height", 10, "antenna height above ground, metres")
	fromTx := fs.Float64("from-tx", 22, "transmit power, dBm")
	fromGain := fs.Float64("from-gain", 2.15, "antenna gain, dBi")
	toLat := fs.Float64("to-lat", 0, "latitude of the second station")
	toLon := fs.Float64("to-lon", 0, "longitude of the second station")
	toH := fs.Float64("to-height", 1.5, "antenna height above ground, metres")
	toTx := fs.Float64("to-tx", 22, "transmit power, dBm")
	toGain := fs.Float64("to-gain", -2, "antenna gain, dBi")
	freq := fs.Float64("freq", 869.525, "frequency, MHz")
	sens := fs.Float64("sensitivity", -137, "receiver sensitivity, dBm")
	if err := parse(fs, args, "link budget between two points, in both directions"); err != nil {
		return err
	}
	if err := requireAll(map[string]bool{
		"from-lat": *fromLat == 0, "from-lon": *fromLon == 0,
		"to-lat": *toLat == 0, "to-lon": *toLon == 0,
	}); err != nil {
		return err
	}

	t, err := store()
	if err != nil {
		return err
	}
	r := &coverage.Raster{
		South: *toLat, North: *toLat, West: *toLon, East: *toLon,
		Width: 1, Height: 1, FreqMHz: *freq,
	}
	fixed := coverage.Endpoint{
		Name: "from", Lat: *fromLat, Lon: *fromLon, HeightAGLm: *fromH,
		TxPowerDBm: *fromTx, SensitivityDBm: *sens,
		GainTowardsDBi: func(float64, float64) float64 { return *fromGain },
	}
	opts := coverage.Options{
		RemoteHeightAGLm: *toH, RemoteTxPowerDBm: *toTx, RemoteGainDBi: *toGain,
		RemoteSensitivityDBm: *sens, ProfileStepM: 30,
	}
	if err := coverage.Compute(fixed, t, r, opts); err != nil {
		return err
	}
	cell := r.At(0, 0)
	if cell.NoData {
		return fmt.Errorf("no terrain covers this path; run 'meshcoresim terrain' for the area first")
	}

	distKm := geo.DistanceKm(*fromLat, *fromLon, *toLat, *toLon)
	l := planning.Summarise("from", "to", distKm, cell)

	fmt.Printf("%.2f km, path loss %.1f dB at %.3f MHz\n\n", distKm, cell.PathLossDB, *freq)
	fmt.Printf("  from -> to  %+7.1f dB\n", cell.OutboundMarginDB)
	fmt.Printf("  to -> from  %+7.1f dB\n\n", cell.InboundMarginDB)
	switch {
	case l.Workable:
		fmt.Printf("Works both ways; %+.1f dB in the weaker (%s) direction.\n", l.WorstCaseDB, l.LimitedBy)
	case l.OneWayOnly:
		fmt.Printf("ONE WAY ONLY — the %s direction fails.\n"+
			"This is the answer worth having: one end will hear the other and not be heard back.\n",
			l.LimitedBy)
	default:
		fmt.Printf("Does not work; the weaker direction is %.1f dB short.\n", -l.WorstCaseDB)
	}
	return ctx.Err()
}

func runProfile(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("profile", flag.ExitOnError)
	store := terrainFlags(fs)
	fromLat := fs.Float64("from-lat", 0, "latitude of the first point")
	fromLon := fs.Float64("from-lon", 0, "longitude of the first point")
	fromH := fs.Float64("from-height", 10, "antenna height above ground, metres")
	toLat := fs.Float64("to-lat", 0, "latitude of the second point")
	toLon := fs.Float64("to-lon", 0, "longitude of the second point")
	toH := fs.Float64("to-height", 1.5, "antenna height above ground, metres")
	samples := fs.Int("samples", 200, "profile samples")
	if err := parse(fs, args, "terrain profile and the worst obstruction on a path"); err != nil {
		return err
	}
	if err := requireAll(map[string]bool{
		"from-lat": *fromLat == 0, "from-lon": *fromLon == 0,
		"to-lat": *toLat == 0, "to-lon": *toLon == 0,
	}); err != nil {
		return err
	}

	t, err := store()
	if err != nil {
		return err
	}
	fromGround, ok := t.ElevationM(*fromLat, *fromLon)
	if !ok {
		return fmt.Errorf("no terrain at the first point")
	}
	toGround, ok := t.ElevationM(*toLat, *toLon)
	if !ok {
		return fmt.Errorf("no terrain at the second point")
	}
	distKm := geo.DistanceKm(*fromLat, *fromLon, *toLat, *toLon)
	txAlt, rxAlt := fromGround+*fromH, toGround+*toH

	worst, worstAt, worstGround := math.Inf(-1), 0.0, 0.0
	for i := 0; i <= *samples; i++ {
		f := float64(i) / float64(*samples)
		h, ok := t.ElevationM(*fromLat+(*toLat-*fromLat)*f, *fromLon+(*toLon-*fromLon)*f)
		if !ok {
			return fmt.Errorf("terrain runs out %.1f km along the path", f*distKm)
		}
		d1, d2 := f*distKm*1000, (1-f)*distKm*1000
		if d1 <= 0 || d2 <= 0 {
			continue
		}
		intrusion := h + terrain.EarthBulgeM(d1, d2) - (txAlt + (rxAlt-txAlt)*f)
		if intrusion > worst {
			worst, worstAt, worstGround = intrusion, f*distKm, h
		}
	}

	fmt.Printf("%.2f km. Ends %.0f m and %.0f m AMSL (ground %.0f/%.0f, antennas %.0f/%.0f).\n",
		distKm, txAlt, rxAlt, fromGround, toGround, *fromH, *toH)
	fmt.Printf("Worst obstruction %.1f km along: ground %.0f m, %+.1f m against the line of sight\n"+
		"(earth curvature included).\n\n", worstAt, worstGround, worst)
	if worst > 0 {
		fmt.Printf("BLOCKED by %.0f m. Raising an antenna by about that, or moving, is what changes it.\n", worst)
	} else {
		fmt.Printf("Line of sight clear by %.0f m. Clearance is not the same as a clear Fresnel zone —\n"+
			"run 'meshcoresim link' for the answer that counts.\n", -worst)
	}
	return ctx.Err()
}

func runCoverage(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("coverage", flag.ExitOnError)
	store := terrainFlags(fs)
	lat := fs.Float64("lat", 0, "station latitude")
	lon := fs.Float64("lon", 0, "station longitude")
	height := fs.Float64("height", 10, "antenna height above ground, metres")
	tx := fs.Float64("tx", 22, "transmit power, dBm")
	gain := fs.Float64("gain", 2.15, "antenna gain, dBi")
	radiusKm := fs.Float64("radius", 20, "half-width of the area, km")
	px := fs.Int("pixels", 400, "raster width in pixels")
	freq := fs.Float64("freq", 869.525, "frequency, MHz")
	sens := fs.Float64("sensitivity", -137, "receiver sensitivity, dBm")
	remoteH := fs.Float64("remote-height", 1.5, "height of the imagined far station, metres")
	remoteTx := fs.Float64("remote-tx", 22, "far station transmit power, dBm")
	out := fs.String("o", "coverage.png", "output PNG")
	if err := parse(fs, args, "coverage raster from one station"); err != nil {
		return err
	}
	if err := requireAll(map[string]bool{"lat": *lat == 0, "lon": *lon == 0}); err != nil {
		return err
	}

	t, err := store()
	if err != nil {
		return err
	}
	dLat := *radiusKm / 111.32
	dLon := *radiusKm / (111.32 * math.Cos(*lat*math.Pi/180))
	r := &coverage.Raster{
		South: *lat - dLat, North: *lat + dLat,
		West: *lon - dLon, East: *lon + dLon,
		Width: *px, Height: *px, FreqMHz: *freq,
	}

	if !t.Offline {
		e := t.Estimate(r.South, r.North, r.West, r.East)
		fmt.Fprintf(os.Stderr, "terrain: %d tiles, %d cached, about %d MB to fetch\n",
			e.Tiles, e.Cached, e.BytesRough>>20)
		t.OnProgress = func(done, total int) {
			if done == total || done%25 == 0 {
				fmt.Fprintf(os.Stderr, "\rterrain: %d/%d", done, total)
			}
		}
		if err := t.Prefetch(ctx, r.South, r.North, r.West, r.East); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr)
	}

	station := coverage.Endpoint{
		Name: "station", Lat: *lat, Lon: *lon, HeightAGLm: *height,
		TxPowerDBm: *tx, SensitivityDBm: *sens,
		GainTowardsDBi: func(float64, float64) float64 { return *gain },
	}
	opts := coverage.Options{
		RemoteHeightAGLm: *remoteH, RemoteTxPowerDBm: *remoteTx, RemoteGainDBi: -2,
		RemoteSensitivityDBm: *sens, ProfileStepM: 30,
	}
	if err := coverage.Compute(station, t, r, opts); err != nil {
		return err
	}

	var both, oneWay, none, nodata int
	for _, c := range r.Cells {
		switch {
		case c.NoData:
			nodata++
		case c.Workable():
			both++
		case c.OneWay():
			oneWay++
		default:
			none++
		}
	}
	known := len(r.Cells) - nodata
	if known == 0 {
		return fmt.Errorf("no terrain covers this area")
	}
	fmt.Printf("%d x %d cells over +/-%.0f km.\n", r.Width, r.Height, *radiusKm)
	fmt.Printf("  two-way    %5.1f%%\n", 100*float64(both)/float64(known))
	fmt.Printf("  one-way    %5.1f%%   <- heard but not heard back\n", 100*float64(oneWay)/float64(known))
	fmt.Printf("  no link    %5.1f%%\n", 100*float64(none)/float64(known))
	if nodata > 0 {
		fmt.Printf("  no data    %5.1f%%   (outside the downloaded tiles)\n",
			100*float64(nodata)/float64(len(r.Cells)))
	}
	return writeCoveragePNG(*out, r)
}

// writeCoveragePNG draws the raster.
//
// One-way cells get their own colour rather than being lumped in with "covered"
// or "not covered". They are neither, and they are the single most useful thing
// a coverage map can show.
func writeCoveragePNG(path string, r *coverage.Raster) error {
	img := image.NewNRGBA(image.Rect(0, 0, r.Width, r.Height))
	for y := 0; y < r.Height; y++ {
		for x := 0; x < r.Width; x++ {
			c := r.At(x, y)
			var col color.NRGBA
			switch {
			case c.NoData:
				col = color.NRGBA{R: 32, G: 32, B: 36, A: 255}
			case c.Workable():
				// Brighter with more margin, so a marginal cell does not look
				// like a comfortable one.
				m := math.Min(1, math.Max(0, math.Min(c.OutboundMarginDB, c.InboundMarginDB)/30))
				col = color.NRGBA{R: uint8(40 + 60*m), G: uint8(120 + 110*m), B: uint8(60 + 40*m), A: 255}
			case c.OneWay():
				col = color.NRGBA{R: 232, G: 160, B: 40, A: 255}
			default:
				col = color.NRGBA{R: 60, G: 62, B: 74, A: 255}
			}
			img.Set(x, y, col)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	fmt.Printf("\nwrote %s (green two-way, amber one-way, grey none, dark no data)\n", path)
	return f.Close()
}

func runSpectrum(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("spectrum", flag.ExitOnError)
	sf := fs.Int("sf", 10, "spreading factor of the transmission")
	bw := fs.Float64("bandwidth", 250, "bandwidth, kHz")
	freq := fs.Float64("freq", 869.525, "centre frequency, MHz")
	rxDBm := fs.Float64("rx", -100, "received signal level, dBm")
	symbols := fs.Int("symbols", 8, "symbols to capture")
	nf := fs.Float64("noise-figure", 6, "observer noise figure, dB")
	outPNG := fs.String("o", "waterfall.png", "waterfall PNG")
	outWAV := fs.String("wav", "", "also write audio here, as an SDR would sound")
	if err := parse(fs, args, "what an SDR observer captures"); err != nil {
		return err
	}

	obs := sdr.Observer{
		Name: "observer", CentreHz: *freq * 1e6,
		SampleRateHz: *bw * 1000, NoiseFigureDB: *nf,
	}
	mod := dsp.Modulator{SF: *sf}
	n := dsp.SamplesPerSymbol(*sf)
	cap := obs.Capture([]rf.Transmission{{
		Node: "tx", Samples: mod.Modulate(make([]int, *symbols)), GainDB: *rxDBm,
	}}, 4417, 0, n*(*symbols+1))

	spec := cap.Spectrogram(128, 32)
	if len(spec.Frames) == 0 {
		return fmt.Errorf("capture too short to make a waterfall")
	}
	bins := len(spec.Frames[0])
	perBin := spec.NoiseFloorDB - 10*math.Log10(float64(bins)) + 1.76
	fmt.Printf("SF%d at %.0f kHz, %.0f dBm into a %.0f dB noise figure.\n", *sf, *bw, *rxDBm, *nf)
	fmt.Printf("Noise floor %.1f dB across the band, %.1f dB per bin. SNR about %.1f dB.\n",
		spec.NoiseFloorDB, perBin, *rxDBm-spec.NoiseFloorDB)

	img := image.NewNRGBA(image.Rect(0, 0, bins, len(spec.Frames)))
	lo, hi := perBin-2, perBin+22
	for y, row := range spec.Frames {
		for x, v := range row {
			t := math.Max(0, math.Min(1, (v-lo)/(hi-lo)))
			img.Set(x, y, color.NRGBA{
				R: uint8(8 + 240*t*t), G: uint8(12 + 230*t), B: uint8(30 + 170*t), A: 255,
			})
		}
	}
	f, err := os.Create(*outPNG)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", *outPNG)

	if *outWAV != "" {
		tune := sdr.Tuning{AudioRateHz: 6000}
		wf, err := os.Create(*outWAV)
		if err != nil {
			return err
		}
		if err := sdr.WriteWAV(wf, cap.Listen(tune), int(cap.AudioRate(tune))); err != nil {
			_ = wf.Close()
			return err
		}
		if err := wf.Close(); err != nil {
			return err
		}
		fmt.Printf("wrote %s — a chirp through a narrow filter is a rising whistle\n", *outWAV)
	}
	return ctx.Err()
}

func runAirtime(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("airtime", flag.ExitOnError)
	sf := fs.Int("sf", 10, "spreading factor")
	bw := fs.Float64("bandwidth", 250, "bandwidth, kHz")
	cr := fs.Int("coding-rate", 1, "1 to 4, for 4/5 to 4/8")
	bytes := fs.Int("bytes", 32, "payload length")
	if err := parse(fs, args, "LoRa time on air, as the firmware computes it"); err != nil {
		return err
	}
	if *sf < 5 || *sf > 12 {
		return fmt.Errorf("spreading factor %d is outside SF5-SF12", *sf)
	}
	ms := dsp.AirtimeMillis(*sf, *bw*1000, *cr, *bytes, true, true)
	fmt.Printf("SF%d, %.0f kHz, CR 4/%d, %d bytes: %.0f ms on air\n", *sf, *bw, *cr+4, *bytes, ms)
	fmt.Printf("At a 1%% duty cycle that is %.0f transmissions per hour per node.\n", 36000/ms)
	fmt.Printf("Noise floor at this bandwidth: %.1f dBm with a 6 dB noise figure.\n",
		dsp.NoiseFloorDBm(*bw*1000, 6))
	return nil
}

func runBoards(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("boards", flag.ExitOnError)
	if err := parse(fs, args, "the hardware profiles this build knows about"); err != nil {
		return err
	}
	fmt.Printf("%-20s %-10s %-8s %8s %8s  %s\n", "BOARD", "MCU", "EMULATE", "TX dBm", "RADIATED", "NOTES")
	for _, b := range scenario.Boards() {
		emu := "no"
		if b.Emulated {
			emu = "yes"
		}
		fmt.Printf("%-20s %-10s %-8s %8.0f %8.1f  %s\n",
			b.Name, b.MCU, emu, b.MaxTxDBm, b.RadiatedDBm(b.MaxTxDBm), firstSentence(b.Notes))
	}
	fmt.Println("\nRADIATED is what leaves the antenna: chip power minus board loss plus the")
	fmt.Println("antenna it ships with. It is the number that decides range, and it is not the")
	fmt.Println("number on the box.")
	return nil
}

func firstSentence(s string) string {
	for i, r := range s {
		if r == '.' {
			return s[:i+1]
		}
	}
	return s
}
