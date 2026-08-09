package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/capture"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// runTraffic is the flood simulator: put a message into a network and report
// what happened to it, node by node.
//
// The whole point of the project reduces to this command. Everything else —
// terrain, budgets, coverage — is machinery for answering the question it asks.
func runTraffic(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("traffic", flag.ExitOnError)
	store := terrainFlags(fs)
	nodesPath := fs.String("nodes", "", "scenario JSON, or a CoreScope/Beacon export")
	source := fs.String("source", "", "load nodes from a provider: corescope or beacon")
	baseURL := fs.String("url", "", "provider base URL")
	token := fs.String("token", "", "provider token, if it needs one")
	board := fs.String("board", "RAK4631", "board profile for imported nodes")
	freq := fs.Float64("freq", 869.525, "frequency, MHz")
	sf := fs.Int("sf", 10, "spreading factor")
	bw := fs.Float64("bandwidth", 250, "bandwidth, kHz")
	origin := fs.String("from", "", "node to send from; the first repeater by default")
	forMs := fs.Uint("for", 20000, "how long to simulate, ms")
	verbose := fs.Bool("v", false, "print every event rather than a summary")
	real := fs.Bool("firmware", false,
		"run a real MeshCore build on every node, rather than injecting traffic")
	if err := parse(fs, args, "flood a message through a network and report what happened"); err != nil {
		return err
	}

	t, err := store()
	if err != nil {
		return err
	}
	nodes, err := loadNodes(ctx, *nodesPath, *source, *baseURL, *token, *board, *freq, *sf, *bw)
	if err != nil {
		return err
	}
	if len(nodes) < 2 {
		return fmt.Errorf("a flood needs at least two nodes; got %d", len(nodes))
	}

	e := engine.New(t, engine.Config{
		FreqMHz: *freq, SF: *sf, BandwidthHz: *bw * 1000, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	defer func() { _ = e.Close() }()

	originIdx := -1
	for i, n := range nodes {
		e.Add(n, nil)
		if n.Name == *origin || (*origin == "" && originIdx < 0 && n.Kind.Transmits()) {
			originIdx = i
		}
	}
	if originIdx < 0 {
		return fmt.Errorf("no node named %q to send from", *origin)
	}

	mode := "injected traffic; no firmware is running"
	if *real {
		if err := e.AttachNative(ctx, 4417); err != nil {
			return fmt.Errorf("%w\n\nBuild one with tools/native/build.sh, or drop -firmware "+
				"to inject traffic instead", err)
		}
		mode = fmt.Sprintf("%d nodes running a real MeshCore build", e.FirmwareCount())
	}
	fmt.Printf("%d nodes, SF%d at %.0f kHz, %.3f MHz. Sending from %s.\n%s.\n\n",
		len(nodes), *sf, *bw, *freq, nodes[originIdx].Name, mode)

	start := time.Now()
	e.Inject(originIdx, []byte("meshcoresim flood test"))
	if err := e.Run(ctx, uint32(*forMs)); err != nil {
		return err
	}
	elapsed := time.Since(start)

	reportTraffic(e, nodes, nodes[originIdx].Name, *verbose)
	fmt.Printf("\n%.1f s of simulated time in %v.\n", float64(*forMs)/1000, elapsed.Round(time.Millisecond))
	return nil
}

func reportTraffic(e *engine.Engine, nodes []scenario.Node, origin string, verbose bool) {
	events := e.Events()

	// Reasons, counted. The distinction between them is the reason the engine
	// records a cause rather than a boolean, and collapsing them here would
	// throw that away at the last step.
	reasons := map[string]int{}
	var delivered, transmitted int
	for _, ev := range events {
		switch ev.Kind {
		case "tx":
			transmitted++
		case "rx":
			delivered++
		case "miss":
			reasons[shortReason(ev)]++
		}
	}

	if verbose {
		fmt.Println("TIMELINE")
		for _, ev := range events {
			switch ev.Kind {
			case "tx":
				fmt.Printf("  %7.2fs  %-12s transmitted   %s\n", float64(ev.AtMs)/1000, ev.From, ev.Detail)
			case "rx":
				fmt.Printf("  %7.2fs  %-12s -> %-12s  %+5.1f dB  %s\n",
					float64(ev.AtMs)/1000, ev.From, ev.To, ev.SNRdB, ev.Detail)
			default:
				fmt.Printf("  %7.2fs  %-12s -x %-12s  %+5.1f dB  %s\n",
					float64(ev.AtMs)/1000, ev.From, ev.To, ev.SNRdB, ev.Detail)
			}
		}
		fmt.Println()
	}

	fmt.Printf("%d transmissions, %d receptions.\n\n", transmitted, delivered)
	if len(reasons) > 0 {
		fmt.Println("WHY PACKETS DID NOT ARRIVE")
		for reason, n := range reasons {
			fmt.Printf("  %5d  %s\n", n, reason)
		}
		fmt.Println()
	}

	fmt.Println("PER NODE")
	fmt.Printf("  %-14s %5s %6s %9s %7s  %s\n", "NODE", "SENT", "HEARD", "AIRTIME", "DUTY", "UNIQUE/REDUNDANT")
	for _, s := range e.Scoreboard() {
		flag := ""
		if s.DutyCyclePct > 1 {
			flag = "  <- over the 1% limit"
		}
		fmt.Printf("  %-14s %5d %6d %8.0fms %6.2f%%  %d / %d%s\n",
			s.Name, s.Sent, s.Heard, s.AirtimeMs, s.DutyCyclePct,
			s.UniqueDelivery, s.RedundantRelay, flag)
	}

	// Reach, which is the answer people came for.
	//
	// The sender is excluded from both counts. It has the message by
	// definition, and listing it as unreached is the kind of small wrongness
	// that makes a reader distrust the rest of the report.
	heard := map[string]bool{}
	for _, ev := range events {
		if ev.Kind == "rx" {
			heard[ev.To] = true
		}
	}
	var missing []string
	for _, n := range nodes {
		if n.Name != origin && !heard[n.Name] {
			missing = append(missing, n.Name)
		}
	}
	fmt.Printf("\n%d of %d other nodes heard the message.\n", len(heard), len(nodes)-1)
	if len(missing) > 0 {
		fmt.Printf("Never reached: %s\n", strings.Join(missing, ", "))
	}
}

func shortReason(ev engine.Event) string {
	switch {
	case strings.Contains(ev.Detail, "half duplex"):
		return "the listener's own transmitter was keyed (LoRa is half duplex)"
	case strings.Contains(ev.Detail, "interferer"):
		return "would have decoded, lost to a stronger interferer"
	case strings.Contains(ev.Detail, "terrain"):
		return "no terrain data covers the path"
	case ev.Outcome == capture.OutOfRange:
		return "nothing measurable arrived"
	default:
		return "below the demodulator's SNR floor"
	}
}

// loadNodes builds a scenario from a file, a provider, or the built-in demo.
func loadNodes(ctx context.Context, path, source, baseURL, token, board string,
	freq float64, sf int, bwKHz float64) ([]scenario.Node, error) {
	radio := scenario.RadioConfig{
		CentreHz: freq * 1e6, BandwidthHz: bwKHz * 1000,
		SpreadFactor: sf, CodingRate: 1,
	}

	if source != "" {
		var p provider.Provider
		switch strings.ToLower(source) {
		case "corescope":
			p = &provider.CoreScope{BaseURL: baseURL, Token: token}
		case "beacon":
			p = &provider.Beacon{BaseURL: baseURL, Token: token}
		default:
			return nil, fmt.Errorf("unknown source %q; have corescope, beacon", source)
		}
		if baseURL == "" {
			return nil, fmt.Errorf("-source needs -url")
		}
		records, err := p.Nodes(ctx)
		if err != nil {
			return nil, err
		}
		res, err := scenario.Import(records, scenario.ImportOptions{
			DefaultBoard: board, Radio: radio, MaxUncertaintyKm: 1,
		})
		if err != nil {
			return nil, err
		}
		fmt.Println(res.Describe())
		fmt.Println()
		out := make([]scenario.Node, 0, len(res.Nodes))
		for _, n := range res.Nodes {
			out = append(out, n.Node)
		}
		return out, nil
	}

	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		// A provider export and a scenario file are both plausible things to be
		// handed, so both are accepted rather than one being guessed at.
		var records []provider.NodeRecord
		if err := json.Unmarshal(b, &records); err == nil && len(records) > 0 {
			res, err := scenario.Import(records, scenario.ImportOptions{
				DefaultBoard: board, Radio: radio, MaxUncertaintyKm: 1,
			})
			if err != nil {
				return nil, err
			}
			fmt.Println(res.Describe())
			fmt.Println()
			out := make([]scenario.Node, 0, len(res.Nodes))
			for _, n := range res.Nodes {
				out = append(out, n.Node)
			}
			return out, nil
		}
		var nodes []scenario.Node
		if err := json.Unmarshal(b, &nodes); err != nil {
			return nil, fmt.Errorf("%s is neither a scenario nor a provider export: %w", path, err)
		}
		return nodes, nil
	}

	return demoNetwork(radio, board)
}

// demoNetwork is a chain of real Perthshire high ground, so a first run has
// something a flood can actually cross.
//
// The coordinates are local maxima found in the DEM itself rather than village
// names off a map. That distinction turned out to matter: a first attempt used
// Dunkeld, Birnam and Perth, and every path failed — correctly, because those
// are settlements in valleys and a repeater in a valley reaches nothing. Real
// repeaters go on hills, and a demo that does not is teaching the wrong lesson
// about why links fail.
func demoNetwork(radio scenario.RadioConfig, boardName string) ([]scenario.Node, error) {
	b, err := scenario.BoardByName(boardName)
	if err != nil {
		return nil, err
	}
	mast := antenna.Mounted{
		Pattern:      antenna.Collinear{GainDBiPeak: b.AntennaDBi + 4},
		Polarisation: "vertical", FeedlineDB: b.FeedlineDB,
	}
	place := func(name string, lat, lon, h float64) scenario.Node {
		return scenario.Node{
			Name: name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: lat, Lon: lon}, HeightAGLm: h,
			Antenna: mast, Radio: radio,
			TxPowerDBm: b.MaxTxDBm, NoiseFigureDB: b.NoiseFigureDB,
		}
	}
	return []scenario.Node{
		place("Ben Vrackie", 56.7520, -3.7200, 10),
		place("Craig Hill", 56.7760, -3.5880, 10),
		place("Deuchary", 56.6800, -3.9000, 10),
		place("Birnam Ridge", 56.5520, -3.8160, 10),
		place("Sidlaw", 56.4480, -3.8880, 10),
	}, nil
}
