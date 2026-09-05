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
// No board in the catalogue is like this today - every declared meter is on a
// converter one of the two emulators models - so the cases are built here
// rather than left untested until somebody adds the first one. Both shapes it
// can take: a part whose converter nothing models, and a part whose converter
// is modelled with the meter declared on a pin that is not one of its inputs.
func TestAMeterOnNoModelledConverterSaysSo(t *testing.T) {
	for _, c := range []struct {
		what  string
		board hw.Board
	}{
		{"a part nothing models", hw.Board{Name: "Invented", MCU: "RP2350",
			Hardware: &hw.Panel{Parts: []hw.Part{
				{Kind: hw.Meter, Name: "battery", Pin: 4, FullScaleMV: 4200},
			}}}},
		{"an nRF52 pin that is not an analogue input", hw.Board{
			Name: "Invented", MCU: "nRF52840",
			Hardware: &hw.Panel{Parts: []hw.Part{
				{Kind: hw.Meter, Name: "battery", Pin: 12, FullScaleMV: 4200},
			}}}},
	} {
		rows := wiringRows(c.board, nil)
		if len(rows) != 1 {
			t.Fatalf("%s: one part declared, %d rows", c.what, len(rows))
		}
		if rows[0].Verdict != NotModelled {
			t.Errorf("%s: read %v, want %v", c.what, rows[0].Verdict, NotModelled)
		}
		if !strings.Contains(rows[0].Why, "waits for ever") {
			t.Errorf("%s: the reason does not say what an unmodelled input "+
				"costs: %q", c.what, rows[0].Why)
		}
	}
}

// And a board whose meter is modelled says which input it lands on.
//
// The counterpart to the test above, and the reason it matters: this column is
// the one a person trusts, and "not modelled" about a converter that is
// modelled is the worst thing it can say.
func TestAModelledMeterNamesItsInput(t *testing.T) {
	b, err := hw.BoardByName("Heltec_t114")
	if err != nil {
		t.Fatal(err)
	}
	var meter *Row
	for i, r := range wiringRows(b, nil) {
		if r.Declared == hw.Meter.String() {
			meter = &wiringRows(b, nil)[i]
			break
		}
	}
	if meter == nil {
		t.Fatal("the T114 declares a battery meter and no row reports one")
	}
	if meter.Verdict != Agrees {
		t.Errorf("its meter read %v (%q), want %v - this board's cell is on "+
			"AIN2 and the converter under it is modelled",
			meter.Verdict, meter.Observed, Agrees)
	}
}

// A board whose parts nobody has recorded is not a node without a board.
//
// Every nRF52 profile was one until their variants were transcribed, and
// refusing them cost the radio table as well, cost the radio table as well,
// which needs no panel at all - the chip's own registers reach the interface
// whichever emulator is driving it.
func TestABoardWithNoRecordedPanelStillHasARadio(t *testing.T) {
	// Built rather than named: a board with no panel today may have one
	// tomorrow, and this is about the code's behaviour rather than the
	// catalogue's contents.
	b := hw.Board{Name: "Unrecorded", MCU: "nRF52840", Radio: "SX1262",
		MaxTxDBm: 22}
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

// A panel no emulator can draw on says so, and does not blame the firmware.
//
// "Silent" is a verdict about the board: it drew nothing and could have. A
// board whose declared panel has no model behind it will never draw at all, and
// reporting that as silence sends somebody looking for a fault in a firmware
// that is behaving perfectly.
//
// The RAK4631 is the one left. Its display is on I2C, and Renode's TWIM model
// answers an address with a NACK, so there is nowhere to put an SSD1306 yet -
// where the two SPI panels beside it are drawn.
func TestAnUndrawablePanelBlamesUsRatherThanTheFirmware(t *testing.T) {
	b, err := hw.BoardByName("RAK_4631")
	if err != nil {
		t.Fatal(err)
	}
	if b.Renode == nil || b.Hardware == nil || b.Hardware.Screen == nil {
		t.Fatal("this test needs a Renode board that declares a screen")
	}
	running := &state.NodeStat{Name: "n", Board: b.Name, Running: true}
	var screen *Row
	rows := wiringRows(b, running)
	for i := range rows {
		if rows[i].Group == "Display" {
			screen = &rows[i]
			break
		}
	}
	if screen == nil {
		t.Fatal("a board that declares a screen produced no display row")
	}
	if screen.Verdict == Silent {
		t.Errorf("a panel with no model behind it read as %v, which says the "+
			"firmware chose not to draw", Silent)
	}
	if screen.Verdict != NotModelled {
		t.Errorf("read %v (%q), want %v", screen.Verdict, screen.Observed, NotModelled)
	}
}

// A button a person can press does not read as one nothing is wired to.
//
// Both emulators drive a declared button now, so "not instrumented" - which is
// what this said while only one of them did - is the same false claim the meter
// used to make. It is still not an observation: nothing reads the line back,
// and the column says what this end does rather than what the pin is at.
func TestADrivenPartSaysItIsDriven(t *testing.T) {
	b := hw.Board{Name: "Invented", MCU: "nRF52840",
		Hardware: &hw.Panel{Parts: []hw.Part{
			{Kind: hw.Button, Name: "user", Pin: 42, ActiveLow: true},
		}}}
	rows := wiringRows(b, &state.NodeStat{Name: "n", Board: b.Name, Running: true})
	if len(rows) != 1 {
		t.Fatalf("one part declared, %d rows", len(rows))
	}
	if strings.Contains(rows[0].Observed, "not instrumented") {
		t.Errorf("a button that moves a real pin reads as %q", rows[0].Observed)
	}
	// And a board that is off says that instead, rather than promising a press
	// would go somewhere.
	off := wiringRows(b, &state.NodeStat{Name: "n", Board: b.Name})
	if off[0].Observed != "not powered" {
		t.Errorf("a stopped board's button reads %q, want \"not powered\"",
			off[0].Observed)
	}
}

// And the two boards whose panels are drawn say so.
//
// The counterpart, and the reason the test above is worth having: "no display
// modelled" about a display that is modelled is the same false claim in the
// other direction, and this window's whole value is that a row can be trusted.
func TestADrawablePanelIsNotCalledUnmodelled(t *testing.T) {
	for _, name := range []string{"Heltec_t114", "Heltec_t096"} {
		b, err := hw.BoardByName(name)
		if err != nil {
			t.Fatal(err)
		}
		var screen *Row
		rows := wiringRows(b, &state.NodeStat{Name: "n", Board: b.Name, Running: true})
		for i := range rows {
			if rows[i].Group == "Display" {
				screen = &rows[i]
				break
			}
		}
		if screen == nil {
			t.Fatalf("%s declares a screen and no display row came back", name)
		}
		if screen.Verdict == NotModelled {
			t.Errorf("%s: its panel is drawn under Renode and the row says %q",
				name, screen.Observed)
		}
	}
}
