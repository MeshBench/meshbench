package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

func boosted(name string) radioSample {
	return radioSample{name, firmware.RadioStats{Configured: true, RxGainReg: firmware.RxGainBoosted}}
}

func saving(name string) radioSample {
	return radioSample{name, firmware.RadioStats{Configured: true, RxGainReg: firmware.RxGainPowerSaving}}
}

func TestAnArmThatReachedEveryRadioIsQuiet(t *testing.T) {
	arm := ExpArm{Set: map[string]string{"radio.rxgain": "on"}}
	if c := armComplaints(arm, []radioSample{boosted("a"), boosted("b")}, 0); len(c) != 0 {
		t.Fatalf("expected no complaint, got %v", c)
	}
}

// The fault the whole check exists for: the arm asked for boosted gain and the
// chips are in power saving, which is what MeshCore's AGC reset does to a
// variant that does not define the compile-time macro.
func TestAnArmWhoseGainNeverLandedFailsTheCell(t *testing.T) {
	arm := ExpArm{Set: map[string]string{"radio.rxgain": "on"}}
	c := armComplaints(arm, []radioSample{boosted("a"), saving("b"), saving("c")}, 0)
	if len(c) != 1 {
		t.Fatalf("expected one complaint, got %v", c)
	}
	if !strings.Contains(c[0], "2 of 3") || !strings.Contains(c[0], "0x96") {
		t.Fatalf("complaint should name the count and the register it wanted: %q", c[0])
	}
}

func TestRxGainOffIsCheckedTheSameWay(t *testing.T) {
	arm := ExpArm{Set: map[string]string{"radio.rxgain": "off"}}
	if c := armComplaints(arm, []radioSample{saving("a")}, 0); len(c) != 0 {
		t.Fatalf("power saving satisfies rxgain off: %v", c)
	}
	c := armComplaints(arm, []radioSample{boosted("a")}, 0)
	if len(c) != 1 || !strings.Contains(c[0], "0x94") {
		t.Fatalf("boosted should fail rxgain off and name 0x94: %v", c)
	}
}

// A node that never answered is not a node that agreed. Silence used to be the
// shape of every fault in this apparatus, so it is reported rather than skipped.
func TestSilentRadiosAreReportedNotAssumedWell(t *testing.T) {
	arm := ExpArm{Set: map[string]string{"radio.rxgain": "on"}}
	c := armComplaints(arm, []radioSample{boosted("a")}, 4)
	if len(c) != 1 || !strings.Contains(c[0], "4 of 5") {
		t.Fatalf("expected the silent nodes counted: %v", c)
	}
}

func TestNothingAnsweredIsNotAPass(t *testing.T) {
	arm := ExpArm{Set: map[string]string{"radio.rxgain": "on"}}
	c := armComplaints(arm, nil, 3)
	if len(c) != 1 || !strings.Contains(c[0], "cannot be confirmed") {
		t.Fatalf("a cell where nothing reported must not pass silently: %v", c)
	}
}

// An arm that pins nothing observable has nothing to prove, and must not be
// failed for it - most arms vary firmware version alone.
func TestAnArmWithNothingObservableIsQuiet(t *testing.T) {
	arm := ExpArm{Set: map[string]string{"agc.reset.interval": "4"}}
	if c := armComplaints(arm, []radioSample{saving("a")}, 0); len(c) != 0 {
		t.Fatalf("agc.reset.interval is not readable off the chip, so nothing to say: %v", c)
	}
}

// An arm pinning only settings the chip cannot report must not be failed for
// silence: a firmware version is not a register, and most arms vary only that.
// This is what made every cell fail the first time the check was wired in.
func TestSilenceOnlyMattersWhenSomethingObservableWasPinned(t *testing.T) {
	arm := ExpArm{RepeaterVersion: "repeater-v1.17.1"}
	if c := armComplaints(arm, nil, 56); len(c) != 0 {
		t.Fatalf("an arm with nothing observable must tolerate silent radios: %v", c)
	}
}
