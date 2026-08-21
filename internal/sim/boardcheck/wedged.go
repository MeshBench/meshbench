package boardcheck

import (
	"fmt"
	"regexp"
	"strconv"
)

// A wedge is the emulated CPU stopped dead: reading one address it will never
// get an answer from, from one instruction it will never leave.
//
// It matters because the row above cannot tell the difference on its own. A
// board that stops executing puts nothing back on the air, which is exactly
// what a board that decided not to relay looks like - and the matrix would
// print "failed" against firmware that never got the chance to decide. One
// board spent five wrong explanations being called slow before anyone noticed
// it had stopped.
type wedge struct {
	Addr  string // the address being read
	PC    string // the instruction it never advances past
	Reads int    // how many times, counting Renode's collapsed repeats
}

// wedgeReads is how many reads of one address stop being a probe and start
// being a hang.
//
// Probing is normal and cheap: the Heltec_t114 reads two unmapped identifier
// registers once each and carries on, which is a driver asking whether a
// peripheral is fitted. A hang is three orders of magnitude louder - the
// RAK4631's untraced run logs 12,356 reads of a single address. Anywhere in
// between would be a new thing worth looking at rather than a verdict, so the
// threshold sits well clear of the probing and well under the hanging.
const wedgeReads = 100

// Renode says this once per access, and then collapses repeats into a
// trailing count rather than writing the line again - so the count is part of
// the measurement, not noise to be stripped.
var wedgeLine = regexp.MustCompile(
	`\[cpu: (0x[0-9A-Fa-f]+)\] Read\w+ from non existing peripheral at (0x[0-9A-Fa-f]+)\.(?: \((\d+)\))?`)

// findWedge returns the address the log spends its life reading, if there is
// one.
func findWedge(log []byte) (wedge, bool) {
	type site struct{ pc, addr string }
	reads := map[site]int{}
	for _, m := range wedgeLine.FindAllSubmatch(log, -1) {
		n := 1
		if len(m[3]) > 0 {
			if c, err := strconv.Atoi(string(m[3])); err == nil {
				n = c
			}
		}
		reads[site{pc: string(m[1]), addr: string(m[2])}] += n
	}
	var worst wedge
	for s, n := range reads {
		if n > worst.Reads {
			worst = wedge{Addr: s.addr, PC: s.pc, Reads: n}
		}
	}
	return worst, worst.Reads >= wedgeReads
}

// downgradeIfWedged rewrites failures the board was never awake to earn.
//
// Only failures, and only behaviour: a column that passed was watched
// happening, and no later hang can un-happen it. What is left is the honest
// shape of the result - the board did these things, then stopped, and the rest
// was never measured.
func (r *BoardReport) downgradeIfWedged(log []byte) {
	w, ok := findWedge(log)
	if !ok {
		return
	}
	why := fmt.Sprintf(
		"not measurable: the board stopped at %s, reading %s %s times - an address "+
			"this platform does not implement, so the answer it is waiting for "+
			"cannot arrive", w.PC, w.Addr, commas(w.Reads))
	for _, c := range Capabilities {
		if r.Results[c].State == Failed {
			r.set(c, Untested, why)
		}
	}
}

// commas groups a count so its size can be read rather than counted. These
// numbers run to nine digits, and the difference between a probe and a hang is
// the number of digits.
func commas(n int) string {
	s := strconv.Itoa(n)
	var b []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, c)
	}
	return string(b)
}
