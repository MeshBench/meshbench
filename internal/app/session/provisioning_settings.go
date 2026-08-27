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

	"github.com/MeshBench/meshbench/internal/world/scenario"
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
	// FloodMaxAdvert caps how far an advert is relayed. The firmware ships
	// with 8, which is generous on a town and short on a country: a node
	// whose advert never arrives is a node nobody can route to. Zero leaves
	// the firmware alone.
	FloodMaxAdvert int `json:"flood_max_advert"`
	// PathHashMode, LoopDetect and CadMode are the firmware's own switches,
	// passed through for studies that vary them. -1 and empty leave the
	// firmware's defaults alone.
	PathHashMode int    `json:"path_hash_mode"`
	LoopDetect   string `json:"loop_detect"`
	CadMode      string `json:"cad"`

	// CompPathHashMode is the companion's own, which is a different question
	// from the repeaters'. What a message carries is stamped by whoever
	// originated it and honoured at every hop; a repeater's setting only
	// affects the adverts it originates. A study usually varies this one and
	// holds the other. Negative leaves it to PathHashMode.
	CompPathHashMode int `json:"comp_path_hash_mode"`
	// Extra is whatever this particular study needs, sent after the rest.
	Extra string `json:"extra"`
}

// DefaultProvisioning is what the shipped fixtures were built with - the same
// values the old workbench applies at attach, because a run moved between the
// two must be the same run.
func DefaultProvisioning() Provisioning {
	return Provisioning{
		SetName: true, SetPosition: true, SetClock: true,
		StaggerMs: 250,
		// 32 against the firmware's default of 8, matching the old
		// workbench: the cost is airtime on adverts, which are small and
		// infrequent; the alternative is unroutable nodes on national runs.
		FloodMaxAdvert:   32,
		PathHashMode:     -1,
		CompPathHashMode: -1,
	}
}

func (s *Sim) provisioning() *Provisioning {
	if s.prov == nil {
		p := DefaultProvisioning()
		s.prov = &p
	}
	return s.prov
}

func (p Provisioning) Describe() map[string]any {
	return map[string]any{
		"set_name": p.SetName, "set_position": p.SetPosition,
		"set_clock": p.SetClock, "region_from_area": p.RegionFromArea,
		"default_scope": p.DefaultScope, "advert_hops": p.AdvertHops,
		"stagger_ms": p.StaggerMs, "extra": p.Extra,
		"advert_minutes":   p.AdvertMinutes,
		"flood_max_advert": p.FloodMaxAdvert, "path_hash_mode": p.PathHashMode,
		"loop_detect": p.LoopDetect, "cad": p.CadMode,
		"comp_path_hash_mode": p.CompPathHashMode,
	}
}

// pathHashFor is the path-hash mode this node should be told, which is not the
// same question for a companion as for a repeater. Negative means say nothing.
func (p Provisioning) pathHashFor(n scenario.Node) int {
	if n.Kind == scenario.Companion && p.CompPathHashMode >= 0 {
		return p.CompPathHashMode
	}
	return p.PathHashMode
}

// commandsFor is what these settings send to one node, before its regions.
//
// The same list the provisioning panel shows, so what an operator reads and
// what a node is told cannot drift apart - which is the whole reason that
// panel exists.
func (p Provisioning) CommandsFor(n scenario.Node) []string {
	var out []string
	if p.SetName && n.Name != "" {
		// Truncated to the firmware's own field width, on a rune boundary:
		// sending more is not an error the CLI reports - it stores the first
		// part - and ScotMesh names carry emoji, so a byte cut would hand the
		// firmware half a UTF-8 sequence.
		out = append(out, "set name "+truncateRunes(n.Name, maxNodeNameRunes))
	}
	if mode := p.pathHashFor(n); mode >= 0 {
		out = append(out, fmt.Sprintf("set path.hash.mode %d", mode))
	}
	if p.LoopDetect != "" && n.Kind.Transmits() {
		out = append(out, "set loop.detect "+p.LoopDetect)
	}
	if p.CadMode != "" && n.Kind.Transmits() {
		out = append(out, "set cad "+p.CadMode)
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
	if p.FloodMaxAdvert > 0 && n.Kind.Transmits() {
		out = append(out, fmt.Sprintf("set flood.max.advert %d", p.FloodMaxAdvert))
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

// maxNodeNameRunes is MeshCore's own node_name field width, the same constant
// the old workbench truncates to.
const maxNodeNameRunes = 32

// truncateRunes cuts on a rune boundary, never inside one.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
