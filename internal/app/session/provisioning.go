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
	arm.applyOver(&prov)
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
	for _, c := range prov.commandsFor(n) {
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
			why = "the wildcard: relays a flood whatever its scope, and the one " +
				"line that makes a node forward something it was never told about"
		case len(c) > 13 && c[:13] == "region allowf":
			why = "permits flooding for that region - a region defined but not " +
				"allowed relays nothing and reports no error"
		}
		out = append(out, state.ProvisionLine{Command: c, Why: why})
	}

	if len(out) == 1 {
		out = append(out, state.ProvisionLine{
			Command: "# nothing to send",
			Why: "this node carries no regions, so it forwards only what its " +
				"defaults allow - which on a fresh import is nothing",
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
	return nil, fmt.Errorf("no node named %q", name)
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
