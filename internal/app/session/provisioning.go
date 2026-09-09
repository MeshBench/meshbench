// What a node is told before a run.
//
// Provisioning is a list of console lines. It has always been a list of console
// lines - fixture.RegionCommands has produced them for two programs since the
// region spelling trap was paid for twice - but nothing ever showed them, so
// "what did we actually send this node" was answerable only by reading the
// source.
//
// This makes the list the thing: generated once, shown, and sent. Not a
// description of what the code does, which would drift from it; the same
// []string, produced here and consumed by whoever brings a node up.
package session

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/fixture"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// ProvisioningFor is the script for one node, with each line's reason.
//
// The reason travels with the line because the failure this exists to prevent
// is silent: a region that is defined but not allowed to flood relays nothing
// and reports no error, which looks like broken RF rather than a missing line.
func ProvisioningFor(n scenario.Node) []state.ProvisionLine {
	return provisioningWith(DefaultProvisioning(), n)
}

// provisioningFor is the script under this session's own settings.
//
// The settings the operator changed, rather than the defaults. Everything sent
// at start went through the defaults, so the Provisioning panel could be set
// to anything at all and the nodes were told the same thing regardless - which
// is worse than the panel not existing.
func (s *Sim) provisionLines(n scenario.Node) []state.ProvisionLine {
	return provisioningWith(*s.provisioning(), n)
}

// provisionLinesFor is the script for one cell of an experiment: the session's
// settings, with whatever this arm names written over them.
func (s *Sim) provisionLinesFor(n scenario.Node, arm ExpArm) []state.ProvisionLine {
	prov := *s.provisioning()
	arm.ApplyOver(&prov)
	return provisioningWith(prov, n)
}

// provisioningWith is the script under a stated set of settings.
func provisioningWith(prov Provisioning, n scenario.Node) []state.ProvisionLine {
	var out []state.ProvisionLine

	out = append(out, state.ProvisionLine{
		Command: fmt.Sprintf("# %s: %s, %s", n.Name, n.Kind, n.Firmware.Version),
		Why:     "which build runs, chosen when the node launches rather than after",
		Comment: true,
	})

	// The session's own settings first: name, position, clock, advert cap.
	// They are what the node is told before anything about regions, and
	// showing them here is the point of this panel.
	for _, c := range prov.CommandsFor(n) {
		out = append(out, state.ProvisionLine{
			Command: c, Why: whyProvision(c),
		})
	}
	for _, c := range fixture.RegionCommands(n) {
		why := "defines a region this node carries"
		switch {
		case c == "region save":
			why = "commits the set to the node's own storage, which is why a node " +
				"that has run before ignores a changed compiled default"
		case len(c) > 14 && c[:14] == "region default":
			why = "the scope this node originates under when nothing says otherwise"
		case c == "region allowf *":
			why = "the wildcard's flood permission, which a fresh node already " +
				"has: this changes nothing unless it was told region denyf *, " +
				"and never makes a scoped flood forward"
		case len(c) > 13 && c[:13] == "region allowf":
			why = "permits flooding for that region - a region defined but not " +
				"allowed relays nothing and reports no error"
		}
		out = append(out, state.ProvisionLine{Command: c, Why: why})
	}

	if len(out) == 1 {
		out = append(out, state.ProvisionLine{
			Command: "# nothing to send",
			Why: "this node carries no regions, so it relays every unscoped " +
				"flood, adverts included, and drops every scoped one without a word",
			Comment: true,
		})
	}
	return out
}

// provisioningFor looks a node up by name.
func (s *Sim) provisioningFor(name string) ([]state.ProvisionLine, error) {
	for _, n := range s.nodes {
		if n.Name == name {
			return s.provisionLines(n), nil
		}
	}
	return nil, noSuchNode(name)
}

// whyProvision is the reason a session-settings line exists, each of which is
// a failure somebody has had.
func whyProvision(cmd string) string {
	switch {
	case strings.HasPrefix(cmd, "set name"):
		return "without it a node reports as its board type, so an event log " +
			"names hardware rather than places"
	case strings.HasPrefix(cmd, "set lat"), strings.HasPrefix(cmd, "set lon"):
		return "a node advertises the position it was told; without one a client " +
			"draws it at null island"
	case strings.HasPrefix(cmd, "time"):
		return "a node whose clock disagrees rejects messages as replays, which " +
			"reads as a radio fault"
	case strings.HasPrefix(cmd, "set advert.hops"):
		return "caps how far an advert floods"
	case strings.HasPrefix(cmd, "set flood.max.advert"):
		return "how far an advert is relayed - the firmware ships with 8, " +
			"short on a national mesh, and a node whose advert never arrives " +
			"is one nobody can route to"
	case strings.HasPrefix(cmd, "set path.hash.mode"):
		return "the firmware's own path-hash switch, varied by studies"
	case strings.HasPrefix(cmd, "set loop.detect"):
		return "the firmware's loop detection level, varied by studies"
	case strings.HasPrefix(cmd, "set cad"):
		return "channel-activity detection, varied by studies"
	}
	return "from this session's settings"
}

// ApplyRegionsLive re-provisions a running node's region map to match the
// scenario, and reports whether it did. A node whose firmware is not up needs
// nothing: it is provisioned from the scenario when it starts.
//
// A region set on a running node was written to the scenario and the map and
// nothing was sent, so the node relayed under whatever it was provisioned with
// at boot and nothing said so. The lines here are the ones a boot sends - put
// and allowf for each region, save, and the default scope - preceded by a
// remove for every region the node held and no longer should, which a boot
// never issues because it starts from an empty map.
func (s *Sim) ApplyRegionsLive(name string, old []string, n scenario.Node) bool {
	if s.Engine() == nil {
		return false
	}
	en, ok := s.Engine().NodeByName(name)
	if !ok || en.Firmware == nil {
		return false
	}
	cmds := liveRegionCommands(old, n)
	if len(cmds) == 0 {
		return false
	}
	for _, cmd := range cmds {
		if err := en.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
			break
		}
	}
	return true
}

// liveRegionCommands is the console script that moves a running node from the
// regions it held to the ones it should: a remove for each region dropped,
// then the put, allowf, save and default a boot would send. A boot never
// removes because it starts from an empty map.
func liveRegionCommands(old []string, n scenario.Node) []string {
	held := map[string]bool{}
	for _, r := range n.Regions {
		held[strings.TrimPrefix(r, "#")] = true
	}
	var cmds []string
	dropped := false
	for _, r := range old {
		if tok := strings.TrimPrefix(r, "#"); !held[tok] {
			cmds = append(cmds, "region remove "+tok)
			dropped = true
		}
	}
	add := fixture.RegionCommands(n)
	cmds = append(cmds, add...)
	// A run that only removed regions still has to save; RegionCommands emits
	// the save only when it added something.
	if dropped && len(add) == 0 {
		cmds = append(cmds, "region save")
	}
	return cmds
}
