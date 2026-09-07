// An emulator that never got the machine built, and a report that said the
// board booted anyway.
//
// Separate from wedged.go because it is the opposite case and the opposite
// rule. A wedged board is awake and stuck, so what it was watched doing still
// happened and only its failures are rewritten. An emulator that aborted never
// ran the firmware at all, so nothing in the report was watched happening -
// including the rows that passed.
package boardcheck

import (
	"fmt"
	"regexp"
	"strings"
)

// abortLine matches the emulator giving up on the machine rather than the
// guest misbehaving.
//
// Narrow on purpose. An emulator log carries plenty that is not fatal - the
// SPI flash device announcing itself, unimplemented-register warnings under
// -d unimp - and treating those as an abort would mark every good probe
// untested. These three are QEMU and Renode saying they are not continuing.
var abortLine = regexp.MustCompile(
	`(?m)^.*(Unexpected error in |Attempt to set property |assertion failed|Aborted \(core dumped\)).*$`)

// findAbort is the emulator's own words about why it stopped, or "".
func findAbort(log []byte) string {
	m := abortLine.FindAll(log, -1)
	if len(m) == 0 {
		return ""
	}
	// The last one: QEMU names the assertion, the file and the line first, and
	// the property or the condition after it, and it is the second that says
	// what is wrong.
	last := strings.TrimSpace(string(m[len(m)-1]))
	const width = 200
	if len(last) > width {
		last = last[:width] + "..."
	}
	return last
}

// downgradeIfAborted rewrites every column when the emulator never ran.
//
// Every column, passed ones included, which is what separates this from
// downgradeIfWedged. A probe whose emulator aborted at hand-over reported
// "boot passed - attached and answering" while emulator.log carried
//
//	Attempt to set property 'apb_freq' on anonymous device
//	(type 'esp_soc.uart') after it was realized
//
// and the firmware had never executed an instruction. A check that disagrees
// with the thing it is checking is worse than no check: that report was read
// as evidence the board worked, on a machine where no board could start.
func (r *BoardReport) downgradeIfAborted(log []byte) {
	said := findAbort(log)
	if said == "" {
		return
	}
	why := fmt.Sprintf(
		"not measurable: the emulator stopped before the firmware ran - %s", said)
	for _, c := range Capabilities {
		r.set(c, Untested, why)
	}
}
