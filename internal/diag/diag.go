// Package diag is opt-in diagnostic logging, chosen by domain.
//
// The program has diagnostic output for many separate areas - the radio, the
// emulator, the channel, the layer chrome - and until now each was its own
// environment variable with its own name and its own on/off, so "show me the
// radio and the emulator and nothing else" was not a thing anyone could say.
// This is the one place to say it.
//
//	MESHBENCH_LOG=radio,emulator     # just those two
//	MESHBENCH_LOG=all                # every domain
//	MESHBENCH_LOG=                   # none - the default
//
// A domain is a coarse area of the program, named in one word. A line prints to
// stderr, prefixed with its domain, only when that domain is selected; a
// suppressed line costs a map lookup and nothing else, so a caller may log
// freely behind On without guarding the work when the work is cheap.
//
// Deliberately not a level system and not structured logging: the question the
// operator has is "which part", not "how severe", and the answer wanted is
// lines a person reads, not JSON a collector ingests. The scattered
// MESHBENCH_*_DEBUG variables can move behind this one at each site, keeping
// their old spelling working, so nothing that already relies on one breaks.
package diag

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Env is the variable that chooses the domains.
const Env = "MESHBENCH_LOG"

var (
	mu      sync.Mutex
	parsed  bool
	domains map[string]bool
	allOn   bool
)

// parse reads a comma-separated domain list. Whitespace around a name is
// ignored so MESHBENCH_LOG="radio, emulator" means what it looks like.
func parse(v string) {
	domains = map[string]bool{}
	allOn = false
	for _, d := range strings.Split(v, ",") {
		d = strings.TrimSpace(d)
		switch d {
		case "":
			continue
		case "all":
			allOn = true
		default:
			domains[d] = true
		}
	}
}

func ensure() {
	mu.Lock()
	defer mu.Unlock()
	if !parsed {
		parse(os.Getenv(Env))
		parsed = true
	}
}

// On reports whether a domain's diagnostics are switched on.
//
// Use it to guard work that is only worth doing to produce a diagnostic - a
// hex dump, a sum over nodes - so the cost is paid only when someone is
// watching:
//
//	if diag.On("radio") { diag.Printf("radio", "%s", expensiveDump()) }
func On(domain string) bool {
	ensure()
	mu.Lock()
	defer mu.Unlock()
	return allOn || domains[domain]
}

// Printf writes one diagnostic line for a domain, if that domain is on. The
// line is prefixed with the domain in brackets and a newline is added.
func Printf(domain, format string, args ...any) {
	if !On(domain) {
		return
	}
	fmt.Fprintf(os.Stderr, "["+domain+"] "+format+"\n", args...)
}

// Println writes its arguments for a domain, if that domain is on, space-joined
// with a newline, after the domain prefix.
func Println(domain string, args ...any) {
	if !On(domain) {
		return
	}
	fmt.Fprint(os.Stderr, "["+domain+"] "+fmt.Sprintln(args...))
}
