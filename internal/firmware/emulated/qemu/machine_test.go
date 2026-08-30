package qemu_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware/emulated/qemu"
)

// base is a board with nothing on it but a radio, which is the smallest
// machine argument that can boot.
func base() qemu.Config {
	return qemu.Config{Machine: "esp32s3", SPI: 3, NSS: 9, Busy: 13}
}

// options splits a machine argument into its comma separated parts, undoing the
// doubling that escapes a comma inside one value.
func options(t *testing.T, arg string) []string {
	t.Helper()
	// A doubled comma is a literal one, so it is protected before the split and
	// put back afterwards.
	const guard = "\x00"
	parts := strings.Split(strings.ReplaceAll(arg, ",,", guard), ",")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(p, guard, ",")
	}
	return parts
}

func has(t *testing.T, arg, want string) bool {
	t.Helper()
	for _, o := range options(t, arg) {
		if o == want {
			return true
		}
	}
	return false
}

func hasKey(t *testing.T, arg, key string) bool {
	t.Helper()
	for _, o := range options(t, arg) {
		if strings.HasPrefix(o, key+"=") {
			return true
		}
	}
	return false
}

// The radio is the one thing every board has, and its three pins are what the
// firmware talks to it through.
func TestTheRadioIsAlwaysWired(t *testing.T) {
	arg := base().Arg("/tmp/radio.sock")
	if got := options(t, arg)[0]; got != "esp32s3" {
		t.Fatalf("the machine name must come first, got %q", got)
	}
	for _, want := range []string{
		"radio-path=/tmp/radio.sock", "radio-spi=3", "radio-nss=9", "radio-busy=13",
	} {
		if !has(t, arg, want) {
			t.Errorf("missing %q in %q", want, arg)
		}
	}
}

// DIO1 is the line a received packet arrives on. A board that does not declare
// it must leave it unwired rather than wiring it to pin zero, and a board that
// does declare it must have it: an emulated board with no DIO1 hears nothing
// and forwards nothing, which reads as a mesh that simply does not relay.
func TestDIO1IsWiredOnlyWhenTheBoardDeclaresIt(t *testing.T) {
	if hasKey(t, base().Arg("r"), "radio-dio1") {
		t.Error("a board that declares no DIO1 must not have the line wired")
	}
	c := base()
	c.DIO1 = 33
	if !has(t, c.Arg("r"), "radio-dio1=33") {
		t.Error("a board that declares DIO1 must have it wired")
	}
}

// A board with no front end module is not the same as one whose module is
// switched off, so the option is absent rather than zero.
func TestTheFrontEndIsWiredOnlyWhenPresent(t *testing.T) {
	if hasKey(t, base().Arg("r"), "radio-fem") {
		t.Error("a board with no FEM must not have the line wired")
	}
	c := base()
	c.FEM = 21
	if !has(t, c.Arg("r"), "radio-fem=21") {
		t.Error("a board with a FEM must have it wired")
	}
}

// Button pins are several values inside one comma separated value, so their
// separator is doubled. A bare comma ends the option instead, and the board
// refuses to start.
func TestButtonPinsAreEscapedFromTheOptionList(t *testing.T) {
	c := base()
	c.ButtonPath = "/tmp/in.sock"
	c.ButtonPins = []int{0, 14}
	arg := c.Arg("r")
	if !has(t, arg, "input-pins=0,14") {
		t.Fatalf("the pin list did not survive the option list: %q", arg)
	}
	if strings.Contains(arg, "input-pins=0,14,") && !strings.Contains(arg, ",,") {
		t.Error("the separator inside the pin list must be doubled")
	}
	// One button needs no escaping and must not gain any.
	c.ButtonPins = []int{7}
	if !has(t, c.Arg("r"), "input-pins=7") {
		t.Error("a single pin should be passed plainly")
	}
}

// The keyboard and touch addresses ride on the same socket as the buttons, so
// they appear only when something is listening on it.
func TestInputAddressesNeedAnInputPath(t *testing.T) {
	c := base()
	c.KbdAddr, c.TouchAddr = 0x55, 0x5D
	if hasKey(t, c.Arg("r"), "kbd-addr") {
		t.Error("addresses without an input path have nothing to arrive on")
	}
	c.ButtonPath = "/tmp/in.sock"
	arg := c.Arg("r")
	if !has(t, arg, "kbd-addr=85") || !has(t, arg, "touch-addr=93") {
		t.Errorf("the input addresses did not reach the machine: %q", arg)
	}
}

// A card's chip select is only meaningful when there is a card behind it.
func TestTheCardSelectNeedsACard(t *testing.T) {
	c := base()
	c.CardCS = 13
	if hasKey(t, c.Arg("r"), "card-cs") {
		t.Error("a chip select with no card is a select for nothing")
	}
	c.CardPath = "/tmp/card.img"
	if !has(t, c.Arg("r"), "card-cs=13") {
		t.Error("a board with a card must declare its chip select")
	}
}

// The battery reading is declared by its raw value, so a board that reports no
// battery says nothing rather than reporting zero volts.
func TestTheBatteryIsDeclaredOnlyWhenItReads(t *testing.T) {
	c := base()
	c.BatChannel = 4
	if hasKey(t, c.Arg("r"), "bat-adc-raw") {
		t.Error("a channel with no reading must not be declared")
	}
	c.BatRaw = 2048
	arg := c.Arg("r")
	if !has(t, arg, "bat-adc-channel=4") || !has(t, arg, "bat-adc-raw=2048") {
		t.Errorf("the battery reading did not reach the machine: %q", arg)
	}
}

// A panel on I2C has a default address; one on SPI carries its own geometry.
func TestThePanelCarriesItsAddressAndGeometry(t *testing.T) {
	c := base()
	c.PanelPath = "/tmp/panel.sock"
	arg := c.Arg("r")
	if !has(t, arg, "panel-addr=60") { // 0x3C, the usual OLED address
		t.Errorf("a panel with no address should default to 0x3C: %q", arg)
	}
	if hasKey(t, arg, "panel-cs") {
		t.Error("a panel with no chip select is not on SPI and needs no geometry")
	}

	c.PanelAddr = 0x3D
	c.PanelCS, c.PanelDC = 10, 11
	c.PanelWidth, c.PanelHgt = 320, 240
	arg = c.Arg("r")
	for _, want := range []string{
		"panel-addr=61", "panel-cs=10", "panel-dc=11", "panel-w=320", "panel-h=240",
	} {
		if !has(t, arg, want) {
			t.Errorf("missing %q in %q", want, arg)
		}
	}
}

// A panel that nothing is listening to is not declared at all, so a board with
// a screen nobody asked to see boots without one.
func TestAPanelNeedsSomewhereToDraw(t *testing.T) {
	c := base()
	c.PanelAddr, c.PanelCS = 0x3C, 10
	if hasKey(t, c.Arg("r"), "panel-addr") {
		t.Error("a panel with no path has nowhere to draw and must not be declared")
	}
}

// The coprocessor lie is off unless asked for, by the build or by the
// environment, and the environment's spelling of "no" is honoured.
func TestCoprocessorAtResetIsOptIn(t *testing.T) {
	t.Setenv(qemu.EnvCoprocAtReset, "")
	if has(t, base().Arg("r"), "cp-at-reset=on") {
		t.Error("the coprocessor lie must be off by default")
	}
	c := base()
	c.CoprocAtReset = true
	if !has(t, c.Arg("r"), "cp-at-reset=on") {
		t.Error("a build that asks for it must get it")
	}

	for _, off := range []string{"0", "false", "FALSE"} {
		t.Setenv(qemu.EnvCoprocAtReset, off)
		if qemu.CoprocAtReset() {
			t.Errorf("%q should not turn it on", off)
		}
	}
	for _, on := range []string{"1", "yes", "on"} {
		t.Setenv(qemu.EnvCoprocAtReset, on)
		if !qemu.CoprocAtReset() {
			t.Errorf("%q should turn it on", on)
		}
	}
	t.Setenv(qemu.EnvCoprocAtReset, "1")
	if !has(t, base().Arg("r"), "cp-at-reset=on") {
		t.Error("the environment must be able to force it on for a board that did not ask")
	}
}

// Octal PSRAM is a property of the part, and is declared only when the board has it.
func TestOctalPSRAMIsDeclaredOnlyWhenPresent(t *testing.T) {
	if has(t, base().Arg("r"), "psram-octal=on") {
		t.Error("a board without octal PSRAM must not claim it")
	}
	c := base()
	c.PSRAMOctal = true
	if !has(t, c.Arg("r"), "psram-octal=on") {
		t.Error("a board with octal PSRAM must declare it")
	}
}
