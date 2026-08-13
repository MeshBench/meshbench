// The node view: what every node is costing and doing (P6).
//
// The question this answers is the one somebody asks when a hundred and fifty
// processes are running and the machine has started to labour: which node, and
// what is it doing. So the columns are cost first, traffic second, and the
// chip's own counters last - because those only matter once you already suspect
// one node of misbehaving.
package main

import (
	"fmt"
	"image"
	"sync/atomic"

	"gioui.org/f32"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

type nodeViewPanel struct {
	// watched is set every time this panel draws, and cleared by whoever
	// refreshes it. A live memory and processor view that only updates when a
	// button is pressed is not a live view; sampling /proc for every node when
	// nobody has the panel open is waste. This is how it does neither.
	watched atomic.Bool

	tb   comp.Table
	init bool

	running, native, emulated, stopped, busy comp.Check
	stop, start, refresh                     comp.Button

	// The firmware picker: every installed build, offered from the firmware
	// cell. A row of buttons was the first attempt and the wrong shape - nine
	// builds overflowed the panel, and the number of builds is however many
	// somebody has installed.
	builds     []string
	buildBtns  []comp.Button
	buildsRead bool
	buildList  widget.List
	// pickFor is the node whose firmware list is open, or "".
	pickFor string
	// menuFor is the node whose context menu is open, and where it opened.
	menuFor   string
	menuAt    image.Point
	menuItems []comp.MenuItem
	closePick comp.Button
	// OnFirmware asks the store to put a build on a node.
	OnFirmware func(node, version string)

	// OnAction asks the store to do something to the selected node.
	OnAction func(action, node string)
}

func (p *nodeViewPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	p.watched.Store(true)

	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "node", Width: 190, Sortable: true},
			{Title: "state", Width: 86, Sortable: true},
			{Title: "backend", Width: 90, Sortable: true},
			{Title: "firmware", Width: 200, Mono: true, Sortable: true, Menu: true},
			{Title: "memory", Width: 96, Right: true, Mono: true, Sortable: true},
			{Title: "cpu time", Width: 88, Right: true, Mono: true, Sortable: true},
			{Title: "cpu now", Width: 78, Right: true, Mono: true, Sortable: true},
			{Title: "tx", Width: 60, Right: true, Mono: true, Sortable: true},
			{Title: "rx", Width: 60, Right: true, Mono: true, Sortable: true},
			{Title: "last sent", Width: 150, Sortable: true},
			{Title: "last heard", Width: 150, Sortable: true},
			{Title: "busy", Width: 74, Right: true, Mono: true, Sortable: true},
			{Title: "spurious", Right: true, Mono: true, Sortable: true},
		}
		p.tb.SortCol, p.tb.SortDesc = 4, true
		p.running.Label, p.running.Bool.Value = "running", true
		p.stopped.Label, p.stopped.Bool.Value = "stopped", true
		p.native.Label, p.native.Bool.Value = "native", true
		p.emulated.Label, p.emulated.Bool.Value = "emulated", true
		p.busy.Label = "only chatty"
		p.stop.Label, p.stop.Kind = "stop this node", comp.Destructive
		p.start.Label, p.start.Kind = "start it", comp.Primary
		p.refresh.Label = "refresh"
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	for _, b := range []struct {
		btn    *comp.Button
		action string
	}{{&p.stop, "node.stop"}, {&p.start, "node.start"}, {&p.refresh, "nodes.stats"}} {
		if b.btn.Click.Clicked(gtx) && p.OnAction != nil {
			p.OnAction(b.action, selectedNode(s))
		}
	}
	for _, c := range []*comp.Check{&p.running, &p.stopped, &p.native, &p.emulated, &p.busy} {
		c.Bool.Update(gtx)
	}

	if !p.buildsRead {
		p.buildsRead = true
		p.builds = installedBuilds()
		p.buildBtns = make([]comp.Button, len(p.builds))
		for i := range p.buildBtns {
			p.buildBtns[i].Label = p.builds[i]
			p.buildBtns[i].Kind = comp.Secondary
		}
	}
	for i := range p.buildBtns {
		if p.buildBtns[i].Click.Clicked(gtx) && p.OnFirmware != nil && selectedNode(s) != "" {
			p.OnFirmware(selectedNode(s), p.builds[i])
		}
	}

	p.tb.OnRightClick = func(key string, at f32.Point) {
		p.menuFor = key
		p.menuAt = image.Pt(int(at.X), int(at.Y))
		p.menuItems = nodeMenuFor(key, s)
	}
	p.tb.OnCell = func(key string, col int) {
		if col == 3 {
			p.pickFor = key
		}
	}

	rows := make([]comp.Row, 0, len(s.Stats))
	var totalRSS int64
	var totalCPU float64
	var totalMs int64
	shownRSS, shownCPU, shownMs := int64(0), 0.0, int64(0)
	for _, n := range s.Stats {
		totalRSS += n.RSSBytes
		totalCPU += n.CPUPct
		totalMs += n.CPUms
		if !p.keep(n) {
			continue
		}
		shownRSS += n.RSSBytes
		shownCPU += n.CPUPct
		shownMs += n.CPUms
		st := n.State
		if st == "" {
			st = "stopped"
			if n.Running {
				st = "running"
			}
		}
		rows = append(rows, comp.Row{
			Key: n.Name,
			Cells: []string{
				n.Name, st, orDash(n.Backend), orDash(n.Firmware),
				cpuTime(n.CPUms), fmt.Sprintf("%.1f%%", n.CPUPct),
				fmt.Sprintf("%d", n.Sent), fmt.Sprintf("%d", n.Heard),
				lastPacket(n.LastSentMs, n.LastSentTo),
				lastPacket(n.LastHeardMs, n.LastHeardFrom),
				busyPct(n), fmt.Sprintf("%d", n.Spurious),
			},
		})
	}
	p.tb.SetRows(rows)
	// After SetRows, because the search box filters inside the table: counting
	// the rows handed over misses it entirely, which is how a total ended up
	// saying 58 nodes above a list of 17.
	shown := p.tb.ShownKeys()

	// The total is of everything, and of what is shown when a filter is on.
	// A total that silently means "the rows you can see" is how somebody
	// concludes the machine is using a third of what it is.
	// Recount from what the table is actually showing, which is the filters
	// and the search box together.
	shownRSS, shownCPU, shownMs = 0, 0, 0
	for _, n := range s.Stats {
		if shown[n.Name] {
			shownRSS += n.RSSBytes
			shownCPU += n.CPUPct
			shownMs += n.CPUms
		}
	}
	total := fmt.Sprintf("%d nodes, %s, %s of processor time, %.0f%% of one core now",
		len(s.Stats), siBytes(totalRSS), cpuTime(totalMs), totalCPU)
	if len(shown) != len(s.Stats) {
		total = fmt.Sprintf("showing %d of %d - %s of %s, %s of %s",
			len(shown), len(s.Stats), siBytes(shownRSS), siBytes(totalRSS),
			cpuTime(shownMs), cpuTime(totalMs))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				chk(t, &p.running), chk(t, &p.stopped), chk(t, &p.native),
				chk(t, &p.emulated), chk(t, &p.busy),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
		layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim, total)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				btn(t, &p.stop), btn(t, &p.start), btn(t, &p.refresh),
			)
		}),
		layout.Rigid(p.firmwareList(t)),
		layout.Rigid(p.contextMenu(t)),
		layout.Rigid(provisioningScript(t, s)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// The graphs give up their space while a menu is open.
			//
			// Both reserve height, and a flex that cannot fit its rigid
			// children draws them over each other - which put the menu's
			// entries on top of the graph titles. One or the other, and the
			// menu wins because it was just asked for.
			if p.menuFor != "" || p.pickFor != "" {
				return layout.Dimensions{}
			}
			gtx.Constraints.Min.Y = gtx.Dp(96)
			gtx.Constraints.Max.Y = gtx.Dp(96)
			return nodeGraphs(t, gtx, s)
		}),
	)
}

// keep applies the filters. The search box is the table's own.
func (p *nodeViewPanel) keep(n state.NodeStat) bool {
	if n.Running && !p.running.Bool.Value {
		return false
	}
	if !n.Running && !p.stopped.Bool.Value {
		return false
	}
	if n.Backend == "native" && !p.native.Bool.Value {
		return false
	}
	if n.Backend == "emulated" && !p.emulated.Bool.Value {
		return false
	}
	if p.busy.Bool.Value && n.Sent == 0 && n.Heard == 0 {
		return false
	}
	return true
}

func chk(t *theme.Theme, c *comp.Check) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = 0
		return layout.Inset{Right: t.Sp.M}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return c.Layout(t, gtx) })
	})
}

func btn(t *theme.Theme, b *comp.Button) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = 0
		return layout.Inset{Right: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions { return b.Layout(t, gtx) })
	})
}

// siBytes prints bytes with an SI prefix, as asked: 1 kB is 1,000 bytes.
//
// SI rather than binary because the number sits beside a percentage and a
// packet count, and somebody comparing it to what `ps` or a system monitor
// says should get the same answer.
func siBytes(b int64) string {
	if b <= 0 {
		return "-"
	}
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTP"[exp])
}

// lastPacket is when and with whom, or nothing at all.
//
// Nothing rather than "0.000 s", which reads as a packet at the start of the
// run rather than as no packet.
func lastPacket(atMs uint32, who string) string {
	if atMs == 0 && who == "" {
		return "-"
	}
	return fmt.Sprintf("%.2fs %s", float64(atMs)/1000, who)
}

// busyPct is what share of the chip's interrupt reads found the air busy.
func busyPct(n state.NodeStat) string {
	if n.IRQReads == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n.BusyReads)/float64(n.IRQReads))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

var _ = widget.Bool{}

// firmwarePicker offers every installed build for the selected node.
//
// It says which node it would change, because a row of version buttons with
// nothing naming the target is a control somebody presses and then has to go
// and check.
// firmwareList is the open dropdown for one node's firmware cell.
//
// A list rather than a row of buttons, because the number of installed builds
// is whatever somebody has installed - nine already overflowed a row, and a
// control that works until you install a tenth is not a control.
func (p *nodeViewPanel) firmwareList(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.pickFor == "" {
			return comp.Text(t, t.Sz.Caption, t.P.Faint,
				"click a firmware cell to change what that node runs")(gtx)
		}
		if p.closePick.Label == "" {
			p.closePick.Label, p.closePick.Kind = "cancel", comp.Quiet
		}
		if p.closePick.Click.Clicked(gtx) {
			p.pickFor = ""
			return layout.Dimensions{}
		}
		for i := range p.buildBtns {
			if p.buildBtns[i].Click.Clicked(gtx) && p.OnFirmware != nil {
				p.OnFirmware(p.pickFor, p.builds[i])
				p.pickFor = ""
				return layout.Dimensions{}
			}
		}
		p.buildList.Axis = layout.Horizontal
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Ink, p.pickFor+" runs:  ")),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return comp.List(t, &p.buildList, len(p.buildBtns),
					func(gtx layout.Context, i int) layout.Dimensions {
						gtx.Constraints.Min.X = 0
						return layout.Inset{Right: t.Sp.S}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return p.buildBtns[i].Layout(t, gtx)
							})
					})(gtx)
			}),
			btn(t, &p.closePick),
		)
	}
}

func (p *nodeViewPanel) firmwarePicker(t *theme.Theme, selected string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		who := selected
		if who == "" {
			return comp.Text(t, t.Sz.Caption, t.P.Faint,
				"select a node to change its firmware")(gtx)
		}
		if len(p.builds) == 0 {
			return comp.Text(t, t.Sz.Caption, t.P.Warn,
				"no firmware installed - meshbench firmware install")(gtx)
		}
		// A scrolling row rather than a plain flex.
		//
		// Nine builds already overflowed the panel and wrapped the last button
		// down the right edge, and the number of builds is whatever somebody
		// has installed - so it has to hold any number rather than as many as
		// happened to fit when it was written.
		p.buildList.Axis = layout.Horizontal
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, "run on "+who+":  ")),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return comp.List(t, &p.buildList, len(p.buildBtns),
					func(gtx layout.Context, i int) layout.Dimensions {
						gtx.Constraints.Min.X = 0
						return layout.Inset{Right: t.Sp.S}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return p.buildBtns[i].Layout(t, gtx)
							})
					})(gtx)
			}),
		)
	}
}

// installedBuilds is the native builds this machine has, newest name last.
func installedBuilds() []string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range firmware.ListInstalled(filepath.Join(cache, "meshcoresim", "firmware")) {
		if f.Native {
			out = append(out, f.Version)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
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

// SetFilter presets the search box.
//
// Scriptable for the same reason the pop-out and the menu are: a control that
// only works under a hand cannot be captured, and a thing nobody can screenshot
// is a thing nobody checks.
func (p *nodeViewPanel) SetFilter(text string) {
	p.tb.Filter.SetText(text)
}

// selectedNode is the world's selection, which is what a click on the map or a
// script sets. The table has a selection of its own; having two was how the
// firmware picker came to say "select a node" with one plainly selected.
func selectedNode(s *state.Snapshot) string {
	if s == nil {
		return ""
	}
	for i := range s.Nodes {
		if s.Nodes[i].Selected {
			return s.Nodes[i].Name
		}
	}
	return ""
}

// nodeMenuFor is what can be done to one node.
//
// Built per node rather than once, because an entry that cannot apply is worse
// than absent: "start" on a running node either does nothing or restarts it,
// and neither is what the word says.
func nodeMenuFor(name string, s *state.Snapshot) []comp.MenuItem {
	running := false
	for _, n := range s.Stats {
		if n.Name == name {
			running = n.Running
		}
	}
	items := []comp.MenuItem{{Label: "Select on the map", Action: "nodes.select_many"}}
	if running {
		items = append(items, comp.MenuItem{Label: "Stop this node", Action: "node.stop"})
	} else {
		items = append(items, comp.MenuItem{Label: "Start this node", Action: "node.start"})
	}
	return append(items,
		comp.MenuItem{Label: "Change its firmware", Action: "ui.firmware"},
		comp.MenuItem{Label: "Show what it is told at boot", Action: "node.provisioning"},
		comp.MenuItem{Label: "Coverage from here", Action: "coverage.compute"},
		comp.MenuItem{Label: "Originate a packet here", Action: "sim.inject"},
		comp.MenuItem{Label: "Capture the waterfall here", Action: "waterfall.capture"},
	)
}

// contextMenu draws the open row menu.
func (p *nodeViewPanel) contextMenu(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if p.menuFor == "" {
			return layout.Dimensions{}
		}
		chosen := comp.MenuRow(t, gtx, p.menuItems, p.menuFor)
		if chosen == "" {
			return layout.Dimensions{}
		}
		who := p.menuFor
		p.menuFor = ""
		if chosen == "ui.firmware" {
			// The one entry the interface handles itself: it opens a control
			// rather than asking the store to do something.
			p.pickFor = who
			return layout.Dimensions{}
		}
		if p.OnAction != nil {
			p.OnAction(chosen, who)
		}
		return layout.Dimensions{}
	}
}

// OpenFirmware and OpenMenu show a node's controls without a click.
//
// Scriptable for the same reason the search box and the pop-out are: a control
// that only opens under a hand cannot be captured, and a thing nobody can
// screenshot is a thing nobody checks. This is now the third time that has
// bitten, so it goes in with the control rather than after it.
func (p *nodeViewPanel) OpenFirmware(node string) { p.pickFor = node }

func (p *nodeViewPanel) OpenMenu(node string, s *state.Snapshot) {
	p.menuFor = node
	p.menuItems = nodeMenuFor(node, s)
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
