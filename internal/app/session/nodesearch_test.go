package session

import "testing"

// The names are the shape ScotMesh actually uses: emoji either side, a Gaelic
// accent, and one name that is a prefix of another.
const (
	westLomond    = "🏔️ West Lomond 📡"
	westLomondTwo = "🏔️ West Lomond Relay Two 📡"
	beinnArd      = "Beinn Àrd ⛰"
	dunfermline   = "📻 Dunfermline Repeater"
)

func TestSearchKeyDropsEmojiAndAccents(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{westLomond, "west lomond"},
		{beinnArd, "beinn ard"},
		{"West  Lomond", "west lomond"},
		{"west-lomond", "west lomond"},
		{"🏔️📡", ""},
	} {
		if got := searchKey(c.in); got != c.want {
			t.Errorf("searchKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The whole point of the verb: the exact name wins over the one that merely
// starts the same way. A caller taking the top result is taking this.
func TestSearchPrefersTheTighterName(t *testing.T) {
	q := searchKey("west lomond")
	tight := searchScore(q, searchKey(westLomond))
	loose := searchScore(q, searchKey(westLomondTwo))
	if tight <= loose {
		t.Fatalf("%q scored %v, %q scored %v: the exact name must win",
			westLomond, tight, westLomondTwo, loose)
	}
	if tight < searchFloor || loose < searchFloor {
		t.Fatalf("both should be offered: %v and %v, floor %v", tight, loose, searchFloor)
	}
}

func TestSearchIgnoresAccentsAndUnrelatedNames(t *testing.T) {
	if got := searchScore(searchKey("beinn ard"), searchKey(beinnArd)); got < 0.9 {
		t.Errorf("accent-insensitive match scored %v, want a prefix-band score", got)
	}
	if got := searchScore(searchKey("west lomond"), searchKey(dunfermline)); got >= searchFloor {
		t.Errorf("%q scored %v against \"west lomond\": should be under the floor",
			dunfermline, got)
	}
}

// Out of order and partly typed still finds it - which is the case that sends
// somebody back to scrolling the list if it does not work.
func TestSearchFindsWordsOutOfOrder(t *testing.T) {
	if got := searchScore(searchKey("lomond west"), searchKey(westLomond)); got < searchFloor {
		t.Errorf("out-of-order query scored %v, want at least the floor", got)
	}
}
