// Varying an arm into several, and what one cell of the matrix floods.
//
// The arm type itself is session.ExpArm and stays in core, because
// provisioning and the sweep verbs describe the same configuration. What is
// here is the matrix's own business: how crossing a parameter turns one arm
// into many, and how a cell's message is built so that two cells differ in the
// thing being measured rather than in the width of somebody's label.
package experiment

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// varied returns the arm with one parameter set, and the label segment that
// records it.
func varied(base session.ExpArm, param, v string) (session.ExpArm, string, error) {
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
	for _, p := range session.VaryParams {
		have = append(have, p.Name)
	}
	return arm, "", fmt.Errorf(
		"cannot vary %q; there is: %s, or set:<any firmware setting>",
		param, strings.Join(have, ", "))
}

// cellText is what one cell floods: its own arm and seed, at a width every
// cell of the matrix shares.
//
// The width is the whole point. Airtime scales with payload and airtime is what
// collides, so a message carrying the arm's label is a message whose size is
// decided by the name somebody typed. A control arm and the arm it duplicates
// differ only in that name, and they were flooding different numbers of bytes:
// the two runs being compared differed in the one quantity the comparison is
// about, and no row of the result could show it. The seed is in the text for
// the same reason and did the same thing at ten, where the number grows a
// digit - so what separated two runs of one arm was not only the seed.
//
// The label stays, because a capture has to say which cell it came from. Only
// the size is held level.
func (e *experiment) cellText(arm session.ExpArm, seed uint64) string {
	return padTo(cellLabel(arm.Label, seed), e.messageBytes())
}

// messageBytes is the width every cell floods at: whatever the experiment asked
// for, or the widest cell text in the matrix where that is wider.
//
// A floor rather than the width, because padding to less than the text leaves
// the arms uneven again, which is the fault rather than a smaller version of it.
func (e *experiment) messageBytes() int {
	want := e.Bytes
	for _, a := range e.Arms {
		for _, seed := range e.Seeds {
			if n := len(cellLabel(a.Label, seed)); n > want {
				want = n
			}
		}
	}
	return want
}

// cellLabel names one cell, before it is padded.
func cellLabel(label string, seed uint64) string {
	return fmt.Sprintf("%s seed %d", label, seed)
}

// padTo brings a message up to a stated size, or leaves it alone when the size
// is zero or already past it.
//
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
