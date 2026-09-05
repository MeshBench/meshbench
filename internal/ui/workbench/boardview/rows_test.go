// What the two tables conclude, and the one thing they must never do: report a
// fault the state does not support, or stay quiet about one it does.
package boardview

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
)

func board(t *testing.T, name string) hw.Board {
	t.Helper()
	b, err := hw.BoardByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A radio that has not reported is not a radio with a fault. Until the chip has
// said anything the table has nothing to compare, and inventing rows from the
// board's own claims alone would report every stopped node as healthy.
func TestASilentRadioProducesNoRowsRatherThanHealthyOnes(t *testing.T) {
	b := board(t, "LilyGo_TDeck")
	if rows := radioRows(b, nil); len(rows) != 0 {
		t.Errorf("a node with no state produced %d radio rows", len(rows))
	}
	st := &state.NodeStat{Name: "Deck", Board: "LilyGo_TDeck"}
	if rows := radioRows(b, st); len(rows) != 0 {
		t.Errorf("a radio that has not reported produced %d rows", len(rows))
	}
}

// The faults the window exists to find, each one raised from state that really
// carries it.
func TestTheRadioTableNamesWhatTheChipWasLeftAt(t *testing.T) {
	b := board(t, "LilyGo_TDeck")
	st := &state.NodeStat{
		Name: "Deck", Board: "LilyGo_TDeck", Running: true, Spurious: 3,
		Radio: state.RadioState{Reported: true, Boosted: false, GainReg: 0x94,
			TxPowerDBm: 22, SF: 10, CR: 5, FreqHz: 869618000, BandwidthHz: 250000,
			IRQMask: 0x0002},
	}
	got := map[string]Row{}
	for _, r := range radioRows(b, st) {
		got[r.Name] = r
	}

	// A receiver left in power saving is a link budget nobody changed on
	// purpose, and it is not a hard fault - so it is a caution, not a failure.
	if v := got["receive gain"].Verdict; v != Silent {
		t.Errorf("a radio in power saving read %v, want %v", v, Silent)
	}
	// Interrupts enabled but never read is the shape of the receive path that
	// is not being woken.
	if v := got["interrupts"].Verdict; v != Silent {
		t.Errorf("a mask that was never read read %v, want %v", v, Silent)
	}
	// Interrupts the firmware never asked for mean the two disagree about what
	// the chip was told, which is a fault on sight.
	if v := got["spurious interrupts"].Verdict; v != Diverged {
		t.Errorf("three spurious interrupts read %v, want %v", v, Diverged)
	}
	// And the healthy ones are not dressed up as faults.
	if v := got["transmit power"].Verdict; v != Agrees {
		t.Errorf("22 dBm on a board rated 22 read %v, want %v", v, Agrees)
	}

	// A boosted receiver that has read its interrupts raises neither.
	st.Radio.Boosted, st.Radio.GainReg, st.IRQReads, st.Spurious = true, 0x96, 41, 0
	for _, r := range radioRows(b, st) {
		if r.Verdict != Agrees {
			t.Errorf("a healthy radio's %q read %v, want %v", r.Name, r.Verdict, Agrees)
		}
	}
}

// Every part the board declares reaches the wiring table, and the ones nothing
// models say so rather than reporting a zero.
func TestTheWiringTableSaysWhatIsNotModelled(t *testing.T) {
	b := board(t, "LilyGo_TDeck")
	rows := wiringRows(b, nil)
	if len(rows) == 0 {
		t.Fatal("the T-Deck declares parts and the table has none")
	}
	var meter *Row
	for i := range rows {
		if strings.Contains(rows[i].Name, "battery") {
			meter = &rows[i]
		}
	}
	if meter == nil {
		t.Fatal("the battery meter is declared and did not reach the table")
	}
	if meter.Verdict != NotModelled {
		t.Errorf("the battery meter read %v, want %v - an unmodelled input is "+
			"not a zero, it is a firmware waiting for ever", meter.Verdict, NotModelled)
	}

	// The rows arrive grouped, or the index heads the same group twice.
	seen := map[string]bool{}
	last := ""
	for _, r := range rows {
		if r.Group == last {
			continue
		}
		if seen[r.Group] {
			t.Errorf("group %q appears in two runs, so the index heads it twice", r.Group)
		}
		seen[r.Group] = true
		last = r.Group
	}
}

// A row we cannot answer for concludes nothing, and says the fact once.
func TestAnUninstrumentedRowDoesNotStateItselfTwice(t *testing.T) {
	if s := Undeclared.String(); s != "" {
		t.Errorf("an uninstrumented row's verdict is %q; the Observed column "+
			"already carries that sentence and two columns of it is noise", s)
	}
	for _, v := range []Verdict{Agrees, Diverged, Silent, NotModelled} {
		if v.String() == "" {
			t.Errorf("verdict %d says nothing, so its row reports no conclusion", v)
		}
	}
}
