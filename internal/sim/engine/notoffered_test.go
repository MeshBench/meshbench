package engine

import (
	"strings"
	"testing"
)

// The three silent paths through deliver each say why, and the sentence
// carries enough to act on.
//
// The ledger stays quiet for these deliberately - on a national network they
// were most of it, and "physics still applies" is not news - but that silence
// is indistinguishable from a packet that arrived and was ignored. A board
// probe spent a long time reading one as the other, so the reason has to exist
// somewhere even though it must not exist in the ledger.
//
// The sentence is tested rather than the writing of it: diag parses
// MESHBENCH_LOG once for the life of the process, so a test cannot switch a
// domain on afterwards - and whether a suppressed domain stays quiet is diag's
// own business, which diag_test.go already holds it to.
func TestNotOfferedNamesTheReason(t *testing.T) {
	for _, tc := range []struct {
		name, why string
		want      []string
	}{
		{
			"another preset",
			"tuned elsewhere: it is on 915.000 MHz SF7 BW250000 CR4/5 and this is " +
				"869.618 MHz SF8 BW62500 CR4/8",
			// Both sides, because "tuned elsewhere" without them does not say
			// which end is wrong.
			[]string{"tuned elsewhere", "915.000", "869.618"},
		},
		{
			"below the floor",
			"below the floor: -142.0 dBm against a noise floor of -125.0 dBm",
			// The two numbers, so a reader can see how far short it fell
			// rather than being told it fell short.
			[]string{"below the floor", "-142.0", "-125.0"},
		},
		{
			"not a listener",
			"it does not listen: emitter",
			[]string{"does not listen", "emitter"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := notOfferedLine("bc-sender", "bc-under-test", tc.why, 7, 1234)
			for _, want := range append(tc.want, "bc-sender", "bc-under-test", "not offered", "1234") {
				if !strings.Contains(got, want) {
					t.Errorf("the line does not carry %q:\n  %s", want, got)
				}
			}
		})
	}
}
