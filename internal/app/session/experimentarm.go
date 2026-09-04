// An arm of a sweep: the settings one cell of the matrix runs under.
//
// It stays in core, beside SweepArm and for the same reason: the sweep verbs,
// the experiment matrix and provisioning all describe a configuration under
// test, and a type two of them import cannot live inside either. How an arm is
// varied into several belongs to the matrix and lives in the experiment
// package; what an arm *is* belongs here.
//
// Arm fields that are pointers are pointers deliberately. Mode 0 is both a
// real value and the zero value, so an arm built a field short once silently
// forced 1-byte hashes while the panel said 3.
package session

import (
	"sort"
	"strings"
)

// VaryParams is every parameter an arm can be crossed on, in the order the
// Bench offers them, with the values it offers by default.
//
// One table so the panel, the verb and anything scripting this cannot drift:
// a dropdown listing a parameter the verb rejects is worse than not offering
// it, because the failure arrives after the arms have been built. That is also
// why it is here rather than in the experiment package that crosses on it: the
// sweep panel reads this table, and the drift it prevents is exactly the drift
// a copy on either side would reintroduce.
var VaryParams = []struct {
	Name, Label, Defaults string
}{
	{"path_hash_mode", "companion path hash", "0, 1, 2"},
	{"rep_path_hash", "repeater path hash", "0, 1, 2"},
	{"loop_detect", "loop.detect", "off, minimal, moderate, strict"},
	{"cad", "cad", "off, on"},
	{"repeater_version", "repeater firmware", ""},
	{"companion_version", "companion firmware", ""},
	{"spread_ms", "spread", "0, 5, 20"},
}

// ExpArm is one configuration under test.
//
// Everything past the two firmware fields is a provisioning setting, because
// the questions worth asking of a mesh are mostly not "which build" - they are
// how much of a path a message carries, whether repeaters detect loops, and
// whether anybody listens before talking.
type ExpArm struct {
	Label            string `json:"label"`
	RepeaterVersion  string `json:"repeater_version"`
	CompanionVersion string `json:"companion_version"`

	// PathHashMode is the *companion's* setting, and it is the one that
	// decides what a message carries: the originator stamps it and every hop
	// honours it. RepPathHash is the repeaters' own, which only affects the
	// adverts they originate and is normally held constant.
	//
	// Pointers rather than a -1 sentinel. Mode 0 is a real value - one byte per
	// hop - and it is also the zero value, so "unset" and "one byte" were the
	// same thing to every path that built an arm without naming the field.
	PathHashMode *int `json:"path_hash_mode,omitempty"`
	RepPathHash  *int `json:"rep_path_hash,omitempty"`

	// LoopDetect is off, minimal, moderate or strict; CAD is on or off. Empty
	// leaves whatever the scenario has.
	LoopDetect string `json:"loop_detect,omitempty"`
	CAD        string `json:"cad,omitempty"`

	// SpreadMs staggers this arm's senders across the burst, overriding the
	// experiment's own.
	SpreadMs *int `json:"spread_ms,omitempty"`

	// Set is any other firmware setting this arm pins, as the CLI spells it:
	// "agc.reset.interval" to "4", "radio.rxgain" to "off".
	//
	// Open rather than a fixed list, because the interesting question is
	// usually about a setting nobody thought to enumerate - the AGC reset
	// interval is the whole reason the 1.17.1 gain fault is reachable at all,
	// and no version of this had a field for it.
	Set map[string]string `json:"set,omitempty"`
}

// ApplyOver writes only what this arm actually names, over a base.
//
// The distinction against writing every field is load-bearing: an arm varying
// one parameter must leave the other seven as the base set them, and a struct
// copy would silently reset them to their zero values - which for path hash
// mode is a real setting rather than "unset".
func (a ExpArm) ApplyOver(p *Provisioning) {
	if a.PathHashMode != nil {
		p.CompPathHashMode = *a.PathHashMode
	}
	if a.RepPathHash != nil {
		p.PathHashMode = *a.RepPathHash
	}
	if a.LoopDetect != "" {
		p.LoopDetect = a.LoopDetect
	}
	if a.CAD != "" {
		p.CadMode = a.CAD
	}
	// Anything else goes on the end of the study's own extra lines, which is
	// where a setting with no field of its own belongs: provisioning sends
	// them verbatim, so an arm can pin a switch this code has never heard of.
	if len(a.Set) > 0 {
		keys := make([]string, 0, len(a.Set))
		for k := range a.Set {
			keys = append(keys, k)
		}
		sort.Strings(keys) // one arm, one order, so a cell is reproducible
		var lines []string
		if p.Extra != "" {
			lines = append(lines, p.Extra)
		}
		for _, k := range keys {
			lines = append(lines, "set "+k+" "+a.Set[k])
		}
		p.Extra = strings.Join(lines, "\n")
	}
}

// Names reports whether this arm sets anything at all. An arm that names
// nothing is a placeholder to be replaced by the first cross, not something to
// cross onto.
func (a ExpArm) Names() bool {
	return a.RepeaterVersion != "" || a.CompanionVersion != "" ||
		a.PathHashMode != nil || a.RepPathHash != nil ||
		a.LoopDetect != "" || a.CAD != "" || a.SpreadMs != nil ||
		len(a.Set) > 0
}
