package boardcheck

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Does pressing the board's own button reach the firmware?
//
// Asked the way somebody would find out with the board in their hand: leave it
// alone until MeshCore switches the display off, hold the program button,
// let go, and see whether the screen comes back. Nothing else in the run
// changes, so a screen that lights is a press that arrived.
//
// This is worth more than checking a pin moved. A pin that moves and a
// firmware that notices are different claims, and only the second one means
// the button works.
func TestPressingTheButtonWakesTheScreen(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	board := os.Getenv("MESHBENCH_BOARD")
	if board == "" {
		t.Skip("set MESHBENCH_BOARD")
	}
	version := os.Getenv("MESHBENCH_BOARD_VERSION")
	if version == "" {
		version = "v1.17.1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417, UnverifiedWiring: true,
	})
	defer func() { _ = e.Close() }()
	for _, n := range probeGeometry(board, version) {
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, 4435); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		t.Fatal("the board never came up")
	}
	dev, ok := under.Firmware.Backend.(interface {
		Screen() (int, int, int, bool, []byte, bool)
		PressButton(int, bool) error
	})
	if !ok {
		t.Fatal("this backend has neither a screen nor buttons")
	}

	pin := 0 // the program button on every Heltec board here
	if p := os.Getenv("MESHBENCH_BUTTON"); p != "" {
		if n, err := parseInt(p); err == nil {
			pin = n
		}
	}

	// Wait for the firmware to put the display to sleep on its own. Not a
	// fixed wait: how long that takes is the firmware's business and it has
	// changed between versions.
	slept := false
	deadline := time.Now().Add(8 * time.Minute)
	for time.Now().Before(deadline) && !slept {
		settle(ctx, e, 5_000)
		if _, _, _, on, _, have := dev.Screen(); have && !on {
			slept = true
		}
	}
	if !slept {
		t.Skip("the screen never went to sleep, so there is nothing to wake")
	}
	t.Log("the firmware switched the display off")

	if err := dev.PressButton(pin, true); err != nil {
		t.Fatalf("holding the button: %v", err)
	}
	settle(ctx, e, 500)
	if err := dev.PressButton(pin, false); err != nil {
		t.Fatalf("releasing the button: %v", err)
	}
	t.Logf("held pin %d and let go", pin)

	woke := false
	var w, h int
	var bits []byte
	deadline = time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) && !woke {
		settle(ctx, e, 2_000)
		if ww, hh, _, on, b, have := dev.Screen(); have && on {
			woke, w, h, bits = true, ww, hh, b
		}
	}
	if !woke {
		t.Fatal("the screen stayed off, so the press never reached the firmware")
	}
	t.Log("the screen came back")

	for y := 0; y < h; y++ {
		var row strings.Builder
		for x := 0; x < w; x++ {
			if bits[(y/8)*w+x]&(1<<(y%8)) != 0 {
				row.WriteByte('#')
			} else {
				row.WriteByte(' ')
			}
		}
		if strings.TrimSpace(row.String()) != "" {
			t.Logf("|%s|", row.String())
		}
	}
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
