// Command mockup renders the UX wireframes in docs/ux to PNG.
//
// The designs are generated rather than drawn so they can be regenerated when
// the design changes, and so a review can see what moved. Run:
//
//	go run ./tools/mockup
package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	m "github.com/A13xB0/meshcoresim/internal/mockup"
)

func main() {
	out := "docs/ux"
	if err := os.MkdirAll(out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	figures := []struct {
		name string
		draw func() *m.Canvas
	}{
		{"01-workbench", workbench},
		{"02-path-profile", pathProfile},
		{"03-link-budget", linkBudget},
		{"04-reception-ledger", receptionLedger},
		{"05-energy", energy},
		{"06-consoles", consoles},
		{"07-interference", interference},
		{"08-coverage-rasters", coverage},
	}
	for _, f := range figures {
		path := filepath.Join(out, f.name+".png")
		fh, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := png.Encode(fh, f.draw().Img); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := fh.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
