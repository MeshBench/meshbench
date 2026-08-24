// What a node actually printed, from each of the three things that can say
// something about it.
//
// The Console pane is a conversation - typed at, answered. When a board
// black-screens there is no conversation to have and no reply to read, and the
// question becomes which of three voices went quiet: the board's own serial
// port, the emulator running it, or the radio model beside it. All three were
// files on disk nobody could reach without leaving the application, which is
// exactly where somebody is standing when the interface says a board is
// running and the board is doing nothing.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// outputSources are the three voices, in the order somebody asks for them, and
// what each one is - because "emulator" and "radio" are not obvious until you
// have needed the difference once.
var outputSources = []struct{ key, label, what string }{
	{"serial", "serial", "what the board itself printed"},
	{"emulator", "emulator", "what the emulator said about running it"},
	{"radio", "radio", "what the radio model beside it logged"},
}

// outputPane is the Output tab's own state.
//
// Its own type in its own file: it holds five widgets and a chosen source, and
// hanging those off the window struct is how that struct became long enough
// that nobody could see what belonged to which pane.
type outputPane struct {
	source   string
	list     widget.List
	search   comp.Field
	pauseBtn comp.Button
	// A slice rather than an array so the audit walks it: it descends slices
	// of buttons and not arrays, and a control it cannot see is a control
	// nothing checks is wired.
	srcBtns []comp.Button
	// follow keeps the newest line on screen. A board that is failing prints
	// while it is being read, and the last line is the one that matters -
	// but so is the ability to stop and read one, which is what pausing is.
	follow bool
	built  bool
	// asked is the node and source last requested, so the verb is called when
	// either changes rather than every frame.
	asked string
}

func (o *outputPane) build() {
	o.source, o.follow = "serial", true
	o.srcBtns = make([]comp.Button, len(outputSources))
	o.search.Hint = "search this output"
	o.search.Editor.SingleLine = true
	o.pauseBtn.Label, o.pauseBtn.Kind = "pause", comp.Secondary
	o.list.Axis = layout.Vertical
	o.built = true
}

// output draws the tab.
func (p *nodeWindowPanel) output(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	o := &p.out
	if !o.built {
		o.build()
	}
	p.askOutput(o.source)

	lines, total, note, path := o.readFrom(p.node, s)
	shown := filterLines(lines, fieldText(&o.search))

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return o.head(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return o.body(t, gtx, shown, note)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return o.foot(t, gtx, len(shown), total, path)
		}),
	)
}

// readFrom is the snapshot's output, but only when it is this node's and this
// source's.
//
// The snapshot carries one node's output at a time, the way it carries one
// node's console. Showing another node's under this node's heading is worse
// than showing nothing: it is a wrong answer to the question the pane is for.
func (o *outputPane) readFrom(node string, s *state.Snapshot) (lines []string, total int, note, path string) {
	if s == nil || s.OutputNode != node || s.OutputSource != o.source {
		return nil, 0, "", ""
	}
	return s.Output, s.OutputTotal, s.OutputNote, s.OutputPath
}

func (o *outputPane) head(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	kids := []layout.FlexChild{}
	for i := range outputSources {
		src := outputSources[i]
		b := &o.srcBtns[i]
		b.Label = src.label
		b.Kind = comp.Quiet
		if o.source == src.key {
			b.Kind = comp.Primary
		}
		kids = append(kids,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return b.Layout(t, gtx)
			}),
			layout.Rigid(layout.Spacer{Width: t.Sp.XXS}.Layout),
		)
	}
	kids = append(kids,
		layout.Flexed(1, comp.Spacer),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(200)
			return o.search.Layout(t, gtx)
		}),
	)
	return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
	})
}

func (o *outputPane) body(t *theme.Theme, gtx layout.Context, shown []string, note string) layout.Dimensions {
	if len(shown) == 0 {
		// A reason where there is one, and never a blank pane: an empty pane
		// reads as a component that has failed, and most of the time it means
		// the node has not run yet or this source does not exist for it.
		said := note
		if said == "" {
			said = "nothing on this source yet"
		}
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint, said))
	}
	o.list.ScrollToEnd = o.follow
	return comp.List(t, &o.list, len(shown), func(gtx layout.Context, i int) layout.Dimensions {
		return comp.Mono(t, t.Sz.Caption, t.P.Ink, shown[i])(gtx)
	})(gtx)
}

func (o *outputPane) foot(t *theme.Theme, gtx layout.Context, shown, total int, path string) layout.Dimensions {
	label := fmt.Sprintf("showing %d of %d lines", shown, total)
	if total >= outputPaneCap {
		label += " - the oldest have scrolled off; the file on disk still has all of them"
	}
	if path != "" {
		label += "  ·  " + path
	}
	return layout.Inset{Top: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint, label, false)),
			layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return o.pauseBtn.Layout(t, gtx)
			}),
		)
	})
}

// outputPaneCap mirrors the session's own cap on how many lines cross the
// snapshot. Off by a little costs nothing: it only decides when the footer
// admits that the pane is not the whole file.
const outputPaneCap = 2000

// askOutput points the session's output pane at this node and source.
//
// Asked for on a change rather than every frame: the tick refreshes whatever
// is already being shown, and this is only what tells it what that is. Two
// tabs ask - the Output pane for the source somebody chose, and the Hardware
// tab for the serial strip under the board - because a window opened straight
// onto Hardware has never asked at all, and its strip would sit empty saying
// the board had printed nothing.
func (p *nodeWindowPanel) askOutput(source string) {
	if p.OnDo == nil {
		return
	}
	want := p.node + "/" + source
	if want == p.out.asked {
		return
	}
	p.out.asked = want
	p.OnDo("node.output", map[string]any{"node": p.node, "source": source})
}

// outputClicks handles the pane's controls.
//
// Called from the window's clicks() whatever tab is showing, so a control
// cannot be live in the normal draw and dead in the audit's flat one.
func (p *nodeWindowPanel) outputClicks(gtx layout.Context) {
	o := &p.out
	if !o.built {
		o.build()
	}
	if o.pauseBtn.Click.Clicked(gtx) {
		o.follow = !o.follow
	}
	o.pauseBtn.Label = "pause"
	if !o.follow {
		o.pauseBtn.Label = "follow"
	}
	for i := range o.srcBtns {
		if !o.srcBtns[i].Click.Clicked(gtx) {
			continue
		}
		o.source = outputSources[i].key
		// Re-read even when it is the source already showing. The pane is
		// refreshed by the tick while the engine is running, and a stopped
		// node's file is not - so pressing the source you are on is how
		// somebody asks again, and doing nothing there reads as a dead
		// button on the one board that most needs looking at.
		o.asked = ""
		// Chasing the end of a file somebody has just switched to is what
		// they want: the newest line is the reason they switched.
		o.follow = true
	}
}

// outputSummary is the last few lines of a node's serial output, for the
// Hardware tab's strip.
//
// The strip exists because that is where somebody is standing when a board
// draws nothing: looking at a picture of the board. Sending them to another
// tab to find out why is the trip this saves.
func outputSummary(node string, s *state.Snapshot, n int) []string {
	if s == nil || s.OutputNode != node || s.OutputSource != "serial" {
		return nil
	}
	lines := s.Output
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// trimLine keeps a strip's line to a width that will not push the board's
// picture out of the window.
func trimLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimRight(s[:n], " ") + "…"
}
