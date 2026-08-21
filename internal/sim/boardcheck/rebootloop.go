// A board that keeps starting over never started.
package boardcheck

import (
	"fmt"
	"regexp"
	"strings"
)

// rebootLoopMin is how many restarts stop being a reset and start being a
// loop.
//
// One is ordinary - a board may reset itself once on the way up, and some
// bootloaders do it deliberately. Three in a run that should contain exactly
// one is not something a healthy board does.
const rebootLoopMin = 3

// bootBanner is the first line an Espressif part prints out of its own ROM,
// once per start. Matching the ROM rather than the application is what makes
// it a count of starts: an application that never runs prints nothing.
var bootBanner = regexp.MustCompile(`(?m)^(?:ESP-ROM:|ets [A-Z][a-z]{2} )`)

// assertLine is what ESP-IDF prints when it gives up. The file and expression
// are usually unreadable - the panic handler turns the flash cache off before
// printing, so anything living in flash comes out as a placeholder - which is
// why this keeps the whole line rather than trying to parse it.
var assertLine = regexp.MustCompile(`(?m)^(assert failed:.*|Guru Meditation Error.*)$`)

// looping reports how many times the board started, and what it said before
// giving up, if it said anything.
func looping(log []byte) (starts int, why string) {
	starts = len(bootBanner.FindAll(log, -1))
	if m := assertLine.Find(log); m != nil {
		why = strings.TrimSpace(string(m))
	}
	return starts, why
}

// downgradeIfRebooting turns every row into the truth about a board that
// never finished starting.
//
// Boot is the row that has to change, and it is the one that reads as passing
// today: the row asks whether the emulator attached and the node kept its
// clock, and an emulated part advances its clock whether or not it is getting
// anywhere. A board restarting for ever satisfies both. Everything measured
// after it is measured against a board that is not running, so those become
// untested rather than failed - the firmware never got the chance to decline.
func (r *BoardReport) downgradeIfRebooting(log []byte) {
	starts, why := looping(log)
	if starts < rebootLoopMin {
		return
	}
	said := ""
	if why != "" {
		said = " - " + why
	}
	r.set(Boot, Failed, fmt.Sprintf(
		"started %d times and never finished starting%s", starts, said))
	for _, c := range Capabilities {
		if c == Boot || c == Build {
			continue
		}
		if r.Results[c].State == Failed {
			r.set(c, Untested, "the board never finished starting")
		}
	}
}
