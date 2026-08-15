package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// An arm that cannot demonstrate its own manipulation should not produce a row.
//
// The 1.17.0/1.17.1 sweep ran eight cells, failed none, and measured nothing,
// because the setting the whole study turned on was never made: MeshCore's
// rx_boosted_gain defaults to 0, the arms enabled the AGC reset that destroys
// it, and there was nothing for the reset to destroy. Every arm was genuinely
// identical and the bench reported that accurately - an hour spent to learn
// that a command had not been sent.
//
// So this runs before the measurement rather than as an explanation afterwards.
// It is wb1's `investigate` check - "read the setting back off a node" -
// promoted from a post-hoc account of a null result to a precondition of
// producing a row at all.
//
// Only settings the chip itself reports can be checked this way, which is few.
// That is the point: those are exactly the settings where firmware state and
// chip state can disagree with nothing in any log to say so.

// armDidNotReachTheChip returns one complaint per setting the arm asked for that
// the radios do not show, or nil when everything checks out.
//
// Nodes whose radio has not reported are counted and named in the complaint
// rather than treated as passing. A node that never answered is not a node that
// agreed.
func armDidNotReachTheChip(arm ExpArm, eng *engine.Engine, nodes []scenario.Node) []string {
	var got []radioSample
	var silent int
	for _, n := range nodes {
		en, ok := eng.NodeByName(n.Name)
		if !ok || en.Firmware == nil || en.Firmware.Bridge == nil {
			continue
		}
		st := en.Firmware.Bridge.Stats()
		if !st.Configured {
			silent++
			continue
		}
		got = append(got, radioSample{n.Name, st})
	}
	return armComplaints(arm, got, silent)
}

// radioSample is one node's answer about its own radio.
type radioSample struct {
	name string
	st   firmware.RadioStats
}

// armComplaints is the decision, separated from collecting it so it can be
// tested without an emulator: this is what fails a cell, and a check that
// wrongly passes is worse than no check.
func armComplaints(arm ExpArm, got []radioSample, silent int) []string {
	// An arm that pins nothing the chip reports has nothing to prove here, and
	// most arms are of that kind - a firmware version is not a register. Saying
	// anything about silence in that case would fail every cell run against a
	// build older than the extended radio payload, which is a fact about the
	// binary's age and not about the arm.
	if _, ok := arm.Set["radio.rxgain"]; !ok {
		return nil
	}

	if len(got) == 0 {
		// Nothing to check against is not the same as nothing wrong, and saying
		// "verified" here would be the exact failure this exists to prevent.
		if silent > 0 {
			return []string{fmt.Sprintf(
				"no node reported its radio state (%d attached but silent) - "+
					"this arm pins radio.rxgain, which cannot be confirmed against "+
					"firmware that predates the extended radio payload", silent)}
		}
		return nil
	}

	var out []string
	if want, ok := arm.Set["radio.rxgain"]; ok {
		boosted := strings.EqualFold(want, "on") || want == "1"
		var wrong []string
		for _, s := range got {
			if s.st.RxBoosted() != boosted {
				wrong = append(wrong, s.name)
			}
		}
		if len(wrong) > 0 {
			reg := firmware.RxGainPowerSaving
			if boosted {
				reg = firmware.RxGainBoosted
			}
			out = append(out, fmt.Sprintf(
				"radio.rxgain %s: %d of %d radios do not hold 0x%02X (%s)",
				want, len(wrong), len(got), reg, nameList(wrong)))
		}
	}
	if silent > 0 {
		out = append(out, fmt.Sprintf(
			"%d of %d nodes never reported their radio state",
			silent, silent+len(got)))
	}
	return out
}

// nameList is the first few names and a count of the rest. A cell that fails
// because a hundred nodes disagree should say so without printing a hundred
// names into a log somebody has to scroll.
func nameList(names []string) string {
	sort.Strings(names)
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:3], ", "), len(names)-3)
}
