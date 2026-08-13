// Bringing a real network in, and working out what it holds.
//
// The Gio build could describe an import and nothing else: it counted what a
// deployment had and then dropped it on the floor. The order below matters and
// every step in it has been skipped at least once, with the failure looking
// like bad RF rather than a missing step - which is why they are separate
// verbs rather than one button.
package session

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// importState is what has been fetched but not yet applied.
type importState struct {
	url      string
	records  []provider.NodeRecord
	imported []scenario.Node
	packets  []provider.PacketRecord
	inferred map[string]*provider.Inferred
}

func registerImport(st *state.Store, s *Sim) {
	// import.set_source: where to fetch from.
	st.Handle("import.set_source", func(w *state.World, p any) (any, error) {
		url, _ := stringField(p, "url")
		if url == "" {
			return nil, fmt.Errorf("import.set_source needs a url")
		}
		if s.imp == nil {
			s.imp = &importState{}
		}
		s.imp.url = url
		w.Say("import source: " + url)
		return map[string]any{"url": url}, nil
	})

	// import.fetch: read the deployment, and say what would change before
	// anything does.
	st.Handle("import.fetch", func(w *state.World, p any) (any, error) {
		url, _ := stringField(p, "url")
		if s.imp == nil {
			s.imp = &importState{}
		}
		if url != "" {
			s.imp.url = url
		}
		if s.imp.url == "" {
			return nil, fmt.Errorf("no import source set")
		}
		cs := &provider.CoreScope{BaseURL: s.imp.url}
		records, err := cs.Nodes(context.Background())
		if err != nil {
			return nil, err
		}
		res, err := scenario.Import(records, importOptions())
		if err != nil {
			return nil, err
		}
		s.imp.records = records
		s.imp.imported = s.imp.imported[:0]
		for _, im := range res.Nodes {
			s.imp.imported = append(s.imp.imported, im.Node)
		}
		w.Import = &state.Import{
			URL: s.imp.url, Records: len(records), Nodes: len(res.Nodes),
			SkippedNoPosition: res.SkippedNoPosition,
			SkippedOutside:    res.SkippedOutside,
			Uncertain:         res.Uncertain, Participants: res.Participants,
		}
		w.Say(fmt.Sprintf("fetched %d nodes, %d usable", len(records), len(res.Nodes)))
		return map[string]any{
			"records": len(records), "nodes": len(res.Nodes),
			"skipped_no_position": res.SkippedNoPosition,
			"uncertain":           res.Uncertain,
		}, nil
	})

	// import.commit: apply it. "replace-all" is the strategy the shipped
	// fixtures were built with; "add" keeps what is here.
	st.Handle("import.commit", func(w *state.World, p any) (any, error) {
		if s.imp == nil || len(s.imp.imported) == 0 {
			return nil, fmt.Errorf("nothing fetched to commit")
		}
		strategy, _ := stringField(p, "strategy")
		if strategy == "" {
			strategy = "replace-all"
		}
		var nodes []scenario.Node
		switch strategy {
		case "replace-all":
			nodes = append(nodes, s.imp.imported...)
		case "add":
			nodes = append(nodes, s.nodes...)
			have := map[string]bool{}
			for _, n := range s.nodes {
				have[n.Name] = true
			}
			for _, n := range s.imp.imported {
				if !have[n.Name] {
					nodes = append(nodes, n)
				}
			}
		default:
			return nil, fmt.Errorf("no strategy %q; there is replace-all and add", strategy)
		}
		s.buildSeeded(nodes, 869.618, s.seed)
		w.Nodes = stateNodes(nodes)
		w.Links = s.links()
		w.Say(fmt.Sprintf("committed %d nodes (%s)", len(nodes), strategy))
		return map[string]any{"nodes": len(nodes), "strategy": strategy}, nil
	})

	// infer.run: read the traffic and work out what each node holds.
	//
	// This is the step that gets forgotten, and it is the one that decides
	// whether anything relays.
	st.Handle("infer.run", func(w *state.World, p any) (any, error) {
		if s.imp == nil || s.imp.url == "" {
			return nil, fmt.Errorf("no import source set")
		}
		hours := 168.0
		if v, ok := numField(p, "hours"); ok && v > 0 {
			hours = v
		}
		id := "infer"
		w.Jobs = append(w.Jobs, state.Job{ID: id, What: "reading traffic", Total: 1})
		url := s.imp.url
		go func() {
			cs := &provider.CoreScope{BaseURL: url}
			_ = hours
			packets, err := cs.Packets(context.Background(), 40000, nil)
			if err != nil {
				_, _ = st.Do(context.Background(), "import.failed", err.Error())
				return
			}
			_, _ = st.Do(context.Background(), "infer.result", packets)
		}()
		return map[string]any{"reading": true, "hours": hours}, nil
	})

	st.Handle("infer.result", func(w *state.World, p any) (any, error) {
		packets, _ := p.([]provider.PacketRecord)
		if s.imp == nil {
			s.imp = &importState{}
		}
		s.imp.packets = packets
		s.imp.inferred = provider.InferFromPackets(packets, nil)
		holders := map[string]int{}
		for _, in := range s.imp.inferred {
			for _, r := range in.Regions {
				holders[r]++
			}
		}
		w.Jobs = finishJob(w.Jobs, "infer")
		w.Say(fmt.Sprintf("read %d packets, %d nodes seen", len(packets), len(s.imp.inferred)))
		return map[string]any{
			"packets": len(packets), "nodes": len(s.imp.inferred),
			"regions": holders,
		}, nil
	})

	// infer.apply: the one that is forgotten. Without it a mesh has regions
	// inferred but not applied, which transmits everything, relays nothing,
	// and reports no error at all.
	st.Handle("infer.apply", func(w *state.World, _ any) (any, error) {
		if s.imp == nil || len(s.imp.inferred) == 0 {
			return nil, fmt.Errorf("nothing inferred yet")
		}
		n := 0
		for i := range s.nodes {
			in, ok := s.imp.inferred[s.nodes[i].Name]
			if !ok || len(in.Regions) == 0 {
				continue
			}
			s.nodes[i].Regions = append([]string(nil), in.Regions...)
			if in.DefaultScope != "" {
				s.nodes[i].DefaultScope = in.DefaultScope
			}
			n++
		}
		for i := range w.Nodes {
			if in, ok := s.imp.inferred[w.Nodes[i].Name]; ok && len(in.Regions) > 0 {
				w.Nodes[i].Regions = append([]string(nil), in.Regions...)
			}
		}
		w.Say(fmt.Sprintf("applied regions to %d nodes", n))
		return map[string]any{"applied": n}, nil
	})
}

func importOptions() scenario.ImportOptions {
	return scenario.ImportOptions{
		DefaultBoard: "RAK4631",
		Radio: scenario.RadioConfig{
			CentreHz: 869.618e6, BandwidthHz: 62.5e3,
			SpreadFactor: 8, CodingRate: 4,
		},
		MaxUncertaintyKm: 1,
	}
}

// stateNodes is the interface's view of a scenario.
func stateNodes(nodes []scenario.Node) []state.Node {
	out := make([]state.Node, 0, len(nodes))
	for i, n := range nodes {
		out = append(out, state.Node{
			Name: n.Name, Kind: string(n.Kind),
			Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightM: n.HeightAGLm, TxDBm: n.TxPowerDBm,
			Regions: n.Regions, Firmware: n.Firmware.Version,
			Selected: i == 0,
		})
	}
	return out
}

func finishJob(jobs []state.Job, id string) []state.Job {
	for i := range jobs {
		if jobs[i].ID == id {
			jobs[i].Finished, jobs[i].Done = true, jobs[i].Total
		}
	}
	return jobs
}

var (
	_ = sort.Strings
	_ = strings.TrimSpace
	_ = time.Now
)
