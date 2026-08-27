package boardcheck

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The companion build, which is the one that uses the hardware.
//
// A repeater drives its display and nothing else on the front of the board.
// The companion is the whole handheld interface - a status bar with the cell
// voltage in it, a contact list, a keyboard - so it is what a screen, a
// keyboard and a battery reading have to be measured against. Every peripheral
// below the radio is proved here or not at all.
func TestTheCompanionDrawsItsInterface(t *testing.T) {
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
	e.Add(scenario.Node{
		Name: "bc-under-test", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.85}, HeightAGLm: 10,
		Antenna:    antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"},
		TxPowerDBm: 20, NoiseFigureDB: 6,
		Radio: scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
			SpreadFactor: 8, CodingRate: 4},
		Firmware: scenario.FirmwareRef{Role: "companion_radio",
			Version: version, Board: board},
	}, nil)
	if err := e.AttachNative(ctx, 4447); err != nil {
		t.Fatalf("attaching: %v", err)
	}
	under, ok := e.NodeByName("bc-under-test")
	if !ok || under.Firmware == nil {
		t.Fatal("the board never came up")
	}
	dev, ok := under.Firmware.Backend.(interface {
		Screen() (int, int, int, bool, []byte, bool)
		TypeKey(byte) error
		TouchScreen(int, int, bool) error
	})
	if !ok {
		t.Fatal("this backend has no screen")
	}

	started := time.Now()
	best, bw, bh, bpp := 0, 0, 0, 0
	var bits []byte
	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		settle(ctx, e, 5_000)
		w, h, bp, _, b, have := dev.Screen()
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
			best, bw, bh, bpp, bits = lit, w, h, bp, b
		}
		if best > 400 && time.Since(started) > settleFor() {
			break
		}
	}
	if best == 0 {
		t.Fatal("the companion never drew anything on its screen")
	}
	// Type at it, and see whether the picture changes. That is the whole
	// question a modelled keyboard has to answer: a device that answers a bus
	// scan proves it is there, not that anything reads it.
	before := lit(bits)
	writeShot(t, "before", bw, bh, bpp, bits)
	// A finger in the middle, put down and taken off. Together with the keys
	// below, that is every way somebody can reach this board's interface.
	if at := os.Getenv("MESHBENCH_TOUCH"); at != "" {
		// Where, in the panel's own pixels. A touch in the middle of a screen
		// whose controls are along the bottom proves nothing, which is what
		// the first run of this proved.
		// A sequence, separated by semicolons: an interface is reached by
		// getting to it, and one tap into the middle of whatever is showing
		// proves nothing. The last point is the one watched.
		var pts [][2]int
		for _, part := range strings.Split(at, ";") {
			var x, y int
			if n, err := fmt.Sscanf(strings.TrimSpace(part), "%d,%d", &x, &y); n == 2 && err == nil {
				pts = append(pts, [2]int{x, y})
			}
		}
		if len(pts) == 0 {
			pts = [][2]int{{bw / 2, bh / 2}}
		}
		tx, ty := pts[len(pts)-1][0], pts[len(pts)-1][1]
		for _, p := range pts[:len(pts)-1] {
			// A click, not a hold: press and release with nothing between
			// them, which is what a mouse sends and a finger never does.
			if err := dev.TouchScreen(p[0], p[1], true); err != nil {
				t.Fatalf("touching: %v", err)
			}
			if err := dev.TouchScreen(p[0], p[1], false); err != nil {
				t.Fatalf("releasing: %v", err)
			}
			settle(ctx, e, 2_000)
		}
		// The one that is watched. Every tap before it was only getting here.
		if err := dev.TouchScreen(tx, ty, true); err != nil {
			t.Fatalf("touching: %v", err)
		}
		if err := dev.TouchScreen(tx, ty, false); err != nil {
			t.Fatalf("releasing: %v", err)
		}
		settle(ctx, e, 3_000)
		if _, _, _, _, b, have := dev.Screen(); have {
			t.Logf("touched %d,%d: %d bytes lit before, %d after", tx, ty, before, lit(b))
			writeShot(t, "touched", bw, bh, bpp, b)
			bits, before = b, lit(b)
		}
	}
	for _, ch := range os.Getenv("MESHBENCH_TYPE") {
		if err := dev.TypeKey(byte(ch)); err != nil {
			t.Fatalf("typing: %v", err)
		}
		settle(ctx, e, 500)
	}
	if os.Getenv("MESHBENCH_TYPE") != "" {
		settle(ctx, e, 5_000)
		if _, _, _, _, b, have := dev.Screen(); have {
			t.Logf("typed %q: %d bytes lit before, %d after",
				os.Getenv("MESHBENCH_TYPE"), before, lit(b))
			bits = b
			writeShot(t, "after", bw, bh, bpp, b)
		}
	}
	t.Logf("busiest frame: %dx%d at %d bpp, %d bytes lit", bw, bh, bpp, best)
	if bpp != 1 {
		return
	}
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

// lit is how much of a frame is not blank, which is the cheapest thing that
// tells two pictures apart.
func lit(b []byte) int {
	n := 0
	for _, v := range b {
		if v != 0 {
			n++
		}
	}
	return n
}

// writeShot saves a frame where somebody can look at it, which is the only way
// a display is really checked: a count of lit bytes passes just as happily on
// a panel drawing noise.
func writeShot(t *testing.T, name string, w, h, bpp int, bits []byte) {
	t.Helper()
	dir := os.Getenv("MESHBENCH_SHOTS")
	if dir == "" {
		return
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Through the same reader the workbench draws with, rather than
			// a second copy of the arithmetic. Two decoders is how one of
			// them came to disagree about which byte comes first, and the
			// picture that would have shown it was only ever black and white.
			c := color.NRGBA{A: 255}
			if bpp == 1 {
				if bits[(y/8)*w+x]&(1<<(y%8)) != 0 {
					c = color.NRGBA{R: 220, G: 240, B: 255, A: 255}
				}
			} else if r, g, b, ok := emulated.RGB565At(bits, w, x, y); ok {
				c = color.NRGBA{R: r, G: g, B: b, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatalf("saving the frame: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encoding the frame: %v", err)
	}
	t.Logf("wrote %s", f.Name())
}

// settleFor is how long to let the board run before looking. A companion has a
// lot to do before it draws anything worth seeing - storage, radio, mesh - and
// how long that takes depends on the build, so it is set from outside rather
// than guessed here.
func settleFor() time.Duration {
	if v := os.Getenv("MESHBENCH_SETTLE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 90 * time.Second
}
