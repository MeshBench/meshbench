// An arm of a sweep: the settings one cell of the matrix runs under, and how
// varying a parameter turns one arm into several.
//
// Arm fields that are pointers are pointers deliberately. Mode 0 is both a
// real value and the zero value, so an arm built a field short once silently
// forced 1-byte hashes while the panel said 3.
package session

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

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

// applyOver writes only what this arm actually names, over a base.
//
// The distinction against writing every field is load-bearing: an arm varying
// one parameter must leave the other seven as the base set them, and a struct
// copy would silently reset them to their zero values - which for path hash
// mode is a real setting rather than "unset".
func (a ExpArm) applyOver(p *Provisioning) {
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

// names reports whether this arm sets anything at all. An arm that names
// nothing is a placeholder to be replaced by the first cross, not something to
// cross onto.
func (a ExpArm) names() bool {
	return a.RepeaterVersion != "" || a.CompanionVersion != "" ||
		a.PathHashMode != nil || a.RepPathHash != nil ||
		a.LoopDetect != "" || a.CAD != "" || a.SpreadMs != nil ||
		len(a.Set) > 0
}

// varied returns the arm with one parameter set, and the label segment that
// records it.
func varied(base ExpArm, param, v string) (ExpArm, string, error) {
	arm := base
	switch param {
	case "repeater_version":
		arm.RepeaterVersion = v
		return arm, bareVersion(v, "repeater-"), nil
	case "companion_version":
		arm.CompanionVersion = v
		return arm, bareVersion(v, "companion-"), nil
	case "path_hash_mode", "rep_path_hash":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 2 {
			return arm, "", fmt.Errorf("path hash mode is 0, 1 or 2; got %q", v)
		}
		if param == "path_hash_mode" {
			arm.PathHashMode = &n
			// n+1 because mode 0 is one byte per hop, and an arm labelled
			// "0-byte" reads as a path that carries nothing.
			return arm, fmt.Sprintf("%d-byte", n+1), nil
		}
		arm.RepPathHash = &n
		return arm, fmt.Sprintf("rpt %d-byte", n+1), nil
	case "loop_detect":
		switch v {
		case "off", "minimal", "moderate", "strict":
		default:
			return arm, "", fmt.Errorf("loop.detect is off, minimal, moderate or strict; got %q", v)
		}
		arm.LoopDetect = v
		return arm, "loop " + v, nil
	case "cad":
		if v != "on" && v != "off" {
			return arm, "", fmt.Errorf("cad is on or off; got %q", v)
		}
		arm.CAD = v
		return arm, "cad " + v, nil
	case "spread_ms":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return arm, "", fmt.Errorf("spread is a whole number of seconds; got %q", v)
		}
		ms := n * 1000
		arm.SpreadMs = &ms
		if n == 0 {
			return arm, "all at once", nil
		}
		return arm, fmt.Sprintf("over %ds", n), nil
	}
	// Any firmware setting, named as the CLI names it: "set:agc.reset.interval".
	//
	// The enumerated parameters above are the ones with somewhere structured to
	// live; this is everything else, and it is what makes a question like "does
	// the AGC reset interval change anything" askable without a code change.
	if name, ok := strings.CutPrefix(param, "set:"); ok && name != "" {
		if arm.Set == nil {
			arm.Set = map[string]string{}
		} else {
			// Copied, or crossing would write through into the arm this one
			// was crossed from and every sibling would end up sharing a map.
			cp := make(map[string]string, len(arm.Set))
			for k, v := range arm.Set {
				cp[k] = v
			}
			arm.Set = cp
		}
		arm.Set[name] = v
		return arm, name + " " + v, nil
	}

	var have []string
	for _, p := range VaryParams {
		have = append(have, p.Name)
	}
	return arm, "", fmt.Errorf(
		"cannot vary %q; there is: %s, or set:<any firmware setting>",
		param, strings.Join(have, ", "))
}

// padTo brings a message up to a stated size, or leaves it alone when the size
// is zero or already past it.
//
// Airtime scales with payload and airtime is what collides, so the size of the
// thing being flooded is part of the experiment rather than a detail of it.
// Padded with dots rather than spaces: a run of spaces in a console is
// indistinguishable from a message that ended.
func padTo(text string, size int) string {
	if size <= 0 || len(text) >= size {
		return text
	}
	return text + strings.Repeat(".", size-len(text))
}

// bareVersion is a build name with its role and its v stripped, because an arm
// column headed "repeater-v1.17.0 · 1-byte" spends most of its width on the two
// things every arm in the sweep has in common.
func bareVersion(v, role string) string {
	return strings.TrimPrefix(strings.TrimPrefix(v, role), "v")
}

// joinLabel builds an arm's name one crossed parameter at a time.
func joinLabel(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + " · " + b
}
