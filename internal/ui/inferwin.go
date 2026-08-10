package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/provider"
)

// inferState is the Infer window: what to read from observed traffic, and what
// it found.
type inferState struct {
	// What to take from the result. Ticked separately because they are
	// different claims — a cryptographically confirmed region is not the same
	// kind of fact as "never seen relaying unscoped", and an operator should be
	// able to accept one without the other.
	applyRegions bool
	applyScope   bool
	applyHops    bool

	// extraRegions are names an operator knows about that CoreScope does not
	// list. A region nobody names cannot be identified.
	extraRegions string

	maxPackets int32
	running    bool
	fetched    atomic.Int64
	done       chan inferResult

	result   map[string]*provider.Inferred
	regions  []string
	packets  int
	err      string
	appliedN int
}

type inferResult struct {
	nodes   map[string]*provider.Inferred
	regions []string
	packets int
	err     error
}

// drawInferBody reads a live deployment's configuration off its own traffic.
// It lives inside the Import window: import-then-infer is one workflow.
//
// The alternative is asking each node, and a repeater's self-reported default
// scope is empty for most of them — HopReach measured about three quarters of
// real repeaters. What a node actually relays is not optional and cannot be
// stale, so behaviour is the better source.
func (a *App) drawInferBody() {
	s := &a.infer
	if s.maxPackets == 0 {
		s.maxPackets = 5000
		s.applyRegions, s.applyScope, s.applyHops = true, true, false
	}

	imgui.TextWrapped("Reads what each node's own traffic proves about its configuration. " +
		"Separate from the study area, which only decides which nodes are in " +
		"the scenario - an already-loaded network is fine, this reads packets, not nodes.")
	imgui.TextDisabled("reading from " + a.imp.url)

	imgui.SetNextItemWidth(120)
	imgui.InputInt("packets to walk", &s.maxPackets)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Newest first. More packets means more nodes seen and more\n" +
			"regions confirmed; a quiet network needs a longer walk.")
	}

	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##extraregions", "region names you know of, comma separated",
		&s.extraRegions, 0, nil)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Added to the ones CoreScope lists. A region's key is not in the\n" +
			"packet, so a name is the only way to check a code against it - one\n" +
			"nobody names shows as scoped but unidentified.")
	}

	if s.running {
		imgui.ProgressBarV(-1*float32(imgui.Time()), imgui.NewVec2(-1, 0),
			fmt.Sprintf("walking... %d packets", s.fetched.Load()))
	} else if imgui.Button("read the traffic") {
		a.startInference()
	}
	if s.done != nil {
		a.pollInference()
	}

	if s.err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
		imgui.TextWrapped(s.err)
		imgui.PopStyleColor()
	}
	if s.result == nil {
		return
	}

	imgui.SeparatorText(fmt.Sprintf("Found - %d packets, %d nodes, %d candidate regions",
		s.packets, len(s.result), len(s.regions)))
	if len(s.regions) > 0 {
		imgui.TextDisabled("candidates: " + strings.Join(s.regions, ", "))
	}

	imgui.Checkbox("regions each node holds", &s.applyRegions)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Cryptographically confirmed: the packet's transport code is an\n" +
			"HMAC over its payload under the region's key, so a match is proof\n" +
			"rather than a guess.")
	}
	imgui.SameLine()
	imgui.Checkbox("default scope", &s.applyScope)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("From what a node originates - its own adverts - because that is\n" +
			"what a default scope governs.")
	}
	imgui.SameLine()
	imgui.Checkbox("flood.max lower bound", &s.applyHops)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("The largest hop count a node was seen to relay. A lower bound\n" +
			"only: it cannot be below what it has already done, but it may be\n" +
			"far higher.")
	}

	if imgui.Button("apply to the matching nodes") {
		s.appliedN = a.applyInference()
	}
	imgui.SameLine()
	imgui.TextDisabled(fmt.Sprintf("%d applied", s.appliedN))

	if !imgui.BeginTableV("##inferred", 5,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	for _, h := range []string{"node", "in scenario", "regions", "default scope", "what it did"} {
		imgui.TableSetupColumnV(h, imgui.TableColumnFlagsWidthStretch, 0, 0)
	}
	imgui.TableHeadersRow()

	names := make([]string, 0, len(s.result))
	for n := range s.result {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		v := s.result[name]
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(name)
		imgui.TableSetColumnIndex(1)
		if a.nodeIndex(name) >= 0 {
			imgui.Text("yes")
		} else {
			// Named in the traffic but absent from the scenario. Worth showing:
			// it is usually a node the import filtered out, and silently
			// dropping it from this table would hide that.
			imgui.TextDisabled("no")
		}
		imgui.TableSetColumnIndex(2)
		if len(v.Regions) > 0 {
			imgui.Text(strings.Join(v.Regions, ", "))
		} else if v.ScopedRelay {
			imgui.TextDisabled("scoped, unidentified")
		} else {
			imgui.TextDisabled("-")
		}
		imgui.TableSetColumnIndex(3)
		if v.DefaultScope != "" {
			imgui.Text(v.DefaultScope)
		} else {
			imgui.TextDisabled("-")
		}
		imgui.TableSetColumnIndex(4)
		imgui.TextDisabled(v.Summary())
	}
	imgui.EndTable()
}

func (a *App) startInference() {
	s := &a.infer
	s.running, s.err, s.result = true, "", nil
	s.fetched.Store(0)
	s.done = make(chan inferResult, 1)

	url, token := strings.TrimRight(a.imp.url, "/"), a.imp.token
	max := int(s.maxPackets)
	extra := s.extraRegions
	ch := s.done
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cs := &provider.CoreScope{BaseURL: url, Token: token}

		// The names CoreScope knows, plus any the operator supplied. A failure
		// here is not fatal: behaviour is still inferable without names, and
		// saying so beats refusing to run.
		names, err := cs.Regions(ctx)
		if err != nil {
			names = nil
		}
		for _, n := range strings.Split(extra, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}

		packets, err := cs.Packets(ctx, max, func(n int) { s.fetched.Store(int64(n)) })
		if err != nil && len(packets) == 0 {
			ch <- inferResult{err: err}
			return
		}
		ch <- inferResult{
			nodes:   provider.InferFromPackets(packets, provider.NewNamedRegions(names)),
			regions: names, packets: len(packets),
		}
	}()
}

func (a *App) pollInference() {
	select {
	case r := <-a.infer.done:
		a.infer.done, a.infer.running = nil, false
		if r.err != nil {
			a.infer.err = r.err.Error()
			return
		}
		a.infer.result, a.infer.regions, a.infer.packets = r.nodes, r.regions, r.packets
		if len(r.nodes) == 0 {
			a.infer.err = "no packets carried a raw frame, so nothing could be read from them"
		}
	default:
	}
}

// applyInference writes what was found onto the scenario's nodes.
func (a *App) applyInference() int {
	s := &a.infer
	applied := 0
	for name, v := range s.result {
		i := a.nodeIndex(name)
		if i < 0 {
			continue
		}
		n := &a.Nodes[i]
		changed := false
		if s.applyRegions && len(v.Regions) > 0 {
			n.Regions = append(n.Regions[:0], v.Regions...)
			changed = true
		}
		if s.applyScope && v.DefaultScope != "" {
			n.DefaultScope = v.DefaultScope
			changed = true
		}
		if s.applyHops && v.MaxHops > 0 {
			n.FloodMaxSeen = v.MaxHops
			changed = true
		}
		if changed {
			applied++
		}
	}
	return applied
}
