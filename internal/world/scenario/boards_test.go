package scenario_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Every figure in a board profile has to be defensible. These are the checks
// that catch a copied-and-edited profile with a field left from the one above.
func TestBoardProfilesArePlausible(t *testing.T) {
	for _, b := range scenario.Boards() {
		if b.Name == "" || b.MCU == "" || b.Radio == "" {
			t.Errorf("%+v is missing an identity field", b)
		}
		if b.MaxTxDBm < 10 || b.MaxTxDBm > 33 {
			t.Errorf("%s: %.0f dBm is not a LoRa board", b.Name, b.MaxTxDBm)
		}
		// SF12/BW125 sensitivity lives in a narrow band across every SX1262
		// board; a figure outside it is a typo, not a design difference.
		if b.SensitivityDBm > -130 || b.SensitivityDBm < -142 {
			t.Errorf("%s: sensitivity %.0f dBm is outside anything an SX1262 achieves", b.Name, b.SensitivityDBm)
		}
		if b.FeedlineDB < 0 || b.FeedlineDB > 3 {
			t.Errorf("%s: %.1f dB of board loss is not credible", b.Name, b.FeedlineDB)
		}
		if b.NoiseFigureDB < 3 || b.NoiseFigureDB > 12 {
			t.Errorf("%s: noise figure %.0f dB", b.Name, b.NoiseFigureDB)
		}
		if b.Notes == "" {
			t.Errorf("%s: no notes; every profile should say what an engineer needs to know", b.Name)
		}
	}
}

// The number people quote is the datasheet transmit power. The number that
// reaches the far end is that minus the board's losses plus its antenna, and on
// a board with a chip antenna the difference is most of a decade of range.
func TestRadiatedPowerIsNotDatasheetPower(t *testing.T) {
	xiao, err := scenario.BoardByName("Xiao_S3_WIO")
	if err != nil {
		t.Fatal(err)
	}
	rak, err := scenario.BoardByName("rak4631") // case-insensitive on purpose
	if err != nil {
		t.Fatal(err)
	}

	if xiao.RadiatedDBm(22) >= xiao.MaxTxDBm {
		t.Errorf("a board with a -2 dBi antenna radiated %.1f dBm from a %.0f dBm chip",
			xiao.RadiatedDBm(22), xiao.MaxTxDBm)
	}
	// The two boards quote the same chip power and are several dB apart in
	// practice. That gap is the entire reason this type exists.
	gap := rak.RadiatedDBm(22) - xiao.RadiatedDBm(22)
	if gap < 3 {
		t.Errorf("a whip and a chip antenna differ by only %.1f dB", gap)
	}

	// Drive above the chip's maximum is clamped rather than extrapolated.
	if rak.RadiatedDBm(40) != rak.RadiatedDBm(rak.MaxTxDBm) {
		t.Error("transmit power was extrapolated beyond the chip's maximum")
	}
}

// A scenario built around a board that cannot be emulated should say so at
// build time rather than fail at run time.
func TestEmulationSupportIsStated(t *testing.T) {
	ok, blocked := scenario.EmulatableBoards()
	if len(ok) == 0 {
		t.Fatal("no board can be emulated, which contradicts MSIM-13")
	}
	for _, b := range ok {
		// Offered means booted, not merely configured. Wiring read off a
		// config file and never run is exactly the kind of thing that looks
		// like support and is not.
		//
		// Either emulator counts. An nRF52 board is wired for Renode and has
		// no QEMU machine at all, so asking every offered board for a verified
		// QEMU wiring made a Renode board impossible to offer however
		// thoroughly it had been booted.
		switch {
		case b.QEMU != nil:
			if !b.QEMU.Verified {
				t.Errorf("%s is offered for emulation without verified wiring", b.Name)
			}
		case b.Renode != nil:
			// Renode wiring carries no Verified flag of its own; the
			// EmulationVerified list is what says a board has been booted,
			// and EmulatableBoards has already required it.
		default:
			t.Errorf("%s is offered for emulation with no wiring at all", b.Name)
		}
	}
	// Every board that is not offered owes the operator a reason, because a
	// board missing without one reads as a bug in the picker.
	for _, b := range scenario.Boards() {
		offered := false
		for _, o := range ok {
			offered = offered || o.Name == b.Name
		}
		if !offered && blocked[b.Name] == "" {
			t.Errorf("%s is neither offered nor explained", b.Name)
		}
	}
}

// The wiring is per board and it is load-bearing: the radio sits on a different
// SPI controller and different pins from one board to the next, and a wrong pin
// produces a driver reporting no chip, which reads as a broken emulator.
func TestEmulationWiringIsComplete(t *testing.T) {
	ok, _ := scenario.EmulatableBoards()
	for _, b := range ok {
		if b.QEMU == nil {
			// A Renode board's wiring is checked below on its own terms:
			// these fields are the ESP32's SPI controllers and mean nothing
			// to an nRF52.
			if b.Renode.Platform == "" || b.Renode.SPIBase == 0 {
				t.Errorf("%s has Renode wiring with no platform or SPI base", b.Name)
			}
			if b.Renode.NssPort == "" || b.Renode.IrqPort == "" {
				t.Errorf("%s is missing NSS or DIO1: without them the driver "+
					"reports no chip, which reads as a broken emulator", b.Name)
			}
			continue
		}
		w := b.QEMU
		if w.Machine == "" {
			t.Errorf("%s has no QEMU machine type", b.Name)
		}
		if w.NSS == 0 || w.Busy == 0 {
			t.Errorf("%s is missing NSS or BUSY; without NSS a command cannot be framed",
				b.Name)
		}
		if w.SPI < 0 || w.SPI > 3 {
			t.Errorf("%s wants SPI%d, and the ESP32 has four controllers", b.Name, w.SPI)
		}
	}
}

// Sleep current is where a datasheet and a real board diverge most, so the
// profiles must not all quote the MCU's own figure.
func TestSleepCurrentReflectsTheBoardNotTheMCU(t *testing.T) {
	var distinct = map[float64]bool{}
	for _, b := range scenario.Boards() {
		distinct[b.SleepUA] = true
	}
	if len(distinct) < 4 {
		t.Errorf("only %d distinct sleep currents across %d boards; these look copied",
			len(distinct), len(scenario.Boards()))
	}

	xiao, _ := scenario.BoardByName("Xiao_nRF52840")
	heltec, _ := scenario.BoardByName("Heltec_v3")
	if xiao.SleepUA >= heltec.SleepUA {
		t.Error("a bare nRF52840 should sleep far below an ESP32 board with a regulator and an LED")
	}
	if xiao.Load().SleepUA != xiao.SleepUA {
		t.Error("the board's sleep current is not reaching its electrical model")
	}
}

func TestUnknownBoardListsWhatExists(t *testing.T) {
	_, err := scenario.BoardByName("nonesuch")
	if err == nil {
		t.Fatal("an unknown board was accepted")
	}
	if !strings.Contains(err.Error(), "RAK_4631") {
		t.Errorf("the error should list what is available: %v", err)
	}
}

// A board is named for the build that produces its image.
//
// Both halves of this were once wrong at the same time: a "RAK4631" that no
// release published sat beside the "RAK_4631" that did, and a Xiao named for
// its chip could never be given an image because the release names it for its
// variant. Neither failed loudly - the boards simply stayed grey in the matrix.
func TestBoardsAreNamedForTheirBuild(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range scenario.Boards() {
		if seen[strings.ToLower(b.Name)] {
			t.Errorf("two profiles named %q", b.Name)
		}
		seen[strings.ToLower(b.Name)] = true
	}
	// A rename keeps the old name working: fixtures on disk name their boards.
	for _, old := range []string{"RAK4631", "Xiao_nRF52840"} {
		b, err := scenario.BoardByName(old)
		if err != nil {
			t.Errorf("a fixture naming %q can no longer be loaded: %v", old, err)
			continue
		}
		if seen[strings.ToLower(old)] {
			t.Errorf("%q was renamed, yet still exists as a profile", old)
		}
		if b.Name == old {
			t.Errorf("%q resolved to itself rather than to its new name", old)
		}
	}
}
