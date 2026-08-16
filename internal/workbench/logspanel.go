// The Logs panel: every status line this session has said, scrollable, and
// a way to hand a copy of it to somebody who was not watching when it
// mattered.
//
// Say already went to a real file on disk the moment the run started - this
// is a window onto the same words, not a second source of them.
package workbench

import (
	"fmt"
	"strings"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

type logsPanel struct {
	list      widget.List
	search    comp.Field
	pauseBtn  comp.Button
	exportBtn comp.Button
	// follow keeps the newest line on screen; pausing stops the chase so a
	// line can be read while the run keeps talking.
	follow bool
	do     Do
	built  bool
	// exported carries a browse answer back from the goroutine the dialog
	// blocks on, to be read at the top of the next frame.
	exported atomic.Value
}

func (p *logsPanel) build() {
	p.follow = true
	p.search.Hint = "search the log"
	p.search.Editor.SingleLine = true
	p.pauseBtn.Label, p.pauseBtn.Kind = "pause", comp.Secondary
	p.exportBtn.Label, p.exportBtn.Kind = "export...", comp.Secondary
	p.list.Axis = layout.Vertical
	p.built = true
}

func (p *logsPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.build()
	}
	if s == nil {
		return layout.Dimensions{}
	}
	if p.pauseBtn.Click.Clicked(gtx) {
		p.follow = !p.follow
	}
	p.pauseBtn.Label = "pause"
	if !p.follow {
		p.pauseBtn.Label = "follow"
	}
	if p.exportBtn.Click.Clicked(gtx) && shell.Browse != nil && p.do != nil {
		go func() {
			got, err := shell.Browse("Export the log to...", "meshbench.log",
				shell.PathAsk{Kind: shell.PathSaveFile,
					FilterName: "Log files", Extensions: []string{"log", "txt"}})
			p.exported.Store(&pickResult{path: got, err: err})
		}()
	}
	if r, _ := p.exported.Swap((*pickResult)(nil)).(*pickResult); r != nil {
		switch {
		case r.err != nil:
			p.do("ui.said", "could not open a file dialog: "+r.err.Error())
		case r.path != "":
			p.do("logs.export", r.path)
		}
	}

	want := fieldText(&p.search)
	shown := filterLines(s.FullLog, want)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.headerRow(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.body(t, gtx, shown)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.footer(t, gtx, len(shown), len(s.FullLog))
		}),
	)
}

func (p *logsPanel) headerRow(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(240)
				return p.search.Layout(t, gtx)
			}),
			layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.exportBtn.Layout(t, gtx)
			}),
		)
	})
}

func (p *logsPanel) body(t *theme.Theme, gtx layout.Context, shown []string) layout.Dimensions {
	if len(shown) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"nothing said yet"))
	}
	p.list.ScrollToEnd = p.follow
	return comp.List(t, &p.list, len(shown), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx,
			comp.Mono(t, t.Sz.Caption, t.P.Ink, shown[i]))
	})(gtx)
}

func (p *logsPanel) footer(t *theme.Theme, gtx layout.Context, shown, total int) layout.Dimensions {
	label := fmt.Sprintf("showing %d of %d lines", shown, total)
	if total >= maxFullLogHint {
		label += " - the oldest have scrolled off; the file on disk still has all of them"
	}
	return layout.Inset{Top: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, label)),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.pauseBtn.Layout(t, gtx)
			}),
		)
	})
}

// maxFullLogHint mirrors state.maxFullLog: the panel does not import state's
// unexported constant, so this is what "probably truncated" means here. Off
// by a little costs nothing - it only changes when the footer's caveat shows.
const maxFullLogHint = 5000

// filterLines is the log, narrowed to lines containing the search text.
func filterLines(lines []string, want string) []string {
	if want == "" {
		return lines
	}
	want = strings.ToLower(want)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), want) {
			out = append(out, l)
		}
	}
	return out
}
