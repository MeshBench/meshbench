package boardview

import (
	"testing"
)

// The wire is what shows until decoding is asked for.
//
// This window is about what the board actually did, so the bytes it actually
// sent are the default and the decode is an aid somebody turns on. A tick that
// started on would be this window quietly choosing to show something other than
// what happened.
func TestDecodingIsOffUntilItIsAskedFor(t *testing.T) {
	p := &Panel{Node: "citizen71"}
	if p.decode.Bool.Value {
		t.Error("decoding starts on, so the console shows an interpretation " +
			"before anybody asked for one")
	}
	// And the tick is only offered where there is something framed to decode.
	if p.framed {
		t.Error("the tick is offered before anything has said this node speaks " +
			"a framed protocol")
	}
}
