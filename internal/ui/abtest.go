package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// abState is the A/B firmware split and the bisect over it.
type abState struct {
	verA, verB string
	running    bool
	cancel     context.CancelFunc
	done       chan string
	result     string
}

// abSimMs is how long each probe run lasts, in simulated time. Long enough
// for adverts to propagate and relays to relay; short enough that a bisect
// over forty nodes finishes while the operator is still interested.
const abSimMs = 60_000

// drawABSection is the Compare panel's A/B arm: split the fleet between two
// firmware versions, and when the outcomes differ, bisect to the node that
// carries the difference.
func (a *App) drawABSection() {
	s := &a.ab
	imgui.SeparatorText("A/B firmware split")
	imgui.TextWrapped("Give half the fleet version A and half version B, run, and see " +
		"whether behaviour differs. Then bisect: rerun with smaller and smaller " +
		"sets on B until one node carries the difference.")

	a.fw.load()
	_, versions := a.fw.forThisMachine()
	if len(versions) < 2 {
		imgui.TextDisabled("needs at least two published firmware versions to compare")
		return
	}
	if s.verA == "" {
		s.verA, s.verB = versions[0], versions[1]
	}
	// Stacked, not side by side: two 140 px combos plus labels overflowed
	// every dock narrower than a laptop, and the B combo drew half off-panel.
	pick := func(label string, v *string) {
		imgui.SetNextItemWidth(-60)
		if imgui.BeginCombo(label, *v) {
			for _, ver := range versions {
				if imgui.SelectableBool(ver + "##" + label) {
					*v = ver
				}
			}
			imgui.EndCombo()
		}
	}
	pick("A", &s.verA)
	pick("B", &s.verB)

	if imgui.Button("assign alternately") {
		n := 0
		for i := range a.Nodes {
			if !a.Nodes[i].Kind.RunsFirmware() {
				continue
			}
			if n%2 == 0 {
				a.Nodes[i].Firmware.Version = s.verA
			} else {
				a.Nodes[i].Firmware.Version = s.verB
			}
			n++
		}
		a.buildEngine()
		a.status = fmt.Sprintf("%d nodes split between %s and %s - run real firmware to compare",
			n, s.verA, s.verB)
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Alternate nodes get A and B. Watch the scoreboard and the\n" +
			"divergence readout above; the split is visible per node in the Inspector.")
	}

	imgui.SameLine()
	if s.running {
		if imgui.Button("stop bisect") && s.cancel != nil {
			s.cancel()
		}
	} else if imgui.Button("bisect A vs B") {
		a.startBisect()
	}
	if imgui.IsItemHovered() && !s.running {
		imgui.SetTooltip(fmt.Sprintf("Runs the scenario headlessly, %d s of simulated time per probe:\n"+
			"once all-A as the baseline, then halving the set of nodes on B until\n"+
			"the divergence is pinned to a node. Each probe starts real firmware,\n"+
			"so this takes minutes, in the background.", abSimMs/1000))
	}

	if s.done != nil {
		select {
		case r := <-s.done:
			s.done, s.running, s.cancel = nil, false, nil
			s.result = r
		default:
		}
	}
	if s.running {
		imgui.TextDisabled("bisecting... watch the jobs popover; cancel there or here")
	}
	if s.result != "" {
		imgui.TextWrapped(s.result)
	}
}

// startBisect runs the whole A/B bisect off the frame thread.
func (a *App) startBisect() {
	s := &a.ab
	nodes := make([]scenario.Node, len(a.Nodes))
	copy(nodes, a.Nodes)
	var names []string
	for _, n := range nodes {
		if n.Kind.RunsFirmware() {
			names = append(names, n.Name)
		}
	}
	if len(names) < 2 {
		s.result = "needs at least two firmware-running nodes"
		return
	}
	cfg := engine.Config{
		FreqMHz: a.freqMHz, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: a.runSeed(),
	}
	terrain := a.Terrain
	verA, verB := s.verA, s.verB
	stagger := a.cfg.bootSpread

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string, 1)
	s.running, s.cancel, s.done, s.result = true, cancel, done, ""

	go func() {
		defer cancel()
		// One probe: everything on A except `changed`, which runs B.
		probe := func(ctx context.Context, changed []string) ([]engine.Event, error) {
			onB := map[string]bool{}
			for _, n := range changed {
				onB[n] = true
			}
			e := engine.New(terrain, cfg)
			defer func() { _ = e.Close() }()
			e.StaggerBoot = stagger
			for _, n := range nodes {
				if n.Kind.RunsFirmware() {
					if onB[n.Name] {
						n.Firmware.Version = verB
					} else {
						n.Firmware.Version = verA
					}
				}
				e.Add(n, nil)
			}
			if err := e.AttachNative(ctx, cfg.Seed); err != nil {
				return nil, err
			}
			if err := e.Run(ctx, abSimMs); err != nil {
				return nil, err
			}
			return e.Events(), nil
		}

		baseline, err := probe(ctx, nil)
		if err != nil {
			done <- "baseline run failed: " + err.Error()
			return
		}
		allB, err := probe(ctx, names)
		if err != nil {
			done <- "all-B run failed: " + err.Error()
			return
		}
		if d := engine.Diverge(baseline, allB); !d.Found {
			done <- fmt.Sprintf("%s and %s behave identically here (%d s, seed %d) - "+
				"nothing to bisect", verA, verB, abSimMs/1000, cfg.Seed)
			return
		}

		culprits, err := engine.BisectNodes(ctx, names,
			func(ctx context.Context, changed []string) (bool, error) {
				ev, err := probe(ctx, changed)
				if err != nil {
					return false, err
				}
				return engine.Diverge(baseline, ev).Found, nil
			})
		switch {
		case err != nil:
			done <- "bisect stopped: " + err.Error()
		case len(culprits) == 1:
			done <- fmt.Sprintf("%s on %s is what changes the outcome", culprits[0], verB)
		default:
			done <- fmt.Sprintf("no single node carries it; the smallest set that does: %s",
				strings.Join(culprits, ", "))
		}
	}()
}
