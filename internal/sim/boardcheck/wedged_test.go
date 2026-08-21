package boardcheck

import "testing"

// Lines in the shape Renode writes them, including the collapsed-repeat
// counter, because that counter is most of the count.
const (
	probing = `[01:45:50.8388] [WARNING] sysbus: [cpu: 0x7283C] ReadDoubleWord from non existing peripheral at 0x5002B910.
[01:45:50.8389] [WARNING] sysbus: [cpu: 0x7283C] ReadDoubleWord from non existing peripheral at 0x5002B928.
`
	hanging = `[01:47:14.2204] [WARNING] sysbus: [cpu: 0x76D50] ReadDoubleWord from non existing peripheral at 0x5002B0B4.
[01:47:14.2204] [WARNING] sysbus: [cpu: 0x76D50] ReadDoubleWord from non existing peripheral at 0x5002B0B4. (10000)
[01:47:14.2205] [WARNING] sysbus: [cpu: 0x76D50] ReadDoubleWord from non existing peripheral at 0x5002B0B4. (9516)
`
)

// A driver asking whether a peripheral is fitted looks nothing like a hang,
// and must not be read as one - this is what the board that relays does.
func TestProbingIsNotAWedge(t *testing.T) {
	if w, ok := findWedge([]byte(probing)); ok {
		t.Fatalf("two identifier reads called a wedge: %+v", w)
	}
}

func TestWedgeCountsCollapsedRepeats(t *testing.T) {
	w, ok := findWedge([]byte(probing + hanging))
	if !ok {
		t.Fatal("19,517 reads of one address not called a wedge")
	}
	if w.Addr != "0x5002B0B4" || w.PC != "0x76D50" {
		t.Errorf("wrong site: %+v", w)
	}
	if want := 1 + 10000 + 9516; w.Reads != want {
		t.Errorf("Reads = %d, want %d - the collapsed counter is the measurement", w.Reads, want)
	}
}

// A failure the board was never awake to earn becomes untested; a pass it was
// watched earning stays a pass.
func TestDowngradeKeepsWhatWasSeen(t *testing.T) {
	r := untestedReport("RAK_4631", "v1.17.0")
	r.set(Boot, Passed, "attached and answering")
	r.set(TX, Passed, "adverted unprompted at 64.0 s")
	r.set(Flood, Failed, "put nothing back on the air within 240 s")

	r.downgradeIfWedged([]byte(hanging))

	if got := r.Results[Flood].State; got != Untested {
		t.Errorf("Flood = %q, want %q", got, Untested)
	}
	if got := r.Results[Flood].Detail; got == "" {
		t.Error("a downgrade with no reason sends someone back to the logs")
	}
	if got := r.Results[TX].State; got != Passed {
		t.Errorf("TX = %q, want %q - a hang cannot un-happen a transmission", got, Passed)
	}
	if got := r.Results[Boot].State; got != Passed {
		t.Errorf("Boot = %q, want %q", got, Passed)
	}
}

func TestNoWedgeChangesNothing(t *testing.T) {
	r := untestedReport("Heltec_t114", "v1.17.0")
	r.set(Flood, Failed, "put nothing back on the air within 240 s")
	r.downgradeIfWedged([]byte(probing))
	if got := r.Results[Flood].State; got != Failed {
		t.Errorf("Flood = %q, want %q - without a hang a failure is a failure", got, Failed)
	}
}

func TestCommas(t *testing.T) {
	for in, want := range map[int]string{0: "0", 1: "1", 999: "999", 1000: "1,000",
		12356: "12,356", 124570001: "124,570,001"} {
		if got := commas(in); got != want {
			t.Errorf("commas(%d) = %q, want %q", in, got, want)
		}
	}
}
