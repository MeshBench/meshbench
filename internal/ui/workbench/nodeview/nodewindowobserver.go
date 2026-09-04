// The SDR observer's own window.
//
// An observer runs no firmware: there is no console to type into, no chip to
// read back, nothing to start. What it has instead is the one thing no other
// node offers - its antenna, served live to real SDR software - so its window
// leads with that and drops the tabs that would always be empty.
package nodeview

import (
	"fmt"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// isObserver decides whether this window is an observer's - a different tab
// set, a different head, and the SDR pane in place of a console.
func (p *WindowPanel) isObserver() bool {
	return strings.Contains(strings.ToLower(p.Kind), "observer")
}

// sourceFor is this node's serving entry, or nil while it is idle.
func (p *WindowPanel) sourceFor(s *state.Snapshot) *state.SDRSource {
	if s == nil {
		return nil
	}
	for i := range s.SDRSources {
		if s.SDRSources[i].Node == p.Node {
			return &s.SDRSources[i]
		}
	}
	return nil
}

// observerStatus is the head's answer for an observer: not running/stopped,
// which belong to firmware, but whether the antenna is on the wire.
func (p *WindowPanel) observerStatus(t *theme.Theme, s *state.Snapshot) layout.Widget {
	col, what := t.P.Faint, "idle"
	if src := p.sourceFor(s); src != nil {
		col, what = t.P.Good, "serving "+src.Addr
	}
	return comp.Text(t, t.Sz.Caption, col, what)
}

// observerServeButton is the head's one action: put the antenna on the wire,
// or take it off.
func (p *WindowPanel) observerServeButton(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {
	if p.sourceFor(s) != nil {
		p.sdrStop.Label, p.sdrStop.Kind = "stop serving", comp.Destructive
		return p.sdrStop.Layout(t, gtx)
	}
	p.sdrServe.Label, p.sdrServe.Kind = "serve rtl_tcp", comp.Primary
	return p.sdrServe.Layout(t, gtx)
}

// sdrTab is the observer window's front pane: the address to point a client
// at, and what the stream honestly is.
func (p *WindowPanel) sdrTab(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {
	src := p.sourceFor(s)
	gap := layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout)
	kids := []layout.FlexChild{
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"serve this observer's antenna as an rtl_tcp source - SDR++'s stock "+
				"client connects to it as if it were a dongle. The IQ is rendered "+
				"from the same synthesis the verdicts are judged from, never from "+
				"packet events: a collision is on the stream because the summed "+
				"air contains it.")),
		gap,
	}
	if src == nil {
		kids = append(kids,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.observerServeButton(t, gtx, s)
			}),
			gap,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"not served yet - the address appears here once it is")),
		)
	} else {
		kids = append(kids,
			layout.Rigid(comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
				attached, col := "waiting for a client", t.P.Dim
				if src.Attached {
					attached, col = "client connected", t.P.Good
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(comp.Mono(t, t.Sz.Section, t.P.Ink, src.Addr)),
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
						"any client sample rate works - the stream follows the "+
							"client's own rate menu (native %.0f Hz)", src.RateHz))),
					layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
					layout.Rigid(comp.Text(t, t.Sz.Caption, col, attached)),
				)
			})),
			gap,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.observerServeButton(t, gtx, s)
			}),
		)
	}
	kids = append(kids,
		gap,
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"one client at a time, exactly as the real rtl_tcp behaves - a second "+
				"connection is refused rather than fed interleaved samples")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"drag the observer on the map while a client listens: the next window "+
				"prices the new geometry")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"pausing the run holds the stream at the pause point - frozen time "+
				"cannot honestly produce signal")),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}
