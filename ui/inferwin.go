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

	lookbackH int32
	running   bool
	fetched   atomic.Int64
	done      chan inferResult

	init     bool
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
	s.ensureDefaults()

	textWrap("Reads what each node's own traffic proves about its configuration. " +
		"Separate from the study area, which only decides which nodes are in " +
		"the scenario - an already-loaded network is fine, this reads packets, not nodes.")
	textDim("reading from " + a.imp.url)

	imgui.SetNextItemWidth(110)
	imgui.InputIntV("hours of traffic to read", &s.lookbackH, 0, 0, 0)
	if s.lookbackH < 1 {
		s.lookbackH = 1
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Newest first, back this far. Hours rather than a packet count:\n" +
			"a quiet night and a busy afternoon hold wildly different numbers of\n" +
			"packets, and coverage is what decides whether a region is confirmed.")
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
			fmt.Sprintf("reading %d h of traffic... %d packets", s.lookbackH, s.fetched.Load()))
	} else if imgui.Button("read the traffic") {
		a.startInference()
	}
	if s.done != nil {
		a.pollInference()
	}

	if s.err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
		textWrap(s.err)
		imgui.PopStyleColor()
	}
	if s.result == nil {
		return
	}

	imgui.SeparatorText(fmt.Sprintf("Found - %d packets, %d nodes, %d candidate regions",
		s.packets, len(s.result), len(s.regions)))
	scoped := 0
	for _, v := range s.result {
		if v.ScopedRelay || len(v.Regions) > 0 {
			scoped++
		}
	}
	if scoped == 0 && len(s.result) > 0 {
		textDimWrap("nothing in this traffic was transport-scoped: every packet read was " +
			"unscoped flood. That is a finding about the network, not a failure to " +
			"read it - these nodes are not using MeshCore regions.")
	}
	if len(s.regions) > 0 {
		textDim("candidates: " + strings.Join(s.regions, ", "))
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
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Writes what was read onto the scenario's nodes. Those regions\n" +
			"reach a node's firmware when it starts, which needs Provisioning's\n" +
			"region option - turned on for you when this finds anything.")
	}
	imgui.SameLine()
	textDim(fmt.Sprintf("%d applied", s.appliedN))
	if s.appliedN > 0 {
		if a.eng != nil && a.eng.FirmwareCount() > 0 {
			textDimWrap("sent to the running nodes now, and issued again whenever firmware " +
				"starts from here on")
		} else {
			textDimWrap("stored on the nodes: the region commands are issued when firmware " +
				"starts, which play does for you")
		}
	}

	if !imgui.BeginTableV("##inferred", 5,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY|
			imgui.TableFlagsResizable|imgui.TableFlagsReorderable|imgui.TableFlagsHideable,
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
		if a.nodeForObserved(name) >= 0 {
			imgui.Text("yes")
		} else {
			// Named in the traffic but absent from the scenario. Worth showing:
			// it is usually a node the import filtered out, and silently
			// dropping it from this table would hide that.
			textDim("no")
		}
		imgui.TableSetColumnIndex(2)
		if len(v.Regions) > 0 {
			imgui.Text(strings.Join(v.Regions, ", "))
		} else if v.ScopedRelay {
			textDim("scoped, unidentified")
		} else {
			textDim("-")
		}
		imgui.TableSetColumnIndex(3)
		if v.DefaultScope != "" {
			imgui.Text(v.DefaultScope)
		} else {
			textDim("-")
		}
		imgui.TableSetColumnIndex(4)
		textDim(v.Summary())
	}
	imgui.EndTable()
}

func (a *App) startInference() {
	s := &a.infer
	s.running, s.err, s.result = true, "", nil
	s.fetched.Store(0)
	s.done = make(chan inferResult, 1)

	url, token := strings.TrimRight(a.imp.url, "/"), a.imp.token
	// The scenario's CoreScope keys, for resolving path hops ourselves.
	pubkeys := make([]string, 0, len(a.Nodes))
	for i := range a.Nodes {
		if k := strings.ToLower(a.Nodes[i].PublicKey); k != "" {
			pubkeys = append(pubkeys, k)
		}
	}
	since := time.Now().Add(-time.Duration(s.lookbackH) * time.Hour)
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

		packets, err := cs.PacketsSince(ctx, since, func(n int) { s.fetched.Store(int64(n)) })
		if err != nil && len(packets) == 0 {
			ch <- inferResult{err: err}
			return
		}
		resolvePaths(packets, pubkeys)
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
		a.seedScopesFromImport()
		if len(r.nodes) == 0 {
			a.infer.err = "no packets carried a raw frame, so nothing could be read from them"
		}
	default:
	}
}

// applyInference writes what was found onto the scenario's nodes.
// applyInference writes what was found onto the scenario's nodes.
//
// Onto the *scenario*: nothing is sent to a repeater here. The regions reach
// a node when its firmware starts, through Provisioning's region commands -
// which are off by default, so applying used to update 118 nodes and issue
// not one CLI line, with nothing on any console to say so. Finding real
// regions is a good enough reason to turn that on, and to say that is what
// happened.
func (a *App) applyInference() int {
	s := &a.infer
	applied := 0
	// CoreScope names a packet's origin by public key, and only sometimes by
	// a resolved name, so the results arrive keyed by whichever it had. The
	// scenario is keyed by name. Matching on the name alone found four nodes
	// out of twenty and silently applied nothing to the rest - the same
	// "keys are identities, names are labels" lesson the merge learned.
	for name, v := range s.result {
		i := a.nodeForObserved(name)
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
	if applied > 0 {
		a.ensureConfig()
		if !a.cfg.setRegionOnStart {
			a.cfg.setRegionOnStart = true
			a.saveConfig()
		}
		running := 0
		if a.eng != nil {
			running = a.eng.FirmwareCount()
		}
		if running > 0 {
			// Firmware is already up, so the nodes would otherwise keep the
			// configuration they booted with until someone restarted them.
			sent := a.applyRegionsToFleet()
			a.status = fmt.Sprintf("%d nodes updated; region commands sent to %d running "+
				"nodes now, and issued at boot from here on", applied, sent)
		} else {
			a.status = fmt.Sprintf("%d nodes updated; their region commands will be issued "+
				"when firmware starts (Provisioning's region option is now on)", applied)
		}
	}
	return applied
}

// ensureDefaults sets what the panel would have set on its first draw.
//
// Called from the control verb as well, because a run driven from outside
// never draws the panel first - which left every "apply" tick-box false, so
// the inference ran, found twenty nodes' regions, and applied none of them
// without a word.
func (s *inferState) ensureDefaults() {
	if s.init {
		return
	}
	s.init = true
	if s.lookbackH == 0 {
		s.lookbackH = 24
	}
	s.applyRegions, s.applyScope, s.applyHops = true, true, false
}

// nodeForObserved resolves what CoreScope called a node to a node here.
//
// CoreScope identifies a node by its real public key; a simulated node
// generates its own identity at boot, because we do not have anyone's
// private key and never will. So the key that arrives with observed traffic
// can only ever be an *external reference* - the one the import stored
// against the node - or a name. Matching on the simulated node's own
// identity would match nothing, for ever.
func (a *App) nodeForObserved(ref string) int {
	if i := a.nodeIndex(ref); i >= 0 {
		return i
	}
	want := strings.ToLower(ref)
	for i := range a.Nodes {
		if strings.ToLower(a.Nodes[i].PublicKey) == want {
			return i
		}
	}
	return -1
}

// seedScopesFromImport fills in what CoreScope already published.
//
// A node's default scope is a field on its record - the SCOPE column in
// CoreScope's own list - and the import keeps it. Inferring it from adverts
// as well is fine, but a node that has been quiet, or whose adverts nobody
// observed inside the window, came back with nothing while the source knew
// all along. Observation refines what the source said; it does not replace
// it.
func (a *App) seedScopesFromImport() {
	for ref, v := range a.infer.result {
		i := a.nodeForObserved(ref)
		if i < 0 || v == nil {
			continue
		}
		n := &a.Nodes[i]
		if v.DefaultScope == "" && n.DefaultScope != "" {
			v.DefaultScope = n.DefaultScope
		}
		// A node scopes its own traffic to a region it must therefore hold.
		if v.DefaultScope != "" && !containsStr(v.Regions, v.DefaultScope) {
			v.Regions = append(v.Regions, v.DefaultScope)
		}
	}
}

// resolvePaths fills each packet's relay path from its own path hashes.
//
// CoreScope resolves some hops to names and leaves the rest as hashes, so
// trusting its resolved path silently drops every hop it could not name -
// and with them every region those nodes proved they carry. The hashes are
// prefixes of the real public keys, so they can be resolved here against the
// keys the import kept. This is HopReach's prefixIndex, and the ambiguity
// rule matters: a prefix that matches two nodes identifies neither, and
// guessing would credit a region to the wrong repeater.
func resolvePaths(packets []provider.PacketRecord, pubkeys []string) {
	if len(pubkeys) == 0 {
		return
	}
	cache := map[string]string{}
	resolve := func(prefix string) (string, bool) {
		prefix = strings.ToLower(prefix)
		if prefix == "" {
			return "", false
		}
		if v, ok := cache[prefix]; ok {
			return v, v != ""
		}
		match := ""
		for _, pk := range pubkeys {
			if strings.HasPrefix(pk, prefix) {
				if match != "" {
					cache[prefix] = ""
					return "", false // ambiguous: identifies nobody
				}
				match = pk
			}
		}
		cache[prefix] = match
		return match, match != ""
	}
	for i := range packets {
		p := &packets[i]
		seen := map[string]bool{}
		for _, n := range p.RelayPath {
			seen[strings.ToLower(n)] = true
		}
		for _, h := range p.PathHashes {
			if key, ok := resolve(h); ok && !seen[key] {
				p.RelayPath = append(p.RelayPath, key)
				seen[key] = true
			}
		}
	}
}
