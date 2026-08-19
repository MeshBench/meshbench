// Serving the port instead of holding it, so a real client can have it.
//
// The transports already existed - engine.ServeCompanionTCP and
// ServeCompanionSerial, behind the bench.serve verb - but only from a panel
// that had no idea which node you were looking at. This is the same machinery
// where the node already is.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// tcpPane offers the port to something else, and says what has taken it.
func (c *companionTab) tcpPane(t *theme.Theme, gtx layout.Context, cs state.Companion) layout.Dimensions {
	ep := cs.Serving
	return layout.UniformInset(t.Sp.M).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink, "Serve this node to your own client")),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
				"The workbench hands the port over rather than holding it. While it is "+
					"served, Client and CLI are not reading - one claim, one holder.")),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return c.serveControls(t, gtx, ep)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return c.servedAddress(t, gtx, ep)
			}),
		)
	})
}

// serveControls picks the transport and starts or stops it.
func (c *companionTab) serveControls(t *theme.Theme, gtx layout.Context, ep state.Endpoint) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "transport  ")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.tcpChip.Layout(t, gtx, "TCP socket", "", c.serveKind != "serial", t.P.Accent)
		}),
		layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.ptyChip.Layout(t, gtx, "PTY device", "", c.serveKind == "serial", t.P.Accent)
		}),
		layout.Flexed(1, comp.Spacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ep.Addr == "" {
				return c.serveBtn.Layout(t, gtx)
			}
			return c.stopServeBtn.Layout(t, gtx)
		}),
	)
}

// servedAddress is the endpoint, who has it, and what to point at it.
func (c *companionTab) servedAddress(t *theme.Theme, gtx layout.Context, ep state.Endpoint) layout.Dimensions {
	if ep.Addr == "" {
		return comp.Text(t, t.Sz.Caption, t.P.Faint,
			"not served - nothing outside the workbench can reach this node")(gtx)
	}
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, kindWord(ep.Kind)+"  ")),
					layout.Rigid(comp.Mono(t, t.Sz.Body, t.P.Accent, ep.Addr)),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return borderedAction(t, gtx, c.click("copyaddr"), "copy", t.P.Rule, t.P.Dim)
					}),
					layout.Flexed(1, comp.Spacer),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if !ep.Attached {
							return layout.Dimensions{}
						}
						return c.dropBtn.Layout(t, gtx)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, attachedInk(t, ep), attachedWord(ep))),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "point a client at it")),
			layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, clientHint(ep))),
		)
	})(gtx)
}

func kindWord(kind string) string {
	if kind == "serial" {
		return "device"
	}
	return "address"
}

func attachedInk(t *theme.Theme, ep state.Endpoint) colorNRGBA {
	if ep.Attached {
		return t.P.Good
	}
	return t.P.Faint
}

func attachedWord(ep state.Endpoint) string {
	if ep.Attached {
		return "a client is attached"
	}
	return "waiting for a client"
}

// clientHint is the command that connects to what is being served.
func clientHint(ep state.Endpoint) string {
	if ep.Kind == "serial" {
		return "meshcore-cli -s " + ep.Addr
	}
	return fmt.Sprintf("meshcore-cli -t %s   ·   or a phone app over TCP", ep.Addr)
}
