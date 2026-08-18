package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/companion/proto"
)

// The whole point of filling unset fields from the node: the firmware sets all
// four at once, so a frame with three of them zero fails its range check and
// leaves every field unchanged - including the one that was asked for.
func TestChangingOneModemFieldKeepsTheOthers(t *testing.T) {
	c := &compSession{node: "n", self: &proto.SelfInfo{
		FreqKHz: 869525, BWKHz: 250, SF: 11, CR: 5,
	}}
	frame, ok, err := radioFrame(c, map[string]any{"sf": float64(8)})
	if err != nil || !ok {
		t.Fatalf("wanted a frame, got ok=%v err=%v", ok, err)
	}
	want := proto.SetRadioParams(869525, 250000, 8, 5)
	if string(frame) != string(want) {
		t.Errorf("frame % x, wanted % x", frame, want)
	}
}

// Bandwidth is the one that has already cost a day: it is read in kHz and
// written in Hz, and sending 250 where the firmware wants 250000 fails the
// range check and silently leaves frequency unset too.
func TestBandwidthGoesOutInHz(t *testing.T) {
	c := &compSession{node: "n", self: &proto.SelfInfo{FreqKHz: 869525, BWKHz: 250, SF: 11, CR: 5}}
	frame, _, err := radioFrame(c, map[string]any{"bw_khz": float64(125)})
	if err != nil {
		t.Fatal(err)
	}
	if string(frame) != string(proto.SetRadioParams(869525, 125000, 11, 5)) {
		t.Errorf("bandwidth was not converted to Hz: % x", frame)
	}
}

func TestNoModemFieldsMeansNoModemFrame(t *testing.T) {
	c := &compSession{node: "n", self: &proto.SelfInfo{FreqKHz: 869525}}
	if _, ok, err := radioFrame(c, map[string]any{"name": "x"}); ok || err != nil {
		t.Errorf("a name-only configure should not touch the modem: ok=%v err=%v", ok, err)
	}
}

// A partial change needs something to be partial against, and inventing it
// would be inventing a modem setting.
func TestAPartialChangeNeedsTheNodeToHaveAnswered(t *testing.T) {
	c := &compSession{node: "kestrel"}
	_, _, err := radioFrame(c, map[string]any{"sf": float64(8)})
	if err == nil {
		t.Fatal("wanted a refusal when the node has not said what its radio is")
	}
	if !strings.Contains(err.Error(), "kestrel") {
		t.Errorf("the refusal should name the node, got %q", err)
	}
}

func TestModemValuesAreRangeCheckedHere(t *testing.T) {
	c := &compSession{node: "n", self: &proto.SelfInfo{FreqKHz: 869525, BWKHz: 250, SF: 11, CR: 5}}
	for _, tc := range []struct {
		name   string
		params map[string]any
		says   string
	}{
		{"frequency", map[string]any{"freq_khz": float64(100)}, "150"},
		{"bandwidth", map[string]any{"bw_khz": float64(900)}, "bandwidth"},
		{"spreading factor", map[string]any{"sf": float64(13)}, "spreading factor"},
		{"coding rate", map[string]any{"cr": float64(9)}, "coding rate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := radioFrame(c, tc.params)
			if err == nil {
				t.Fatal("wanted a refusal")
			}
			// Named, so the operator is not left with the firmware's
			// ERR_CODE_ILLEGAL_ARG and four fields to guess between.
			if !strings.Contains(err.Error(), tc.says) {
				t.Errorf("the refusal should say which field, got %q", err)
			}
		})
	}
}

// Three, not four. The two-bit field in the path-length byte makes four look
// reachable and the firmware rejects it.
func TestThePathHashSizesAreTheOnesTheFirmwareAccepts(t *testing.T) {
	if got := pathHashModes; len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("path hash sizes are %v, wanted 1, 2 and 3 bytes", got)
	}
	for bytes, wantMode := range map[uint8]uint8{1: 0, 2: 1, 3: 2} {
		frame := proto.SetPathHashMode(bytes - 1)
		if proto.Command(frame[0]) != proto.CmdSetPathHashMode {
			t.Errorf("%d bytes did not make a path hash command", bytes)
		}
		if frame[2] != wantMode {
			t.Errorf("%d bytes sent mode %d, wanted %d", bytes, frame[2], wantMode)
		}
	}
}
