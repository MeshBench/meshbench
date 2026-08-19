// The Feed panel: recent traffic on the real network.
//
// Beside the Validate panel because a feed and a validation are the same data
// asked two questions - what is happening, and whether we would have predicted
// it.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// feedPanel is recent traffic on the real network.
type feedPanel struct {
	tb   comp.Table
	init bool
	// OnPull asks the store to fetch recent receptions.
	OnPull func()
}

func (p *feedPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "when", Width: 160, Mono: true, Sortable: true},
			{Title: "heard by", Width: 200, Sortable: true},
			{Title: "from", Width: 200, Sortable: true},
			{Title: "hops", Width: 66, Right: true, Mono: true},
			{Title: "SNR", Width: 80, Right: true, Mono: true, Sortable: true},
			{Title: "packet", Mono: true},
		}
		p.tb.SortCol, p.tb.SortDesc = 0, true
		p.init = true
	}
	body := func(gtx layout.Context) layout.Dimensions {
		if s == nil || len(s.Observed) == 0 {
			return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
				"nothing pulled yet - this is the real network, not the simulated one"))
		}
		rows := make([]comp.Row, 0, len(s.Observed))
		for i, o := range s.Observed {
			snr := "-"
			if o.HasSNR {
				snr = fmt.Sprintf("%.1f", o.SNRdB)
			}
			rows = append(rows, comp.Row{
				Key: fmt.Sprintf("%d/%s", i, o.PacketID),
				Cells: []string{
					o.At.Format("2006-01-02 15:04:05"),
					o.Receiver, o.Origin, fmt.Sprintf("%d", o.HopCount),
					snr, o.PacketID,
				},
			})
		}
		p.tb.SetRows(rows)
		return p.tb.Layout(t, gtx, nil)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "the real network")),
		layout.Flexed(1, body),
	)
}
