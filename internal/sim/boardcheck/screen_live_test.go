package boardcheck

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// Does a board with a declared screen actually draw one?
//
// Run for each board that declares an I2C panel. Three have been through it
// and drew MeshCore's own boot screen: the Heltec V3, the Heltec V4 and the
// LilyGo T3S3 - three different sets of radio pins and three different RAM
// configurations, all reaching the same picture.
//
// Watched through the same path the workbench will use - the board's own
// declaration decides whether a display exists, the emulator sends its
// framebuffer down a socket, and this reads what arrived. A test that stopped
// at "bytes were sent" would pass against a panel drawing noise.
func TestTheBoardDrawsItsScreen(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	board := os.Getenv("MESHCORESIM_BOARD")
	if board == "" {
		t.Skip("set MESHCORESIM_BOARD")
	}
	version := os.Getenv("MESHCORESIM_BOARD_VERSION")
	if version == "" {
		version = "v1.17.1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417, UnverifiedWiring: true,
	})
	defer func() { _ = e.Close() }()
	for _, n := range probeGeometry(board, version) {
		e.Add(n, nil)
	}
	if err := e.AttachNative(ctx, 4431); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		t.Fatal("the board never came up")
	}
	screen, ok := under.Firmware.Backend.(interface {
		Screen() (int, int, bool, []byte, bool)
	})
	if !ok {
		t.Fatal("this backend has no display to read")
	}

	// The busiest frame, not the last. MeshCore turns the display off after an
	// idle, so the final picture of a run is blank on purpose and the one
	// worth looking at is somewhere in the middle.
	best, bw, bh := 0, 0, 0
	var bits []byte
	deadline := time.Now().Add(6 * time.Minute)
	for time.Now().Before(deadline) {
		settle(ctx, e, 5_000)
		w, h, _, b, have := screen.Screen()
		if !have {
			continue
		}
		lit := 0
		for _, v := range b {
			if v != 0 {
				lit++
			}
		}
		if lit > best {
			best, bw, bh, bits = lit, w, h, b
		}
		if best > 200 {
			break
		}
	}
	if best == 0 {
		t.Fatal("the board never drew anything on its screen")
	}
	t.Logf("busiest frame: %dx%d, %d bytes lit", bw, bh, best)

	// Drawn as text so a failure says what was on the screen rather than that
	// a number was wrong.
	for y := 0; y < bh; y++ {
		var row strings.Builder
		for x := 0; x < bw; x++ {
			if bits[(y/8)*bw+x]&(1<<(y%8)) != 0 {
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
