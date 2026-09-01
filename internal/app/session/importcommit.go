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

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/provider"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// importState is what has been fetched but not yet applied.
type importState struct {
	url      string
	records  []provider.NodeRecord
	imported []scenario.Node
	packets  []provider.PacketRecord
	inferred map[string]*provider.Inferred
}

// inferReading is how the reading goroutine hands its work back to the store:
// the packets it read and the matcher that names their regions, both prepared
// off the store thread because one is a large read and the other a network
// call. A matcher of nil is honest - the regions stay unnamed rather than
// guessed - but the CoreScope path always supplies one.
type inferReading struct {
	packets []provider.PacketRecord
	matcher provider.RegionMatcher
}

func registerImport(st *state.Store, s *Sim) {
	st.HandleSpec("import.set_source", state.Spec{
		What: "name the CoreScope deployment every later import and inference " +
			"verb reads from, so they need not be told twice",
		Params: []state.Param{
			{Name: "url", Type: state.ParamString, Required: true, Primary: true,
				What: "the deployment's base URL; an empty one is refused, and a " +
					"trailing slash is trimmed rather than passed on"},
		},
		Returns: []string{"url"},
		Answers: "The URL that comes back is the trimmed one, which is what the " +
			"fetch will actually ask.",
		Example: &state.Example{
			Params:   map[string]any{"url": "https://map.example.net"},
			What:     "point the import at a deployment",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		url, _ := stringField(p, "url")
		if url == "" {
			return nil, fmt.Errorf("import.set_source needs a url")
		}
		// A trailing slash is what anybody pastes from a browser, and the
		// provider joins paths with a bare "/", so it asks for
		// //api/receptions and gets the site's HTML back. Trimmed here rather
		// than in the provider, because this is where a person's typing
		// arrives.
		url = strings.TrimRight(url, "/")
		if s.imp == nil {
			s.imp = &importState{}
		}
		s.imp.url = url
		w.Say("import source: " + url)
		return map[string]any{"url": url}, nil
	})

	st.HandleSpec("import.fetch", state.Spec{
		What: "read a deployment's nodes and say what committing them would " +
			"change, before anything in the scenario changes",
		Params: []state.Param{
			{Name: "url", Type: state.ParamString, Primary: true,
				What: "the deployment to read, remembered as the source; absent, " +
					"the source already set is used, and the fetch is refused " +
					"when there is none"},
		},
		Returns: []string{"records", "nodes", "skipped_no_position", "uncertain"},
		Answers: "`records` is what the deployment published and `nodes` what " +
			"survived: a node with no position is dropped, and one outside an " +
			"accepted study area and its margin is dropped too. `uncertain` " +
			"counts the ones placed more loosely than a kilometre, which are " +
			"kept and marked rather than dropped, because a node imported at " +
			"plus or minus 5 km cannot be given a confident answer and the " +
			"mark is what carries that. Nothing is committed until " +
			"import.commit.",
		Example: &state.Example{
			Params:   map[string]any{"url": "https://map.example.net"},
			What:     "see what a deployment holds before taking it",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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
		res, err := scenario.Import(records, importOptions(s, float64(w.MarginKm)))
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

	st.HandleSpec("import.commit", state.Spec{
		What: "make the fetched nodes the scenario, either in place of what is " +
			"loaded or alongside it, and start measuring the links again",
		Params: []state.Param{
			{Name: "strategy", Type: state.ParamString, Primary: true,
				What: "\"replace-all\" for the imported network on its own, " +
					"\"add\" to keep what is already loaded and add the names it " +
					"has not got; absent it is replace-all, and anything else is " +
					"refused - \"replace\" is not a strategy name, and a caller " +
					"who writes it gets an error rather than a network with the " +
					"demonstration nodes still in it"},
		},
		Returns: []string{"nodes", "strategy"},
		Answers: "`nodes` is the size of the scenario afterwards, not how many " +
			"arrived. It returns before the links are measured: every pair is a " +
			"path loss over real terrain, so that runs as a job and the link " +
			"matrix is empty until it finishes.",
		Example: &state.Example{
			Params:   map[string]any{"strategy": "replace-all"},
			What:     "make the imported deployment the whole scenario",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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
		// Links as a job, not here.
		//
		// Every pair is a path loss over real terrain: 676 imported nodes is
		// 228,000 of them, which is minutes. Computing that inside the handler
		// blocks the store goroutine, so nothing draws and no other verb is
		// answered - and from outside a commit that takes four minutes and a
		// commit that hung are the same thing.
		w.Links = nil
		s.warm(st, len(nodes))
		w.Say(fmt.Sprintf("committed %d nodes (%s); measuring links", len(nodes), strategy))
		return map[string]any{"nodes": len(nodes), "strategy": strategy}, nil
	})

	st.HandleSpec("infer.run", state.Spec{
		What: "read a window of the deployment's own traffic and work out which " +
			"regions each node holds, which is the only honest source for what " +
			"a real mesh forwards",
		Params: []state.Param{
			{Name: "hours", Type: state.ParamNumber, Primary: true,
				What: "how far back to read, in hours; anything not positive " +
					"leaves it at 168, a week"},
		},
		Returns: []string{"reading", "hours"},
		Answers: "It returns as soon as the read is started, not when it " +
			"finishes: the packets come back later through the reader's own " +
			"callback, which ends the `infer` job and reports how many nodes " +
			"were seen, and a failed read ends the same job. It is refused " +
			"when no import source has been set. Nothing reaches the nodes " +
			"until infer.apply.",
		Example: &state.Example{
			Params:   map[string]any{"hours": 168},
			What:     "read a week of traffic to see what each node relays",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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
		// The window is what the caller asked for, and until recently it was
		// accepted, echoed back and then dropped: every import read the most
		// recent 40,000 packets whatever it said. On ScotMesh that is under two
		// days, so a week's worth of regions came back missing the quiet ones and
		// the answer looked like a mesh that had gone silent rather than a
		// truncated read.
		since := time.Now().Add(-time.Duration(hours * float64(time.Hour)))
		go func() {
			cs := &provider.CoreScope{BaseURL: url}
			progress := func(n int) {
				_, _ = st.Do(context.Background(), "infer.progress", n)
			}
			packets, err := cs.PacketsSince(context.Background(), since, progress)
			if err != nil {
				_, _ = st.Do(context.Background(), "import.failed", err.Error())
				return
			}
			// Name the regions, not merely detect that traffic was scoped: the
			// candidates are the regions CoreScope has seen, and a transport
			// code checked against a candidate's key turns "relays something
			// scoped" into "relays #ioi". Without this a whole import comes back
			// with every node region-less, which transmits everything and
			// relays nothing - the silence the shipped fixtures were missing.
			// Fetched here, off the store thread, because it is a network call;
			// best-effort, since an unnamed region is still honestly reported as
			// scoped rather than guessed.
			var matcher provider.RegionMatcher
			if names, rerr := cs.Regions(context.Background()); rerr == nil && len(names) > 0 {
				matcher = provider.NewNamedRegions(names)
			}
			_, _ = st.Do(context.Background(), "infer.result",
				inferReading{packets: packets, matcher: matcher})
		}()
		return map[string]any{"reading": true, "hours": hours}, nil
	})

	st.HandleInternalSpec("infer.progress", state.Spec{
		What: "carry the running packet count from the reading goroutine into " +
			"the traffic job, so a long read shows movement rather than a bar " +
			"that has stopped",
		Answers: "Nothing: it updates the job and returns nil.",
	}, func(w *state.World, p any) (any, error) {
		n, ok := p.(int)
		if !ok {
			return nil, wrongCallback("infer.progress")
		}
		for i := range w.Jobs {
			if w.Jobs[i].ID == "infer" {
				// No denominator: the walk ends on a timestamp rather than a
				// count, so a percentage here would be invented.
				w.Jobs[i].Done = n
				w.Jobs[i].What = fmt.Sprintf("reading traffic, %d packets", n)
			}
		}
		return nil, nil
	})

	// infer.result is how the reading goroutine hands its packets back. It was
	// reachable from the socket like every other verb, and from out there it
	// arrived with no packets: the version that ignored that replaced a
	// completed inference with an empty one, so a mesh that had just been
	// imported correctly went silent and nothing said why. The socket refuses
	// it now; the check stays, for a caller in here that gets it wrong.
	st.HandleInternalSpec("infer.result", state.Spec{
		What: "turn the packets the reader collected into the per-node region " +
			"inference and end the traffic job",
		Returns: []string{"packets", "nodes", "regions"},
		Answers: "`regions` is a map of region name to how many nodes hold it. " +
			"The inference is held for infer.apply and reaches no node until " +
			"that is called.",
	}, func(w *state.World, p any) (any, error) {
		r, ok := p.(inferReading)
		if !ok {
			return nil, badParams("infer.result is the reader's own callback; " +
				"use infer.run to start a read")
		}
		if s.imp == nil {
			s.imp = &importState{}
		}
		s.imp.packets = r.packets
		s.imp.inferred = provider.InferFromPackets(r.packets, r.matcher)
		holders := map[string]int{}
		for _, in := range s.imp.inferred {
			for _, rn := range in.Regions {
				holders[rn]++
			}
		}
		w.Jobs = finishJob(w.Jobs, "infer")
		w.Say(fmt.Sprintf("read %d packets, %d nodes seen", len(r.packets), len(s.imp.inferred)))
		return map[string]any{
			"packets": len(r.packets), "nodes": len(s.imp.inferred),
			"regions": holders,
		}, nil
	})

	st.HandleSpec("infer.apply", state.Spec{
		What: "write the inferred regions onto the nodes, which is the step " +
			"that gets forgotten and the one that decides whether anything " +
			"relays: without it a mesh has regions inferred and not applied, " +
			"which transmits everything, relays nothing and reports no error",
		Returns: []string{"applied"},
		Answers: "`applied` is how many nodes were written to, and 0 is the " +
			"answer worth reading: the inference ran and nothing was written " +
			"back. It matches on the public key a node kept from the feed and " +
			"falls back to the name, so it only reaches nodes that were seen " +
			"on the real network. It is refused outright when nothing has been " +
			"inferred yet.",
		Example: &state.Example{
			Params:   map[string]any{},
			What:     "apply what the traffic proved about which node relays what",
			Runnable: false,
		},
	}, func(w *state.World, _ any) (any, error) {
		if s.imp == nil || len(s.imp.inferred) == 0 {
			return nil, fmt.Errorf("nothing inferred yet")
		}
		// A packet names the nodes on its path by public key - that is what
		// CoreScope's resolved_path carries - so the inference is keyed by key,
		// not by the display name. Match on the key an imported node kept from
		// the feed, and fall back to the name for anything placed by hand or a
		// feed that resolved a name. Matching on name alone credited almost
		// nobody: a whole import came back with a handful of regions where the
		// traffic proved hundreds.
		find := func(name, key string) (*provider.Inferred, bool) {
			if key != "" {
				if in, ok := s.imp.inferred[key]; ok && len(in.Regions) > 0 {
					return in, true
				}
			}
			if in, ok := s.imp.inferred[name]; ok && len(in.Regions) > 0 {
				return in, true
			}
			return nil, false
		}
		n := 0
		for i := range s.nodes {
			in, ok := find(s.nodes[i].Name, s.nodes[i].PublicKey)
			if !ok {
				continue
			}
			s.nodes[i].Regions = append([]string(nil), in.Regions...)
			if in.DefaultScope != "" {
				s.nodes[i].DefaultScope = in.DefaultScope
			}
			n++
		}
		// The snapshot's own nodes carry no key, so match them by name; the
		// saved fixture comes from the scenario nodes above, which is where the
		// key match matters. This keeps the live view roughly in step.
		for i := range w.Nodes {
			if in, ok := find(w.Nodes[i].Name, ""); ok {
				w.Nodes[i].Regions = append([]string(nil), in.Regions...)
			}
		}
		w.Say(fmt.Sprintf("applied regions to %d nodes", n))
		return map[string]any{"applied": n}, nil
	})
}

func importOptions(s *Sim, marginKm float64) scenario.ImportOptions {
	o := scenario.ImportOptions{
		DefaultBoard: "RAK_4631",
		Radio: scenario.RadioConfig{
			CentreHz: 869.618e6, BandwidthHz: 62.5e3,
			SpreadFactor: 8, CodingRate: 4,
		},
		MaxUncertaintyKm: 1,
	}
	// The study area, if one has been accepted, applied here rather than
	// afterwards.
	//
	// The importer has taken a Region since it was written and workbench1
	// passed one; this stopped passing it, so a national feed imported whole
	// and the only way to narrow it was to commit all of it and prune. That
	// is not the same cost: committing 454 nodes measures every pair against
	// the terrain and the buildings - tens of thousands of link profiles -
	// and then throws most of the answer away. Applied at the import, the
	// work is only ever done for the nodes that were wanted.
	o.Region = s.importRegion(marginKm)
	return o
}

// importRegion is the accepted study area as the importer wants it, or nil
// when none has been accepted and every node should come in.
func (s *Sim) importRegion(marginKm float64) *scenario.Region {
	if len(s.areas) == 0 {
		return nil
	}
	if marginKm <= 0 {
		marginKm = scenario.DefaultMarginKm
	}
	// Participates, not Contains: a repeater just outside the border still
	// relays to and interferes with the nodes inside, and dropping it makes
	// the mesh behave better than reality. Those arrive as participants.
	return &scenario.Region{Boundaries: s.areas, MarginKm: marginKm}
}

// stateNodes is the interface's view of a scenario.
func stateNodes(nodes []scenario.Node) []state.Node {
	need := buildsNeedingCards()
	out := make([]state.Node, 0, len(nodes))
	for i, n := range nodes {
		slot := boardHasCardSlot(n.Board)
		required := need[n.Firmware.Board+"\x00"+n.Firmware.Version] && slot
		out = append(out, state.Node{
			CardSlot: slot, CardRequired: required,
			CardFitted: n.HasCard(slot, required), CardFile: cardFileFor(n),
			CardShared: n.CardFile != "",
			Name:       n.Name, Kind: string(n.Kind),
			Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightM: n.HeightAGLm, TxDBm: n.TxPowerDBm,
			Regions: n.Regions, DefaultScope: n.DefaultScope,
			Firmware: n.Firmware.Version, Board: n.Firmware.Board,
			Hardware: n.Board,
			Selected: i == 0,
			// The antenna as well as everything else. This converter serves
			// import, placement and the bulk verbs, and left without them a
			// network that had just been imported drew no pattern and offered
			// no antenna to edit, while the same network loaded from a fixture
			// did both.
			Pattern: patternOf(n),
			Antenna: antennaOf(n),
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
