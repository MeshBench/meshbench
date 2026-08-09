package scenario_test

import (
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/scenario"
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
	emulatable := scenario.EmulatableBoards()
	if len(emulatable) == 0 {
		t.Fatal("no board can be emulated, which contradicts MSIM-13")
	}
	for _, name := range emulatable {
		b, err := scenario.BoardByName(name)
		if err != nil {
			t.Fatal(err)
		}
		// Only the nRF52840 path runs today; the ESP32 one is blocked on a
		// radio model. A board claiming otherwise is a documentation bug.
		if b.MCU != "nRF52840" {
			t.Errorf("%s claims emulation on %s, but only nRF52840 runs today", name, b.MCU)
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
	if !strings.Contains(err.Error(), "RAK4631") {
		t.Errorf("the error should list what is available: %v", err)
	}
}
