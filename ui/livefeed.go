package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/capture"
	"github.com/A13xB0/meshcoresim/internal/mqttclient"
	"github.com/A13xB0/meshcoresim/internal/provider"
)

// liveCopy is one real transmission worth replaying: the frame, and the
// scenario node believed to have sent it.
type liveCopy struct {
	raw      []byte
	injector string // node name to inject at — matched by name, never by key
	hops     int    // how many relays the real copy had passed through
}

// liveState is the live feed: real packets, replayed into the simulated mesh
// as they happen.
//
// The mode this enables: the real network's traffic is watched at its first
// hop — the copy heard straight from the origin, or one relay later — so the
// transmitter is known, not reconstructed. Each packet is re-transmitted by
// the same-*named* simulated node (names, because the simulation's keys are
// its own and can never match the real network's), and the simulated mesh
// does the rest. What it does differently from the real one is the finding.
type liveState struct {
	cancel context.CancelFunc

	// Sources. CoreScope polls; MQTT streams when a broker is configured.
	useMQTT    bool
	brokerURL  string
	mqttTopic  string
	minPathLen int32

	mu       sync.Mutex
	running  bool
	pending  []liveCopy
	seen     map[string]bool
	err      string
	fetched  int
	injected int
	unknown  int
	direct   int
}

// livePollEvery is the CoreScope fetch cadence. Wall time, because the
// packets arrive in wall time — which is also why the feed runs at 1x.
const livePollEvery = 10 * time.Second

// drawLiveFeedBody is the Live feed panel.
func (a *App) drawLiveFeedBody() {
	s := &a.live
	if s.minPathLen == 0 {
		s.minPathLen = 2
	}

	textWrap("Replays the real network's traffic into the simulation as it happens. " +
		"Packets are taken at their first hop - from the origin, or the first " +
		"repeater to relay them - matched to the same-named node here, and " +
		"re-transmitted. The simulated mesh relays them from there.")

	if a.imp.source != "corescope" || a.imp.url == "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		textWrap("Set the Import window's source to corescope and give it a URL first.")
		imgui.PopStyleColor()
		return
	}
	if a.eng == nil || a.eng.FirmwareCount() == 0 {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		textWrap("The relays need real firmware to decide anything: press " +
			"\"run real firmware\" in the strip above first.")
		imgui.PopStyleColor()
	}

	imgui.SetNextItemWidth(90)
	imgui.InputIntV("min path bytes", &s.minPathLen, 0, 0, 0)
	if s.minPathLen < 0 {
		s.minPathLen = 0
	}
	if s.minPathLen > 8 {
		s.minPathLen = 8
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Injected frames get their path rewritten to the simulation's own\n" +
			"hashes and padded to at least this many bytes. A short path is a\n" +
			"large remaining flood budget; padding trims it, so replaying a busy\n" +
			"mesh does not become a collision storm the real network never had.")
	}

	imgui.Checkbox("also stream from MQTT", &s.useMQTT)
	if s.useMQTT {
		imgui.SetNextItemWidth(-1)
		imgui.InputTextWithHint("##broker", "tcp://broker:1883", &s.brokerURL, 0, nil)
		imgui.SetNextItemWidth(-1)
		imgui.InputTextWithHint("##topic", "topic filter (default meshcore/+/rx)", &s.mqttTopic, 0, nil)
	}
	textDimWrap("beacon publishes nodes, not packets - it cannot feed this")

	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if running {
		if imgui.Button("stop live feed") {
			s.cancel()
		}
	} else if imgui.Button("start live feed") {
		a.startLiveFeed()
	}
	imgui.SameLine()
	textDim("polls every 10 s; runs at 1x, like the traffic")

	s.mu.Lock()
	err, fetched, injected, unknown, direct := s.err, s.fetched, s.injected, s.unknown, s.direct
	s.mu.Unlock()
	if err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
		textWrap(err)
		imgui.PopStyleColor()
	}
	switch {
	case fetched > 0 || injected > 0:
		imgui.Text(fmt.Sprintf("%d first-hop packets seen, %d injected", fetched, injected))
		if unknown > 0 {
			textDim(fmt.Sprintf("%d skipped - no node here carries their sender's "+
				"name (import it, or they cannot be replayed)", unknown))
		}
		if direct > 0 {
			textDim(fmt.Sprintf("%d direct-routed packets skipped - their path is "+
				"a route through the real network's hashes, meaningless here", direct))
		}
	case running:
		textDim("watching... new origin transmissions appear here and on the map")
	default:
		textDim("not running - start, and real packets drive the simulated mesh")
	}
}

// startLiveFeed begins ingesting from every configured source.
func (a *App) startLiveFeed() {
	s := &a.live
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Lock()
	s.running = true
	s.err, s.seen = "", map[string]bool{}
	s.fetched, s.injected, s.unknown, s.direct = 0, 0, 0, 0
	s.pending = nil
	s.mu.Unlock()

	// The feed is wall-time, so the simulation must be too.
	a.speed = 1
	a.playing = true

	url, token := strings.TrimRight(a.imp.url, "/"), a.imp.token
	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		}()
		cs := &provider.CoreScope{BaseURL: url, Token: token}
		t := time.NewTicker(livePollEvery)
		defer t.Stop()
		for {
			s.pollCoreScope(ctx, cs)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	if s.useMQTT && s.brokerURL != "" {
		m := &provider.MQTT{
			Client: &mqttclient.Client{BrokerURL: s.brokerURL},
			Topic:  s.mqttTopic,
		}
		go func() {
			err := m.Subscribe(ctx, func(r provider.Reception) { s.takeReception(r) })
			if err != nil && ctx.Err() == nil {
				s.mu.Lock()
				s.err = "mqtt: " + err.Error()
				s.mu.Unlock()
			}
		}()
	}
}

// pollCoreScope fetches the newest packets and queues first-hop copies.
func (s *liveState) pollCoreScope(ctx context.Context, cs *provider.CoreScope) {
	fctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	packets, err := cs.Packets(fctx, 200, nil)
	if err != nil {
		s.mu.Lock()
		s.err = err.Error()
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = ""
	for _, p := range packets {
		s.take(p.Raw, p.Sender, p.Origin)
	}
	// The seen set grows with the session; cap it against a runaway.
	if len(s.seen) > 100_000 {
		s.seen = map[string]bool{}
	}
}

// takeReception adapts an MQTT reception into the same queue.
func (s *liveState) takeReception(r provider.Reception) {
	if r.HopCount > 1 {
		return
	}
	sender := r.Origin // hop 0: heard straight from the origin
	if r.HopCount == 1 {
		// One relay in: the receiver heard it from the first repeater, whose
		// name MQTT does not carry. The origin copy will do instead when it
		// appears; skipping beats guessing.
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.take(r.RawPayload, sender, r.Origin)
}

// take queues one copy if it is the kind this mode replays. Caller holds mu.
//
// First hop only: the copy straight from the origin (empty path) or one
// relay in (one path byte, so the first repeater is known by name). Later
// copies are the real network's routing — the thing the simulation is here
// to do for itself.
func (s *liveState) take(raw []byte, sender, origin string) {
	if len(raw) == 0 || sender == "" {
		return
	}
	d := capture.Dissect(raw)
	if d.Truncated || len(d.PathHashes) > 1 {
		return
	}
	if d.RouteName != "flood" && d.RouteName != "transport flood" {
		// A direct-routed packet's path is a route through the real network's
		// hashes; replaying it here would follow hashes that name nobody.
		s.direct++
		return
	}
	if len(d.PathHashes) == 0 && !strings.EqualFold(sender, origin) && origin != "" {
		return // a hop-0 copy must come from its origin, or the label is wrong
	}
	// Dedup on the payload alone: the same message at hop 0 and hop 1 is one
	// message, and only one copy of it gets injected.
	k := string(d.Payload)
	if s.seen[k] {
		return
	}
	s.seen[k] = true
	s.fetched++
	s.pending = append(s.pending, liveCopy{
		raw: raw, injector: sender, hops: len(d.PathHashes),
	})
}

// pumpLiveFeed injects queued frames, on the frame thread — the only thread
// allowed near the engine's node list.
func (a *App) pumpLiveFeed() {
	s := &a.live
	s.mu.Lock()
	pending := s.pending
	s.pending = nil
	minLen := int(s.minPathLen)
	s.mu.Unlock()
	if len(pending) == 0 || a.eng == nil {
		return
	}
	for _, c := range pending {
		// By name, never by key: the simulation's identities are generated
		// from the seed and can never match the real network's. The name is
		// the operator's claim about which node this is.
		idx := a.nodeIndex(c.injector)
		if idx < 0 {
			s.mu.Lock()
			s.unknown++
			s.mu.Unlock()
			continue
		}
		frame := a.rewriteForSim(c, idx, minLen)
		a.eng.InjectFrame(idx, frame)
		s.mu.Lock()
		s.injected++
		s.mu.Unlock()
	}
}

// rewriteForSim makes a recorded frame consistent with the simulated network.
//
// The recorded path bytes are the real network's hashes; here they would
// either name nobody or, one time in 256, accidentally name the wrong node
// and silence it. The path is rewritten to the injector's own simulated hash,
// repeated up to the minimum length — repeating the injector's hash is the
// one padding that cannot inhibit any *other* node's relay decision.
func (a *App) rewriteForSim(c liveCopy, idx, minLen int) []byte {
	want := c.hops
	if want < minLen {
		want = minLen
	}
	if want == 0 {
		return c.raw
	}
	pad := byte(0xEE)
	if h, ok := a.simHashFor(a.Nodes[idx].Name); ok {
		pad = h
	}
	path := make([]byte, want)
	for i := range path {
		path[i] = pad
	}
	out, err := capture.RewritePath(c.raw, path)
	if err != nil {
		return c.raw
	}
	return out
}

// simHashFor is the simulated network's hash for a node, where a run has
// revealed it. hashNames fills as nodes are seen relaying, so early injections
// may fall back to a fixed pad byte.
func (a *App) simHashFor(name string) (byte, bool) {
	for h, n := range a.hashNames {
		if strings.EqualFold(n, name) {
			return h, true
		}
	}
	return 0, false
}
