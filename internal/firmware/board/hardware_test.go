package board

import "testing"

// Every declared panel has to make sense on its own.
//
// There is no per-board rendering code to review - one renderer draws whatever
// a board declared - so this is the review. A panel that cannot be drawn, or
// that quietly claims a pin the radio is already using, is caught against the
// board that introduced it rather than on the day somebody opens that node.
func TestEveryPanelIsDeclaredProperly(t *testing.T) {
	declared := 0
	for _, b := range Boards() {
		if b.Hardware == nil {
			continue
		}
		declared++
		if err := b.Hardware.Validate(b.radioPins()); err != nil {
			t.Errorf("%s: %v", b.Name, err)
		}
	}
	if declared == 0 {
		t.Fatal("no board declares a panel, so this test is checking nothing")
	}
	t.Logf("%d boards declare a panel", declared)
}

// A screen has to say enough about itself to be driven.
func TestAScreenMustSayHowItIsReached(t *testing.T) {
	cases := []struct {
		what string
		p    Panel
	}{
		{"no controller", Panel{Screen: &Screen{Bus: BusI2C, Addr: 0x3C, WidthPx: 128, HeightPx: 64}}},
		{"no size", Panel{Screen: &Screen{Controller: "SSD1306", Bus: BusI2C, Addr: 0x3C}}},
		{"I2C with no address", Panel{Screen: &Screen{Controller: "SSD1306", Bus: BusI2C, WidthPx: 128, HeightPx: 64}}},
		{"SPI with no command pin", Panel{Screen: &Screen{Controller: "ST7789", Bus: BusSPI, CS: 12, WidthPx: 320, HeightPx: 240}}},
	}
	for _, c := range cases {
		if err := c.p.Validate(nil); err == nil {
			t.Errorf("a screen with %s was accepted", c.what)
		}
	}
}

// Two things on one pin is the worst kind of mistake to debug: the board still
// boots and the radio still reports a chip.
func TestAPartCannotTakeAPinTheRadioHas(t *testing.T) {
	p := Panel{Parts: []Part{{Kind: Lamp, Name: "TX", Pin: 8}}}
	if err := p.Validate(map[int]string{8: "chip select"}); err == nil {
		t.Error("a lamp on the radio's chip select was accepted")
	}
	dup := Panel{Parts: []Part{
		{Kind: Lamp, Name: "TX", Pin: 35},
		{Kind: Button, Name: "PRG", Pin: 35},
	}}
	if err := dup.Validate(nil); err == nil {
		t.Error("two parts on one pin were accepted")
	}
	// A screen's select and command lines are pins like any other. They were
	// not checked until the nRF52 boards declared panels, and a display sharing
	// a line with the radio is the fault this whole function exists to catch.
	sc := Panel{Screen: &Screen{Controller: "ST7789", Bus: BusSPI, CS: 8, DC: 12,
		WidthPx: 240, HeightPx: 135}}
	if err := sc.Validate(map[int]string{8: "chip select"}); err == nil {
		t.Error("a display sharing the radio's chip select was accepted")
	}
}

// The Renode boards name the radio's lines by port and pin, and those are the
// same pins a panel declares flat.
//
// Without this, every nRF52 board's panel was checked against an empty set: the
// collision the test above proves is caught would have gone straight through on
// the five boards that most recently declared one.
func TestARenodeBoardsRadioPinsAreClaimed(t *testing.T) {
	b := Board{Renode: &RenodeWiring{
		NssPort: "gpio1", NssPin: 10, IrqPort: "gpio0", IrqPin: 20,
	}}
	taken := b.radioPins()
	if who := taken[42]; who != "chip select" {
		t.Errorf("P1.10 is pin 42 and the chip select, and reads as %q", who)
	}
	if who := taken[20]; who != "interrupt" {
		t.Errorf("P0.20 is pin 20 and the interrupt, and reads as %q", who)
	}
	p := Panel{Parts: []Part{{Kind: Button, Name: "user", Pin: 42}}}
	if err := p.Validate(taken); err == nil {
		t.Error("a button on the radio's chip select was accepted")
	}
}

// A board that declares no button is saying something, and it must survive
// being declared: PinNone is a fact, not a gap.
func TestNoButtonIsAValidDeclaration(t *testing.T) {
	p := Panel{Parts: []Part{
		{Kind: Lamp, Name: "status", Pin: 21},
		{Kind: Button, Name: "user", Pin: PinNone},
	}}
	if err := p.Validate(nil); err != nil {
		t.Errorf("a board with no button was refused: %v", err)
	}
	if !p.HasAnything() {
		t.Error("a board with a lamp has something to draw")
	}
	if got := len(p.PartsOfKind(Button)); got != 1 {
		t.Errorf("the absent button is still declared, got %d", got)
	}
}

// A board is emulated exactly when it says how to wire it up.
//
// Two statements about one fact, in the same struct: the Emulated flag, which
// is what `meshbench boards` prints and what the workbench filters on, and
// the Renode or QEMU wiring block, which is what the runner needs to start it.
// Nothing kept them in step, so Heltec_t096 carried a complete Renode block -
// platform, SPI base, chip select, interrupt pin - beside `Emulated: false`
// and a comment saying it wanted Renode one day. It had wanted it for weeks:
// the board builds, boots, adverts, receives and is still running at 392 s,
// and was invisible to anybody choosing hardware from the list.
//
// Stated as an equivalence rather than one direction, because the other way
// round is worse: a board claiming to be emulated with no wiring is a scenario
// that passes at build time and dies at run time, which is the exact failure
// the Emulated field's own comment says it exists to prevent.
func TestABoardIsEmulatedExactlyWhenItIsWired(t *testing.T) {
	wired := 0
	for _, b := range Boards() {
		w := b.Renode != nil || b.QEMU != nil
		if w {
			wired++
		}
		switch {
		case w && !b.Emulated:
			t.Errorf("%s has wiring for an emulator but says Emulated: false,"+
				" so nothing will offer it", b.Name)
		case !w && b.Emulated:
			t.Errorf("%s says Emulated: true with no wiring, so a scenario"+
				" built around it will fail at run time", b.Name)
		}
	}
	if wired == 0 {
		t.Fatal("no board is wired for an emulator, so this test is checking nothing")
	}
	t.Logf("%d boards are wired for an emulator", wired)
}
