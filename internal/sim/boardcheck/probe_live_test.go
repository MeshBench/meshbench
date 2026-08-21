package boardcheck

import (
	"context"
	"os"
	"testing"
	"time"
)

// flat is a terrain that answers everywhere, so a probe measures the board
// rather than the ground under three imaginary nodes.
type flat struct{}

func (flat) ElevationM(_, _ float64) (float64, bool) { return 0, true }

// One board, one real emulator boot.
//
//	MESHCORESIM_LIVE=1 MESHCORESIM_QEMU=~/.cache/meshcoresim/tools/qemu-system-xtensa \
//	  MESHCORESIM_BOARD=Generic_E22_sx1262 MESHCORESIM_BOARD_VERSION=v1.17.1 \
//	  go test ./internal/sim/boardcheck -run TestProbeOneBoard -v
//
// Deliberately one board and never the catalogue: a probe runs a published
// image under an emulator alongside the native peers it talks to, and starting
// eleven of those at once takes the machine down rather than measuring
// anything.
func TestProbeOneBoard(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	board := os.Getenv("MESHCORESIM_BOARD")
	if board == "" {
		board = "Generic_E22_sx1262"
	}
	version := os.Getenv("MESHCORESIM_BOARD_VERSION")
	if version == "" {
		version = "v1.17.1"
	}
	// Longer than the sum of the phases that can wait, so a probe reports what
	// it measured rather than being cut off mid-phase and reporting nothing.
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	report := Probe(ctx, flat{}, board, version)
	for _, c := range Capabilities {
		r := report.Results[c]
		t.Logf("%-6s %-9s %s", c, r.State, r.Detail)
	}
	if err := report.Save(); err != nil {
		t.Errorf("saving the report: %v", err)
	}
	if report.Results[Build].State != Passed {
		t.Skipf("no image for %s %s, so nothing was measured", board, version)
	}
}
