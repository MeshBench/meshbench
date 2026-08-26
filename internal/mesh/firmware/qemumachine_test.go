package firmware

import (
	"strings"
	"testing"
)

// The coprocessor lie is off unless somebody asks for it.
//
// It matters that this is opt-in and stays opt-in: with it on the machine
// reports a register the way silicon does not, and a firmware that genuinely
// mismanages its floating point enable would be flattered rather than caught.
// It exists because one firmware's exception handler reaches for the FPU
// before anything has enabled it, takes a fatal trap inside an exception
// vector, and loops there - hiding everything behind it.
func TestTheCoprocessorLieIsOptIn(t *testing.T) {
	e := &EmulatedNode{Machine: "esp32s3", SPI: 3, NSS: 9, Busy: 13}

	if got := e.machineString("radio.sock"); strings.Contains(got, "cp-at-reset") {
		t.Errorf("the machine asks for enabled coprocessors by default: %s", got)
	}

	t.Setenv(EnvCoprocAtReset, "1")
	if got := e.machineString("radio.sock"); !strings.Contains(got, "cp-at-reset=on") {
		t.Errorf("asking for it did not reach the machine: %s", got)
	}

	// A value that plainly means no is no, so an exported "0" left in a shell
	// does not quietly change what a board is.
	for _, off := range []string{"0", "false", "FALSE", ""} {
		t.Setenv(EnvCoprocAtReset, off)
		if got := e.machineString("radio.sock"); strings.Contains(got, "cp-at-reset") {
			t.Errorf("%q was read as yes: %s", off, got)
		}
	}
}

// The same switch, asked for by the build rather than by the environment.
//
// Which is the way it is meant to be reached: it is a property of the firmware
// being looked at, so the same board runs one image that needs it and another
// that would be flattered by it. The environment stays as the way a script
// forces it on for everything at once.
func TestABuildCanAskForEnabledCoprocessorsItself(t *testing.T) {
	t.Setenv(EnvCoprocAtReset, "")
	plain := &EmulatedNode{Machine: "esp32s3", SPI: 3, NSS: 9, Busy: 13}
	asked := &EmulatedNode{Machine: "esp32s3", SPI: 3, NSS: 9, Busy: 13,
		CoprocAtReset: true}

	if got := plain.machineString("radio.sock"); strings.Contains(got, "cp-at-reset") {
		t.Errorf("a build that asked for nothing got it anyway: %s", got)
	}
	if got := asked.machineString("radio.sock"); !strings.Contains(got, "cp-at-reset=on") {
		t.Errorf("the build asked and the machine did not hear: %s", got)
	}
}
