package boardcheck

import (
	"strings"
	"testing"
)

// The real log from the abort that took every ESP32 board down.
const abortedLog = `Adding SPI flash device
Unexpected error in qdev_prop_set_after_realize() at ../hw/core/qdev-properties.c:22:
qemu-system-xtensa: Attempt to set property 'apb_freq' on anonymous device (type 'esp_soc.uart') after it was realized
`

// A probe whose emulator never built the machine reported "boot passed -
// attached and answering" while the firmware had not executed an instruction.
// That report was read as evidence the board worked, on a machine where no
// board could start.
func TestAnAbortedEmulatorPassesNothing(t *testing.T) {
	r := BoardReport{Results: map[Capability]Result{}}
	for _, c := range Capabilities {
		r.set(c, Passed, "attached and answering")
	}

	r.downgradeIfAborted([]byte(abortedLog))

	for _, c := range Capabilities {
		got := r.Results[c]
		if got.State != Untested {
			t.Errorf("%v is %v after the emulator aborted, want %v", c, got.State, Untested)
		}
		if !strings.Contains(got.Detail, "apb_freq") {
			t.Errorf("%v does not carry what the emulator said: %q", c, got.Detail)
		}
		if !strings.Contains(got.Detail, "before the firmware ran") {
			t.Errorf("%v does not say the firmware never ran: %q", c, got.Detail)
		}
	}
}

// An emulator log that is only its ordinary chatter must not mark a good probe
// untested - which is the failure mode a broader pattern would have.
func TestOrdinaryEmulatorChatterIsNotAnAbort(t *testing.T) {
	for _, log := range []string{
		"Adding SPI flash device\n",
		"",
		"esp32_gpio: unimplemented device write (0x3ff44004)\n",
	} {
		r := BoardReport{Results: map[Capability]Result{}}
		r.set(Boot, Passed, "attached and answering")
		r.downgradeIfAborted([]byte(log))
		if got := r.Results[Boot].State; got != Passed {
			t.Errorf("log %q downgraded a passing boot to %v", log, got)
		}
	}
}

// The words that decide come back, so the report says why rather than only
// that something went wrong.
func TestTheAbortIsQuotedBack(t *testing.T) {
	if got := findAbort([]byte(abortedLog)); !strings.Contains(got, "apb_freq") {
		t.Errorf("findAbort returned %q", got)
	}
	if got := findAbort([]byte("Adding SPI flash device\n")); got != "" {
		t.Errorf("findAbort found %q in ordinary output", got)
	}
}
