// Which firmware takes MeshCore's console commands (#599).
//
// Provisioning, and every setting an experiment arm pins, is a run of MeshCore
// CLI lines typed at a node's console. A firmware that is not MeshCore refuses
// each of them, and the only trace was a line in its console somebody had to
// read - "set name Deck  Error: unknown config". So a build can say what it
// speaks, a node running one that does not speak MeshCore's is not sent it,
// and an arm that would pin a setting onto one is refused before the run.
package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// dialectOf is the console vocabulary a node's build speaks, or "" for
// MeshCore's own.
func dialectOf(n scenario.Node, installed []firmware.Installed) string {
	if n.Firmware.Version == "" {
		return ""
	}
	for _, in := range installed {
		if in.Board == n.Firmware.Board && in.Version == n.Firmware.Version {
			if set := firmware.LoadBuildSettings(in.Path); !set.SpeaksMeshCore() {
				return set.Dialect
			}
			return ""
		}
	}
	return ""
}

// withDialect is a node's provisioning script, or in place of it one line
// saying why there is none.
func withDialect(n scenario.Node, lines []state.ProvisionLine) []state.ProvisionLine {
	d := dialectOf(n, firmware.ListInstalled(firmware.DefaultCacheDir()))
	if d == "" {
		return lines
	}
	return []state.ProvisionLine{{
		Command: fmt.Sprintf("# %s runs %s, which does not take MeshCore's console commands: "+
			"nothing is sent", n.Name, describeDialect(d)),
		Why:     "a build declares the console vocabulary it speaks, and this one is not MeshCore's",
		Comment: true,
	}}
}

func describeDialect(d string) string {
	if d == "none" {
		return "a firmware with no console"
	}
	return d + " firmware"
}

// PinsSettings reports whether this arm sets anything provisioning carries, as
// opposed to only choosing builds or timing.
func (a ExpArm) PinsSettings() bool {
	return a.PathHashMode != nil || a.RepPathHash != nil ||
		a.LoopDetect != "" || a.CAD != "" || len(a.Set) > 0
}

// RefuseUnprovisionable says which arms would pin settings onto nodes whose
// firmware cannot take them, or "" when none would. Each node is judged on the
// build the arm runs it on.
func (s *Sim) RefuseUnprovisionable(arms []ExpArm, nodes []scenario.Node) string {
	installed := firmware.ListInstalled(firmware.DefaultCacheDir())
	var bad []string
	for _, arm := range arms {
		if !arm.PinsSettings() {
			continue
		}
		var names []string
		for _, n := range nodes {
			n = WithFirmware(n, SweepArm{RepeaterVersion: arm.RepeaterVersion,
				CompanionVersion: arm.CompanionVersion})
			if d := dialectOf(n, installed); d != "" {
				names = append(names, fmt.Sprintf("%s (%s)", n.Name, d))
			}
		}
		if len(names) > 0 {
			sort.Strings(names)
			bad = append(bad, fmt.Sprintf("arm %q pins settings onto %s", arm.Label, strings.Join(names, ", ")))
		}
	}
	if len(bad) == 0 {
		return ""
	}
	return strings.Join(bad, "; ") + ", whose firmware does not take MeshCore's console commands, " +
		"so the settings would not apply and the cells would differ by nothing"
}
