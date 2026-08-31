package meshbench

import "testing"

// Which verb a console command goes to depends on what the node runs.
//
// A companion and a room server speak the framed companion protocol, so their
// console takes a command. Sending them keystrokes reaches the node and is
// never read, which looks exactly like a console that does not work. This
// client sent every kind keystrokes; the Python one has always split them.
func TestAConsoleCommandGoesToTheVerbTheNodeReads(t *testing.T) {
	framed := map[Kind]bool{Companion: true, RoomServer: true}

	// Every kind this build knows about, so a kind added later is not quietly
	// left on whichever branch happens to be the default.
	kinds := []Kind{
		SimpleRepeater, AdvancedRepeater, Companion,
		RoomServer, SDRObserver, Emitter,
	}
	for _, k := range kinds {
		want := "console.type"
		if framed[k] {
			want = "console.cli"
		}
		if got := consoleVerb(k); got != want {
			t.Errorf("%s: wanted %s, got %s", k, want, got)
		}
	}
}

// A kind this build has never heard of is typed at rather than refused: the
// verb on the other end says what it makes of the node, and guessing the
// framed protocol at something that does not speak it is the worse mistake.
func TestAnUnknownKindIsTypedAt(t *testing.T) {
	if got := consoleVerb(Kind("something-new")); got != "console.type" {
		t.Fatalf("wanted console.type for an unknown kind, got %s", got)
	}
}
