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
	// This board's meter is on a converter the emulator models, so the row
	// says so. It read "not modelled" for a day, which is a false claim in the
	// one column that exists to be trusted - the interface had its own copy of
	// which pins are converter inputs, and the copy was wrong.
	if meter.Verdict != Agrees {
		t.Errorf("the T-Deck's battery meter read %v, want %v: its pin is a "+
			"converter input and the converter is modelled", meter.Verdict, Agrees)
	}
	if !strings.Contains(meter.Observed, "channel") {
		t.Errorf("the meter says %q, which does not say which converter "+
			"channel it landed on", meter.Observed)
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

// A meter on a pin no converter we model can read says so, and says it is
// about us rather than about the board.
//
// No board in the catalogue is like this today - every declared meter is on the
// first converter of an ESP32-S3 - so the case is built here rather than left
// untested until somebody adds the first one.
func TestAMeterOnNoModelledConverterSaysSo(t *testing.T) {
	b := hw.Board{
		Name: "Invented", MCU: "nRF52840",
		Hardware: &hw.Panel{Parts: []hw.Part{
			{Kind: hw.Meter, Name: "battery", Pin: 4, FullScaleMV: 4200},
		}},
	}
	rows := wiringRows(b, nil)
	if len(rows) != 1 {
		t.Fatalf("one part declared, %d rows", len(rows))
	}
	if rows[0].Verdict != NotModelled {
		t.Errorf("a meter on a part with no modelled converter read %v, want %v",
			rows[0].Verdict, NotModelled)
	}
	if !strings.Contains(rows[0].Why, "waits for ever") {
		t.Errorf("the reason does not say what an unmodelled input costs: %q",
			rows[0].Why)
	}
}

// A board whose parts nobody has recorded is not a node without a board.
//
// Every nRF52 profile is one today. Refusing them cost the radio table as well,
// which needs no panel at all - the chip's own registers reach the interface
// whichever emulator is driving it.
func TestABoardWithNoRecordedPanelStillHasARadio(t *testing.T) {
	b := board(t, "Heltec_t114")
	if b.Hardware != nil {
		t.Skip("this board has grown a panel; pick another for this case")
	}
	st := &state.NodeStat{Name: "n", Board: b.Name, Running: true, IRQReads: 9,
		Radio: state.RadioState{Reported: true, Boosted: true, GainReg: 0x96,
			TxPowerDBm: 22, SF: 10, CR: 5, FreqHz: 869618000, BandwidthHz: 250000,
			IRQMask: 2}}
	if got := radioRows(b, st); len(got) == 0 {
		t.Error("a board with no recorded panel produced no radio rows, so the " +
			"window has nothing to say about a chip it can see perfectly well")
	}
	// And the wiring side is honest rather than empty: the radio's own lines
	// come from the emulator's wiring, not from a panel.
	for _, r := range wiringRows(b, st) {
		if r.Group != "Radio" {
			t.Errorf("a board with no panel produced a %q row from nowhere", r.Group)
		}
	}
}
