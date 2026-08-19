package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/MeshBench/meshbench/internal/coverage"
	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/energy"
	"github.com/MeshBench/meshbench/internal/geo"
	"github.com/MeshBench/meshbench/internal/planning"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// Terrain is the elevation source the tools need. Aliased from the coverage
// package so a caller wiring up an MCP server does not have to import two
// packages to name one thing.
type Terrain = coverage.Terrain

// RegisterEngineTools adds the simulator's own tools to a server.
//
// The terrain source is supplied by the caller rather than constructed here,
// because whether tiles may be downloaded is the operator's decision and not
// something an assistant should make on their behalf.
func RegisterEngineTools(s *Server, t Terrain) error {
	for _, tool := range []Tool{
		linkBudgetTool(t),
		pathProfileTool(t),
		airtimeTool(),
		energyTool(),
		limitationsTool(),
	} {
		if err := s.Register(tool); err != nil {
			return err
		}
	}
	return nil
}

type linkArgs struct {
	// One field per tag. A combined declaration shares a single struct tag, so
	// `FromLat, FromLon float64 ` + "`json:\"from_lat\"`" + ` silently leaves longitude at
	// zero — and a link budget computed on the Greenwich meridian still returns
	// a perfectly plausible number.
	FromLat            float64 `json:"from_lat"`
	FromLon            float64 `json:"from_lon"`
	FromHeightM        float64 `json:"from_height_m"`
	FromTxDBm          float64 `json:"from_tx_dbm"`
	FromGainDBi        float64 `json:"from_gain_dbi"`
	ToLat              float64 `json:"to_lat"`
	ToLon              float64 `json:"to_lon"`
	ToHeightM          float64 `json:"to_height_m"`
	ToTxDBm            float64 `json:"to_tx_dbm"`
	ToGainDBi          float64 `json:"to_gain_dbi"`
	FreqMHz            float64 `json:"freq_mhz"`
	SF                 int     `json:"spreading_factor"`
	BandwidthKHz       float64 `json:"bandwidth_khz"`
	SensitivityDBmFrom float64 `json:"from_sensitivity_dbm"`
	SensitivityDBmTo   float64 `json:"to_sensitivity_dbm"`
}

func linkBudgetTool(t Terrain) Tool {
	return Tool{
		Name: "link_budget",
		Description: "Compute a point-to-point link budget over real terrain, in BOTH directions. " +
			"Returns the margin each way, which direction limits the link, the path loss and the " +
			"model's own known biases. Reachability is asymmetric: a repeater on a mast reaching a " +
			"handheld is not the same link coming back.",
		InputSchema: schema(map[string]any{
			"from_lat": num("Latitude of the first station"),
			"from_lon": num("Longitude of the first station"),
			"from_height_m": num("Antenna height above ground in metres (not above sea level; " +
				"ground elevation comes from the DEM)"),
			"from_tx_dbm":          num("Transmit power in dBm"),
			"from_gain_dbi":        num("Antenna gain in dBi, feedline loss already deducted"),
			"from_sensitivity_dbm": num("Receiver sensitivity in dBm, e.g. -130"),
			"to_lat":               num("Latitude of the second station"),
			"to_lon":               num("Longitude of the second station"),
			"to_height_m":          num("Antenna height above ground in metres"),
			"to_tx_dbm":            num("Transmit power in dBm"),
			"to_gain_dbi":          num("Antenna gain in dBi"),
			"to_sensitivity_dbm":   num("Receiver sensitivity in dBm"),
			"freq_mhz":             num("Frequency in MHz, e.g. 869.525"),
		}, "from_lat", "from_lon", "to_lat", "to_lon", "freq_mhz"),

		Call: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a linkArgs
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", fmt.Errorf("bad arguments: %w", err)
			}
			if a.FreqMHz <= 0 {
				return "", fmt.Errorf("%w: freq_mhz", ErrMissingArgument)
			}
			defaults(&a)

			// One cell, computed exactly as the raster does, so the answer here
			// and the answer on the map cannot disagree.
			r := &coverage.Raster{
				South: a.ToLat, North: a.ToLat, West: a.ToLon, East: a.ToLon,
				Width: 1, Height: 1, FreqMHz: a.FreqMHz,
			}
			fixed := coverage.Endpoint{
				Name: "from", Lat: a.FromLat, Lon: a.FromLon,
				HeightAGLm: a.FromHeightM, TxPowerDBm: a.FromTxDBm,
				SensitivityDBm: a.SensitivityDBmFrom,
				GainTowardsDBi: func(float64, float64) float64 { return a.FromGainDBi },
			}
			opts := coverage.Options{
				RemoteHeightAGLm: a.ToHeightM, RemoteTxPowerDBm: a.ToTxDBm,
				RemoteGainDBi: a.ToGainDBi, RemoteSensitivityDBm: a.SensitivityDBmTo,
				ProfileStepM: 30,
			}
			if err := coverage.Compute(fixed, t, r, opts); err != nil {
				return "", err
			}
			cell := r.At(0, 0)
			if cell.NoData {
				return "", fmt.Errorf("no terrain data covers this path; download tiles for the area first")
			}

			distKm := geo.DistanceKm(a.FromLat, a.FromLon, a.ToLat, a.ToLon)
			l := planning.Summarise("from", "to", distKm, cell)

			var b strings.Builder
			fmt.Fprintf(&b, "Distance %.2f km, path loss %.1f dB at %.3f MHz.\n\n", distKm, cell.PathLossDB, a.FreqMHz)
			fmt.Fprintf(&b, "  from -> to: %+.1f dB margin\n", cell.OutboundMarginDB)
			fmt.Fprintf(&b, "  to -> from: %+.1f dB margin\n\n", cell.InboundMarginDB)
			switch {
			case l.Workable:
				fmt.Fprintf(&b, "Works both ways. Worst direction has %+.1f dB, limited by the %s path.\n",
					l.WorstCaseDB, l.LimitedBy)
			case l.OneWayOnly:
				fmt.Fprintf(&b, "ONE WAY ONLY. The %s direction fails. This is the case worth telling "+
					"an operator about: they will hear it and it will not hear them.\n", weaker(cell))
			default:
				fmt.Fprintf(&b, "Does not work in either direction; the weaker is %+.1f dB short.\n", l.WorstCaseDB)
			}
			b.WriteString("\n" + biasNote(distKm))
			return b.String(), nil
		},
	}
}

func pathProfileTool(t Terrain) Tool {
	return Tool{
		Name: "path_profile",
		Description: "Sample the terrain between two points and report the highest obstruction " +
			"relative to the line of sight. Use this to explain WHY a link fails rather than just " +
			"that it does.",
		InputSchema: schema(map[string]any{
			"from_lat":      num("Latitude of the first point"),
			"from_lon":      num("Longitude of the first point"),
			"from_height_m": num("Antenna height above ground in metres"),
			"to_lat":        num("Latitude of the second point"),
			"to_lon":        num("Longitude of the second point"),
			"to_height_m":   num("Antenna height above ground in metres"),
			"samples":       num("Number of profile samples (default 128)"),
		}, "from_lat", "from_lon", "to_lat", "to_lon"),

		Call: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a struct {
				FromLat     float64 `json:"from_lat"`
				FromLon     float64 `json:"from_lon"`
				FromHeightM float64 `json:"from_height_m"`
				ToLat       float64 `json:"to_lat"`
				ToLon       float64 `json:"to_lon"`
				ToHeightM   float64 `json:"to_height_m"`
				Samples     int     `json:"samples"`
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", fmt.Errorf("bad arguments: %w", err)
			}
			n := a.Samples
			if n <= 2 {
				n = 128
			}
			distKm := geo.DistanceKm(a.FromLat, a.FromLon, a.ToLat, a.ToLon)
			if distKm <= 0 {
				return "", fmt.Errorf("the two points are the same place")
			}

			fromGround, ok := t.ElevationM(a.FromLat, a.FromLon)
			if !ok {
				return "", fmt.Errorf("no terrain data at the first point")
			}
			toGround, ok := t.ElevationM(a.ToLat, a.ToLon)
			if !ok {
				return "", fmt.Errorf("no terrain data at the second point")
			}
			txAlt := fromGround + a.FromHeightM
			rxAlt := toGround + a.ToHeightM

			worstIntrusion, worstAt, worstGround := math.Inf(-1), 0.0, 0.0
			for i := 0; i <= n; i++ {
				f := float64(i) / float64(n)
				lat := a.FromLat + (a.ToLat-a.FromLat)*f
				lon := a.FromLon + (a.ToLon-a.FromLon)*f
				h, ok := t.ElevationM(lat, lon)
				if !ok {
					return "", fmt.Errorf("terrain data runs out %.1f km along the path", f*distKm)
				}
				d1 := f * distKm * 1000
				d2 := (1 - f) * distKm * 1000
				if d1 <= 0 || d2 <= 0 {
					continue
				}
				los := txAlt + (rxAlt-txAlt)*f
				// The bulge is what turns a long flat path into an obstructed
				// one, so it belongs in any answer about why a link failed.
				intrusion := h + terrain.EarthBulgeM(d1, d2) - los
				if intrusion > worstIntrusion {
					worstIntrusion, worstAt, worstGround = intrusion, f*distKm, h
				}
			}

			var b strings.Builder
			fmt.Fprintf(&b, "Path %.2f km. Ends at %.0f m and %.0f m above sea level "+
				"(ground %.0f m and %.0f m, antennas %.0f m and %.0f m).\n\n",
				distKm, txAlt, rxAlt, fromGround, toGround, a.FromHeightM, a.ToHeightM)
			fmt.Fprintf(&b, "Worst obstruction is %.1f km along, ground %.0f m, "+
				"%+.1f m relative to the line of sight (earth curvature included).\n",
				worstAt, worstGround, worstIntrusion)
			if worstIntrusion > 0 {
				fmt.Fprintf(&b, "\nThe path is BLOCKED by %.0f m. Raising an antenna by roughly that much, "+
					"or moving to clear the obstruction, is what changes this.\n", worstIntrusion)
			} else {
				fmt.Fprintf(&b, "\nLine of sight is clear by %.0f m, but clearance is not the same as an "+
					"unobstructed Fresnel zone — the link budget is the answer, not this.\n", -worstIntrusion)
			}
			return b.String(), nil
		},
	}
}

func airtimeTool() Tool {
	return Tool{
		Name: "lora_airtime",
		Description: "Time on air for a LoRa packet, computed the way the firmware computes it " +
			"(RadioLib getTimeOnAir, with MeshCore's own preamble lengths). Use for duty cycle and " +
			"collision reasoning.",
		InputSchema: schema(map[string]any{
			"spreading_factor": num("SF, 7 to 12"),
			"bandwidth_khz":    num("Bandwidth in kHz, typically 125 or 250"),
			"payload_bytes":    num("Payload length in bytes"),
			"coding_rate":      num("1 to 4, for 4/5 to 4/8 (default 1)"),
		}, "spreading_factor", "bandwidth_khz", "payload_bytes"),

		Call: func(_ context.Context, raw json.RawMessage) (string, error) {
			var a struct {
				SF    int     `json:"spreading_factor"`
				BWkHz float64 `json:"bandwidth_khz"`
				Bytes int     `json:"payload_bytes"`
				CR    int     `json:"coding_rate"`
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", fmt.Errorf("bad arguments: %w", err)
			}
			if a.SF < 5 || a.SF > 12 {
				return "", fmt.Errorf("spreading_factor %d is outside SF5-SF12", a.SF)
			}
			if a.BWkHz <= 0 {
				return "", fmt.Errorf("%w: bandwidth_khz", ErrMissingArgument)
			}
			if a.CR < 1 || a.CR > 4 {
				a.CR = 1
			}
			ms := dsp.AirtimeMillis(a.SF, a.BWkHz*1000, a.CR, a.Bytes, true, true)
			perHour := 3600_000 * 0.01 / ms // 1% duty cycle

			return fmt.Sprintf(
				"SF%d, %.0f kHz, CR 4/%d, %d byte payload: %.0f ms on air.\n\n"+
					"At the 1%% duty cycle limit that is about %.0f transmissions per hour per node. "+
					"Sensitivity at this SF is roughly %.1f dB SNR, and the noise floor at this bandwidth "+
					"is %.1f dBm with a 6 dB noise figure.\n",
				a.SF, a.BWkHz, a.CR+4, a.Bytes, ms, perHour,
				-dsp.ProcessingGainDB(a.SF)+2.5, dsp.NoiseFloorDBm(a.BWkHz*1000, 6)), nil
		},
	}
}

func energyTool() Tool {
	return Tool{
		Name: "solar_budget",
		Description: "Will a solar node survive the winter at this latitude? Simulates a year " +
			"hourly and reports the worst state of charge, dead days and autonomy. Receive current, " +
			"not transmit power, is usually what decides this.",
		InputSchema: schema(map[string]any{
			"lat":            num("Latitude, north positive"),
			"lon":            num("Longitude, east positive"),
			"panel_w":        num("Panel peak watts"),
			"panel_tilt_deg": num("Panel tilt from horizontal (default 50)"),
			"battery_mah":    num("Battery capacity in mAh"),
			"always_on":      boolean("True for a repeater that listens continuously (default true)"),
			"tx_dbm":         num("Transmit power in dBm (default 22)"),
		}, "lat", "panel_w", "battery_mah"),

		Call: func(_ context.Context, raw json.RawMessage) (string, error) {
			var a struct {
				Lat        float64  `json:"lat"`
				Lon        float64  `json:"lon"`
				PanelW     float64  `json:"panel_w"`
				TiltDeg    *float64 `json:"panel_tilt_deg"`
				BatteryMAh float64  `json:"battery_mah"`
				AlwaysOn   *bool    `json:"always_on"`
				TxDBm      *float64 `json:"tx_dbm"`
			}
			if err := json.Unmarshal(raw, &a); err != nil {
				return "", fmt.Errorf("bad arguments: %w", err)
			}
			if a.PanelW <= 0 || a.BatteryMAh <= 0 {
				return "", fmt.Errorf("%w: panel_w and battery_mah", ErrMissingArgument)
			}
			tilt := 50.0
			if a.TiltDeg != nil {
				tilt = *a.TiltDeg
			}
			tx := 22.0
			if a.TxDBm != nil {
				tx = *a.TxDBm
			}
			alwaysOn := true
			if a.AlwaysOn != nil {
				alwaysOn = *a.AlwaysOn
			}

			site := energy.Site{
				Name: "site", LatDeg: a.Lat, LonDeg: a.Lon,
				Battery: energy.Battery{Chemistry: energy.LiIon, CapacityMAh: a.BatteryMAh, Cells: 1, CutoffV: 3.1},
				Panel: energy.Panel{PeakW: a.PanelW, TiltDeg: tilt, AzimuthDeg: 180,
					SoilingFactor: 0.8, ChargeEfficiency: 0.95},
				Load:       energy.SX1262Load(),
				Duty:       energy.DutyFromAirtime(1, 0, 1000, alwaysOn),
				TxPowerDBm: tx,
				CloudByMonth: [12]float64{0.75, 0.72, 0.68, 0.62, 0.58, 0.58,
					0.60, 0.62, 0.65, 0.72, 0.78, 0.80},
				TempCByMonth: [12]float64{1, 1, 3, 5, 8, 11, 13, 13, 10, 7, 3, 1},
			}
			res, err := energy.SimulateYear(site)
			if err != nil {
				return "", err
			}

			verdict := "survives the year"
			if res.DeadDays > 0 {
				verdict = fmt.Sprintf("FAILS: dead on %d days", res.DeadDays)
			} else if res.WorstSoC < 0.3 {
				verdict = "marginal — under 30% at the worst point, with no allowance for snow or a bad winter"
			}
			return fmt.Sprintf(
				"%.0f W panel at %.0f degrees tilt, %.0f mAh battery, latitude %.2f.\n\n"+
					"Worst state of charge %.0f%% on day %d of the year. Autonomy from full with no sun: "+
					"%.1f days.\n\nVerdict: %s.\n\n"+
					"Note: cloud cover is a monthly mean for a British winter, not measured climate data "+
					"for this site, and nothing here models snow on the panel.\n",
				a.PanelW, tilt, a.BatteryMAh, a.Lat,
				res.WorstSoC*100, res.WorstDay, res.AutonomyDays, verdict), nil
		},
	}
}

// limitationsTool exists because an assistant will otherwise present these
// numbers with more confidence than they deserve. Making the error budget
// callable means it can be quoted rather than guessed at.
func limitationsTool() Tool {
	return Tool{
		Name: "model_limitations",
		Description: "What this simulator does NOT model, and in which direction it is wrong. " +
			"Call this before presenting any result as a prediction.",
		InputSchema: schema(map[string]any{}),
		Call: func(context.Context, json.RawMessage) (string, error) {
			return strings.Join([]string{
				"MeshcoreSim is systematically OPTIMISTIC. Treat every result as a best case.",
				"",
				"Absent from the model:",
				"  - No multipath. The failure that dominates real marginal links — good median",
				"    signal, dropping out for hundreds of milliseconds — cannot occur here.",
				"  - No frequency error or oscillator drift, so capture effect is cleaner than reality.",
				"  - Bare-earth terrain: no buildings, no trees, no clutter, no body loss. For woodland",
				"    at 868 MHz the difference is tens of dB, not a correction factor.",
				"  - No ground reflection, and no rounded-obstacle correction. Knife-edge loss is the",
				"    optimistic bound for a real hill.",
				"  - Thermal noise only. No impulsive or man-made noise.",
				"",
				"Measured error we know about:",
				"  - The demodulator decodes up to 1.6 dB below a real SX1262 at SF12 (SF11 1.1 dB,",
				"    SF9 0.6 dB). Long-range links are the most optimistic ones.",
				"  - No FEC, interleaving or CRC, so packet errors do not translate as they would.",
				"",
				"Never validated against a real observation. Every component is checked against a",
				"published reference; none of it has been compared with a packet that crossed real air.",
				"",
				"What it is good for: comparative work. 'This mast versus that one', 'five metres",
				"higher', 'SF10 versus SF12' — the systematic optimism largely cancels between two",
				"answers computed the same way. If it says a link will NOT work, believe it. If it says",
				"a link works marginally, go and measure.",
			}, "\n"), nil
		},
	}
}

func defaults(a *linkArgs) {
	if a.SensitivityDBmFrom == 0 {
		a.SensitivityDBmFrom = -130
	}
	if a.SensitivityDBmTo == 0 {
		a.SensitivityDBmTo = -130
	}
	if a.FromTxDBm == 0 {
		a.FromTxDBm = 22
	}
	if a.ToTxDBm == 0 {
		a.ToTxDBm = 22
	}
}

func weaker(c coverage.Cell) string {
	if c.InboundMarginDB < c.OutboundMarginDB {
		return "return"
	}
	return "outbound"
}

func biasNote(distKm float64) string {
	s := "This is a best case: no multipath, bare-earth terrain, no clutter, and a demodulator " +
		"up to 1.6 dB better than a real SX1262 at high spreading factors. Call model_limitations " +
		"for the full list."
	if distKm > 30 {
		s += "\nOver 30 km the earth-curvature term dominates and small height changes matter " +
			"disproportionately; check path_profile before trusting a marginal answer."
	}
	return s
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func num(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}
