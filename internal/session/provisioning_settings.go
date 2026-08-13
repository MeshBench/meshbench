// What every node is told at boot, as settings rather than as a script.
//
// The provisioning panel already shows the commands a node is sent. These are
// the switches that decide what goes into them - and each one exists because
// leaving it out produces a mesh that looks fine and behaves wrongly:
//
//   - a node without its name reports as its board type, so an event log
//     names hardware rather than places;
//   - a node without its position advertises none, and a client draws it at
//     null island;
//   - a node whose clock is wrong rejects messages as replays, which reads as
//     a radio fault;
//   - a region defined but not made the default scope means the node relays
//     for others and originates nothing.
package session

import (
	"fmt"
	"strings"

	"github.com/A13xB0/meshcoresim/internal/scenario"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// Provisioning is what a node is told when it starts.
type Provisioning struct {
	SetName     bool `json:"set_name"`
	SetPosition bool `json:"set_position"`
	SetClock    bool `json:"set_clock"`
	// RegionFromArea defines a transport region named after the study area on
	// every node, and DefaultScope makes it the one they originate under.
	RegionFromArea bool `json:"region_from_area"`
	DefaultScope   bool `json:"default_scope"`
	// AdvertHops caps how far an advert floods. Zero leaves the firmware's
	// own setting alone.
	AdvertHops int `json:"advert_hops"`
	// AdvertMinutes is how often a node says it is there, zero-hop.
	//
	// Zero, which is MeshCore's own default, and means never. Worth knowing
	// before setting it: the firmware refuses anything outside 60 to 240
	// minutes - "Error: interval range is 60-240 minutes" - so a mesh cannot
	// be made to advertise every minute to liven up a short run. Nothing in
	// either workbench originates traffic on its own; a run is made to talk by
	// asking it to, which is what the fleet's advert command is for.
	AdvertMinutes int `json:"advert_minutes"`

	// StaggerMs spreads node starts, because bringing several hundred up in
	// the same millisecond is a burst no real network ever sees.
	StaggerMs int `json:"stagger_ms"`
	// Extra is whatever this particular study needs, sent after the rest.
	Extra string `json:"extra"`
}

// DefaultProvisioning is what the shipped fixtures were built with.
func DefaultProvisioning() Provisioning {
	return Provisioning{
		SetName: true, SetPosition: true, SetClock: true,
		StaggerMs: 250,
	}
}

func registerProvisioningSettings(st *state.Store, s *Sim) {
	st.Handle("provisioning.get", func(w *state.World, _ any) (any, error) {
		return s.provisioning().describe(), nil
	})

	st.Handle("provisioning.set", func(w *state.World, p any) (any, error) {
		pr := s.provisioning()
		for name, set := range map[string]func(bool){
			"set_name":         func(v bool) { pr.SetName = v },
			"set_position":     func(v bool) { pr.SetPosition = v },
			"set_clock":        func(v bool) { pr.SetClock = v },
			"region_from_area": func(v bool) { pr.RegionFromArea = v },
			"default_scope":    func(v bool) { pr.DefaultScope = v },
		} {
			if v, ok := boolField(p, name); ok {
				set(v)
			}
		}
		if v, ok := numField(p, "advert_hops"); ok {
			pr.AdvertHops = int(v)
		}
		if v, ok := numField(p, "advert_minutes"); ok {
			pr.AdvertMinutes = int(v)
		}
		if v, ok := numField(p, "stagger_ms"); ok {
			pr.StaggerMs = int(v)
		}
		if v, ok := stringField(p, "extra"); ok {
			pr.Extra = v
		}
		s.prov = pr
		w.Provisioning, w.ProvisioningNode = nil, ""
		w.Say("provisioning changed; the next start uses it")
		return pr.describe(), nil
	})

	// provisioning.apply sends the current settings to nodes already running,
	// which is the difference between changing what a future run does and
	// changing what this one is doing.
	st.Handle("provisioning.apply", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		pr := s.provisioning()
		sent := 0
		for _, n := range s.nodes {
			en, ok := s.eng.NodeByName(n.Name)
			if !ok || en.Firmware == nil {
				continue
			}
			for _, line := range pr.commandsFor(n) {
				if err := en.Firmware.Bridge.Type([]byte(line + "\r\n")); err != nil {
					return nil, fmt.Errorf("%s: %w", n.Name, err)
				}
			}
			sent++
		}
		w.Say(fmt.Sprintf("re-provisioned %d running nodes", sent))
		return map[string]any{"nodes": sent}, nil
	})
}

func (s *Sim) provisioning() *Provisioning {
	if s.prov == nil {
		p := DefaultProvisioning()
		s.prov = &p
	}
	return s.prov
}

func (p Provisioning) describe() map[string]any {
	return map[string]any{
		"set_name": p.SetName, "set_position": p.SetPosition,
		"set_clock": p.SetClock, "region_from_area": p.RegionFromArea,
		"default_scope": p.DefaultScope, "advert_hops": p.AdvertHops,
		"stagger_ms": p.StaggerMs, "extra": p.Extra,
		"advert_minutes": p.AdvertMinutes,
	}
}

// commandsFor is what these settings send to one node, before its regions.
//
// The same list the provisioning panel shows, so what an operator reads and
// what a node is told cannot drift apart - which is the whole reason that
// panel exists.
func (p Provisioning) commandsFor(n scenario.Node) []string {
	var out []string
	if p.SetName && n.Name != "" {
		out = append(out, "set name "+n.Name)
	}
	if p.SetPosition && (n.Position.Lat != 0 || n.Position.Lon != 0) {
		out = append(out, fmt.Sprintf("set lat %.6f", n.Position.Lat),
			fmt.Sprintf("set lon %.6f", n.Position.Lon))
	}
	if p.SetClock {
		// The run's own clock, the same on every node. A node whose clock
		// disagrees rejects messages as replays, which reads as a radio fault.
		//
		// A real epoch rather than zero, and the same one the old workbench
		// uses. MeshCore timestamps what it sends and judges freshness by it;
		// a node that believes it is 1970 is not a node in the same
		// conversation as the rest. Fixed rather than wall clock, so a run
		// stays reproducible.
		out = append(out, fmt.Sprintf("time %d", scenarioEpoch))
	}
	// Clamped to what the firmware accepts, rather than sent and refused into
	// a console nobody is reading.
	if p.AdvertMinutes > 0 && n.Kind.Transmits() {
		mins := p.AdvertMinutes
		if mins < 60 {
			mins = 60
		}
		if mins > 240 {
			mins = 240
		}
		out = append(out, fmt.Sprintf("set advert.interval %d", mins))
	}
	if p.AdvertHops > 0 {
		out = append(out, fmt.Sprintf("set advert.hops %d", p.AdvertHops))
	}
	if p.Extra != "" {
		for _, line := range strings.Split(p.Extra, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				out = append(out, line)
			}
		}
	}
	return out
}

// scenarioEpoch is the clock every node is set to at boot: 2026-09-01T00:00:00Z,
// comfortably after the 1.17 release. The same constant the old workbench uses,
// so a run started in either lands on the same instant.
const scenarioEpoch = 1788220800
