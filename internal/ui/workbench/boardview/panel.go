// The Bring-up window: one board, and whether it is behaving like the board it
// claims to be.
//
// The Hardware tab draws a board so somebody can recognise it and press its
// buttons, which is what an operator and an app developer need. This is for the
// third of them. A firmware developer already knows what the board is and
// cannot change it; the question is whether the thing in front of them matches
// its own profile, and if not, which line is lying.
//
// So every row carries a verdict, and the window is a window rather than a tab
// because the move it exists for is "the log said this, so what did the pin
// do" - which needs the log and the table visible at once.
package boardview

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Tab is which table the middle is showing.
type Tab int

const (
	// TabRadio is first because it is the one that answers today: every row on
	// it is something the firmware really left in the chip.
	TabRadio Tab = iota
	TabWiring
	numTabs
)

func (t Tab) String() string {
	if t == TabWiring {
		return "Wiring"
	}
	return "Radio"
}

// Panel is one node's bring-up window.
type Panel struct {
	Node string
	Tab  Tab

	// OnDo fires a verb. Panels never mutate state; the one thing this window
	// writes is a stimulus somebody asked for, and it goes the same way every
	// other control's does.
	OnDo func(verb string, params any)
	// OnPopScreen opens the panel on its own. Held as a callback because which
	// windows exist is the window set's business, not the panel's.
	OnPopScreen func(node string)

	// sel is the row the inspector describes, and picks is what makes it
	// movable: one clickable per row, pooled by the row's own name so a widget
	// keeps its identity as the table is re-derived every frame.
	//
	// Keyed rather than indexed. Widget identity is address here, and a slice
	// indexed by position hands row three's presses to whatever is third this
	// frame - which on a table that changes with the board is a press landing
	// on the wrong line.
	picks map[string]*widget.Clickable

	// scale is the whole-number magnification chosen for the panel, or zero
	// for whatever the rail's budget allows.
	scale     int
	steps     [maxScale]widget.Clickable
	popScreen comp.Button
	split     comp.Splitter
	// logSplit is the rule above the log strip, and logH how tall the strip is
	// in dp. Its own splitter rather than the rail's: two handles that moved
	// together would be one handle drawn twice.
	logSplit comp.Splitter
	logH     int
	// typed is the console's own input and send its button: a line to the
	// board, from the window that is watching it.
	typed comp.Field
	send  comp.Button
	// decode turns the framed protocol into the exchange it carries, and
	// framed is whether there is one to turn. Off by default: the wire is what
	// the board sent, and this window is about what the board did.
	decode comp.Check
	framed bool
	// auditing draws every control that a particular node would hide, so the
	// control audit can find them. See AuditDraw.
	auditing bool
	screen   ScreenView
	// pic is this window's own image of the panel. Not the view's: the popped
	// out screen window shares the view and must not share the image, because
	// the two are separate frame loops drawing at separate scales.
	pic comp.ScreenImage
	// board is the lamps, buttons and trackball, the same widgets the Hardware
	// tab draws. A board that can be looked at and not pressed is half a board:
	// the whole way to find out whether a button reaches the firmware is to
	// press it while watching what the pin did.
	parts comp.BoardControls
	// cardIn and wipeCard drive the slot, where the board has one.
	cardIn   comp.Check
	wipeCard comp.Button
	// reset restarts the board, the way its own reset button does; shot saves
	// what the panel is showing.
	reset comp.Button
	shot  comp.Button
	// OnSaveShot asks for a file and writes the panel to it. Held as a
	// callback because opening the platform's dialog is the shell's business,
	// not a panel's.
	OnSaveShot func(node, suggested string)

	tabs [numTabs]widget.Clickable
	// rows and index scroll, and show that they do. A rail squeezed by a panel
	// at 2:1 cuts its parts list off, and a cut with no scrollbar reads as a
	// board with fewer parts than it has.
	rows  widget.List
	index widget.List
	sel   int

	// The log strip: which voice is showing, what has been asked for, and its
	// own scroll.
	logTabs  [2]widget.Clickable
	logSrc   int
	logAsked string
	logList  widget.List

	// counts is what the last frame's table came to, for the status bar.
	counts Counts
}

// pick is this row's clickable, made on the first frame that draws the row.
//
// The table and the index down the rail are two views of one list, so they
// share one clickable per row: pressing a part in either puts the same row in
// the inspector, and the two cannot disagree about what is selected.
func (p *Panel) pick(key string) *widget.Clickable {
	if p.picks == nil {
		p.picks = map[string]*widget.Clickable{}
	}
	c, ok := p.picks[key]
	if !ok {
		c = &widget.Clickable{}
		p.picks[key] = c
	}
	return c
}

// rowKey is what a row is known by across frames: the tab it is on, its group
// and its name, which is what the eye uses too.
//
// The tab is in it because the two tables have a "Radio" group each, and a key
// that ignored which one is showing would hand a press on one tab's row to the
// other tab's widget the moment the two ever shared a name.
func (p *Panel) rowKey(r Row) string {
	return string(rune('0'+int(p.Tab))) + "/" + r.Group + "/" + r.Name
}

// Draw lays the window out: the board on the left, the tables in the middle,
// the selected row said in full on the right, and what the board printed along
// the bottom.
func (p *Panel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	// Named here rather than where it is drawn: a control whose label is set
	// during layout has no label until the first frame, and anything that
	// inspects the panel before then - the control audit, for one - finds an
	// unnamed button it cannot report on.
	p.popScreen.Kind, p.popScreen.Label = comp.Quiet, "pop out"
	p.reset.Kind, p.reset.Label = comp.Quiet, "reset"
	p.shot.Kind, p.shot.Label = comp.Quiet, "save a picture..."
	p.split.Vertical = true
	p.rows.Axis = layout.Vertical
	p.index.Axis = layout.Vertical

	st := p.statFor(s)
	b, ok := p.board(st)
	if !ok {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Faint,
			"this node is not running a board image, so there is no board to show"))
	}
	// A board whose parts nobody has recorded still has a radio, and the chip's
	// own registers reach here whichever emulator is running it. So the wiring
	// side goes quiet and the rest of the window works, rather than the window
	// refusing a board it has plenty to say about.
	if !hasPanel(b) {
		p.Tab = TabRadio
	}
	rows := p.rowsFor(b, st)
	p.counts = Counts{}
	for _, r := range rows {
		p.counts.add(r.Verdict)
	}
	if p.sel >= len(rows) {
		p.sel = 0
	}

	for i := range p.tabs {
		if p.tabs[i].Clicked(gtx) {
			p.Tab, p.sel = Tab(i), 0
		}
	}
	for i := range p.steps {
		if p.steps[i].Clicked(gtx) {
			p.scale = i + 1
		}
	}
	if p.popScreen.Click.Clicked(gtx) && p.OnPopScreen != nil {
		p.OnPopScreen(p.Node)
	}
	if p.reset.Click.Clicked(gtx) && p.OnDo != nil {
		p.OnDo("board.reset", map[string]any{"node": p.Node})
	}
	if p.shot.Click.Clicked(gtx) && p.OnSaveShot != nil && hasScreen(b) {
		p.OnSaveShot(p.Node, shotName(p.Node, b.Name))
	}
	// Pressing a control here reaches the firmware the same way the Hardware
	// tab's does, through the same verb: this window watches the board, and the
	// one thing it writes is a stimulus somebody asked for.
	if p.OnDo != nil {
		for _, pr := range p.parts.Presses(b.Hardware) {
			p.OnDo("board.press", map[string]any{
				"node": p.Node, "pin": pr.Pin, "down": pr.Down})
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(t, gtx, b, st)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				comp.Fixed(gtx, railWidth(b, p.scale), func(gtx layout.Context) layout.Dimensions {
					return p.rail(t, gtx, b, st, rows, s)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.dragRail(t, gtx, b)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.middle(t, gtx, rows)
				}),
				layout.Rigid(vRule(t)),
				comp.Fixed(gtx, 260, func(gtx layout.Context) layout.Dimensions {
					return p.inspector(t, gtx, rows)
				}),
			)
		}),
		// The rule above the log is also the handle that resizes it.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.dragLog(t, gtx)
		}),
		// The log under everything, at a height that shows a handful of lines
		// without taking the table's room: enough to see what just happened,
		// and the Output tab next door for the whole of it.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h := gtx.Dp(unit.Dp(p.logHeight()))
			gtx.Constraints.Max.Y, gtx.Constraints.Min.Y = h, h
			return p.logStrip(t, gtx, s)
		}),
		layout.Rigid(hRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.status(t, gtx, st)
		}),
	)
}

// dragRail is the rule between the board and the tables, which is also how the
// panel is made bigger without going through the steps.
//
// It reports movement and this decides what movement means: here it is the
// next whole scale up or down, because a panel is only honest at multiples and
// a rail that could stop between two of them would be a rail that lies.
func (p *Panel) dragRail(t *theme.Theme, gtx layout.Context, b hw.Board) layout.Dimensions {
	d := p.split.Layout(t, gtx)
	if p.split.Delta != 0 && b.Hardware != nil && b.Hardware.Screen != nil {
		sc := b.Hardware.Screen
		want := railFor(b, p.scale) + int(p.split.Delta)
		p.scale = fitScale(sc.WidthPx, sc.HeightPx, want, 1<<15)
	}
	return d
}

// rowsFor is the table the current tab shows.
func (p *Panel) rowsFor(b hw.Board, st *state.NodeStat) []Row {
	if p.Tab == TabWiring {
		return wiringRows(b, st)
	}
	if r := radioRows(b, st); len(r) > 0 {
		return r
	}
	return nil
}

// statFor and board are the lookups in screenpanel.go, which the popped-out
// window makes too. One of each, so the two windows cannot come to disagree
// about which board a node is.
func (p *Panel) statFor(s *state.Snapshot) *state.NodeStat { return statOf(s, p.Node) }

func (p *Panel) board(st *state.NodeStat) (hw.Board, bool) { return boardOf(st) }

// AuditDraw is Draw with nothing conditional hidden.
//
// The control audit sweeps a pointer over a panel and reports anything it
// cannot land on. Some controls here belong to one kind of node - the decode
// tick is only worth offering where there is a framed protocol to decode - and
// a sweep of a repeater would never find them, which reads as a control wired
// to nothing rather than one correctly absent.
func (p *Panel) AuditDraw(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {
	p.auditing = true
	return p.Draw(t, gtx, s)
}
