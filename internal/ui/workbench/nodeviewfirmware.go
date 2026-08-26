// Firmware from the node table: what a node is running, what it could run,
// and the script it is told at boot.
package workbench

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// buildChoice is one thing a node could be told to run.
//
// A board image is not a version on its own - "wadamesh" means nothing until
// it is wadamesh for a LilyGo_TDeck, built as a companion - so the label says
// so and the three fields travel together to the verb. A host build carries
// neither, which is what it is.
type buildChoice struct {
	Label   string
	Version string
	Board   string
	Role    string
}

// installedBuilds is every build this machine has: the ones for this machine,
// and the images downloaded or imported for a board.
//
// Board images used to be filtered out here, which meant a build somebody had
// just imported for their board could not then be put on a node - the import
// offered every board and the picker offered none of them.
func installedBuilds() []buildChoice {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []buildChoice
	for _, f := range firmware.ListInstalled(filepath.Join(cache, "meshbench", "firmware")) {
		c := buildChoice{Version: f.Version, Label: f.Version}
		if !f.Native {
			c.Board, c.Role = f.Board, f.Role
			// Named board first, because that is the question somebody is
			// answering: which hardware, then which build for it.
			c.Label = f.Board + " - " + f.Role + " " + f.Version
		}
		key := c.Label
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		// Host builds first: they are what most nodes run, and a list that
		// opens on somebody else's hardware reads as the wrong list.
		if (out[i].Board == "") != (out[j].Board == "") {
			return out[i].Board == ""
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// cpuTime prints processor time at a scale somebody can read.
//
// Milliseconds while a node has barely run, then seconds - rather than a
// percentage, which for fifty nodes ticking over is fifty readings of 0.3% and
// tells nobody which one has done the work.
func cpuTime(ms int64) string {
	switch {
	case ms <= 0:
		return "-"
	case ms < 1000:
		return fmt.Sprintf("%d ms", ms)
	case ms < 60000:
		return fmt.Sprintf("%.2f s", float64(ms)/1000)
	}
	return fmt.Sprintf("%dm %02ds", ms/60000, (ms%60000)/1000)
}

// provisioningScript shows what a node is sent before a run.
//
// In the console's own voice, monospaced and in order, because the point is
// that it is the same text somebody would type - a script they can copy into a
// terminal and watch fail there, rather than a description of what the
// application does somewhere they cannot see.
func provisioningScript(t *theme.Theme, s *state.Snapshot) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if s == nil || len(s.Provisioning) == 0 {
			return layout.Dimensions{}
		}
		kids := []layout.FlexChild{
			layout.Rigid(comp.SectionTitle(t, s.ProvisioningNode+" is told, at boot:")),
		}
		for _, l := range s.Provisioning {
			l := l
			col := t.P.Ink
			if l.Comment {
				col = t.P.Faint
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// A comment is already prose, so it takes the whole width
				// rather than being cut off by a column sized for commands.
				if l.Comment {
					return comp.OneLine(t, t.Sz.Data, col, l.Command+"  -  "+l.Why, false)(gtx)
				}
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(230)
						gtx.Constraints.Max.X = gtx.Dp(230)
						return comp.Mono(t, t.Sz.Data, col, l.Command)(gtx)
					}),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Dim, l.Why, false)),
				)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	}
}

// OpenFirmware and OpenMenu show a node's controls without a click.
//
// Scriptable for the same reason the search box and the pop-out are: a control
// that only opens under a hand cannot be captured, and a thing nobody can
// screenshot is a thing nobody checks. This is now the third time that has
// bitten, so it goes in with the control rather than after it.
func (p *nodeViewPanel) OpenFirmware(node string) { p.pick.open(node) }
