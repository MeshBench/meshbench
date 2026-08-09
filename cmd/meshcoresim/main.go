// Command meshcoresim is the MeshCore network simulator workbench.
//
// It needs a GPU and a display, so it does not run on the development VM —
// see CLAUDE.md. CI exercises the CPU reference path via the packages, not
// this binary.
package main

import (
	"fmt"
	"os"

	"github.com/A13xB0/meshcoresim/internal/dsp"
)

func main() {
	// Placeholder until the UI lands (MSIM-10). Printing the channel's noise
	// floor is a deliberate first output: it is the number every sensitivity
	// claim is measured against, and it is cheap to sanity-check by hand.
	for _, bw := range []float64{125_000, 250_000, 500_000} {
		fmt.Printf("BW %7.0f Hz  noise floor %7.2f dBm (NF %.0f dB)\n",
			bw, dsp.NoiseFloorDBm(bw, dsp.DefaultNoiseFigureDB), dsp.DefaultNoiseFigureDB)
	}
	os.Exit(0)
}
