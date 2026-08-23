package workbench

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Draw real panels on both grounds and look at them.
//
// The palette and the faces are the one part of this interface that no
// assertion can judge: whether the accent reads as interactive, whether the
// status colours are still legible on paper, and whether a column of figures
// lines up are all questions for an eye. So this renders to a file, the same
// way the Hardware tab does, and a person opens it.
//
// The panels are the audit's, because they are already wired to a recorder
// with something in every list - a picture of an empty panel would show the
// ground and nothing standing on it.
func TestDrawTheThemes(t *testing.T) {
	if os.Getenv("MESHCORESIM_SHOTS") == "" {
		t.Skip("set MESHCORESIM_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHCORESIM_SHOTS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	snap := auditSnapshot()
	// Configuration carries the widest spread of controls in one place -
	// boxes, dropdowns, switches and units - so it shows the most per frame.
	wanted := map[string]bool{"Configuration": true, "Sweep": true, "Fleet": true}

	for _, mode := range []struct {
		name string
		m    theme.Mode
	}{{"dark", theme.Dark}, {"light", theme.Light}} {
		for _, tg := range auditTargets(&recorder{}) {
			if !wanted[tg.name] {
				continue
			}
			use := snap
			if tg.snap != nil {
				use = tg.snap
			}
			img := renderMode(t, 1100, 820, mode.m,
				func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
					return tg.draw(th, gtx, use)
				})
			out := filepath.Join(dir, "theme-"+mode.name+"-"+tg.name+".png")
			f, err := os.Create(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", out)
		}
	}
}
