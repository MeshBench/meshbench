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

	"github.com/A13xB0/meshcoresim/internal/fixture"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// ProvisioningFor is the script for one node, with each line's reason.
//
// The reason travels with the line because the failure this exists to prevent
// is silent: a region that is defined but not allowed to flood relays nothing
// and reports no error, which looks like broken RF rather than a missing line.
func ProvisioningFor(n scenario.Node) []state.ProvisionLine {
	var out []state.ProvisionLine

	out = append(out, state.ProvisionLine{
		Command: fmt.Sprintf("# %s: %s, %s", n.Name, n.Kind, n.Firmware.Version),
		Why:     "which build runs, chosen when the node launches rather than after",
		Comment: true,
	})

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
			return ProvisioningFor(n), nil
		}
	}
	return nil, fmt.Errorf("no node named %q", name)
}
