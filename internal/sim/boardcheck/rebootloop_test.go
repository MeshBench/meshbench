package boardcheck

import (
	"os"
	"strings"
	"testing"
)

// A board that restarts for ever must not read as one that booted.
//
// This is the case the boot row got wrong: an emulated part advances its clock
// whether or not its core is getting anywhere, so "attached and kept its
// clock" passed against an ESP32-S3 that asserted in ESP-IDF's startup and
// restarted thirty times in a single run.
func TestARebootLoopIsNotABoot(t *testing.T) {
	log, err := os.ReadFile("testdata/rebootloop.log")
	if err != nil {
		t.Fatalf("reading the captured console: %v", err)
	}
	starts, why := looping(log)
	if starts < rebootLoopMin {
		t.Fatalf("counted %d starts in a log of a board that never stopped restarting", starts)
	}
	if !strings.HasPrefix(why, "assert failed:") {
		t.Fatalf("the board said %q, which is not what it was seen to say", why)
	}

	r := untestedReport("Heltec_v3", "v1.17.1")
	r.set(Boot, Passed, "attached and answering")
	r.set(Radio, Failed, "never transmitted")
	r.downgradeIfRebooting(log)

	if got := r.Results[Boot].State; got != Failed {
		t.Errorf("boot is %v against a board that never finished starting", got)
	}
	if !strings.Contains(r.Results[Boot].Detail, "assert failed:") {
		t.Errorf("boot does not say what the board said: %q", r.Results[Boot].Detail)
	}
	// Untested, not failed: firmware that never ran did not decline anything.
	if got := r.Results[Radio].State; got != Untested {
		t.Errorf("radio is %v, but the board was never running to be measured", got)
	}
}

// A board that resets once on the way up is not looping.
func TestOneResetIsNotALoop(t *testing.T) {
	log := []byte("ESP-ROM:esp32s3-20210327\nentry 0x403c98d0\nRepeater ID: AA\n")
	r := untestedReport("Heltec_v3", "v1.17.1")
	r.set(Boot, Passed, "attached and answering")
	r.downgradeIfRebooting(log)
	if got := r.Results[Boot].State; got != Passed {
		t.Errorf("boot is %v against a board that started once and kept going", got)
	}
}
