// The node view: what every node is costing and doing (P6).
//
// The question this answers is the one somebody asks when a hundred and fifty
// processes are running and the machine has started to labour: which node, and
// what is it doing. So the columns are cost first, traffic second, and the
// chip's own counters last - because those only matter once you already suspect
// one node of misbehaving.
package workbench

import (
	"fmt"
	"image"
	"sync/atomic"

	"gioui.org/f32"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
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
	// pickFilter narrows the build list, because there are dozens.
	pickFilter  comp.Field
	shownBuilds []int
	// OnFirmware asks the store to put a build on a node.
	OnFirmware func(node, version string)

	// OnAction asks the store to do something to the selected node.
	OnAction func(action, node string)
}

func (p *nodeViewPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	// The firmware list is drawn over the panel, not inside its flex.
	//
	// It was a rigid child after a flexed table, so the table took the space
	// and the list was laid out at zero height: open, invisible, and
	// impossible to click. Choosing a build was unreachable by pointing at
	// anything, which is the only way a person would try.
	if p.pickFor != "" {
		defer func() {
			macro := op.Record(gtx.Ops)
			p.firmwareOverlay(t, gtx)
			macro.Stop().Add(gtx.Ops)
		}()
	}
	p.watched.Store(true)

	if !p.init {
		p.tb.Cols = nodeColumns()
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
			Key:   n.Name,
			Cells: nodeCells(n, st),
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
		layout.Rigid(p.contextMenu(t)),
		layout.Rigid(provisioningScript(t, s)),
		layout.Rigid(resolvedProvisioningScript(t, s)),
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

func (p *nodeViewPanel) OpenMenu(node string, s *state.Snapshot) {
	p.menuFor = node
	p.menuItems = nodeMenuFor(node, s)
}

// nodeColumns and nodeCells are declared next to each other, and a test holds
// them side by side. A row one cell short does not fail - it shifts, and every
// value lands under the heading to its left, which is how memory read as a
// dash while receptions were counted as transmissions.
func nodeColumns() []comp.Column {
	return []comp.Column{
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
}

func nodeCells(n state.NodeStat, st string) []string {
	return []string{
		n.Name, st, orDash(n.Backend), orDash(n.Firmware),
		siBytes(n.RSSBytes),
		cpuTime(n.CPUms), fmt.Sprintf("%.1f%%", n.CPUPct),
		fmt.Sprintf("%d", n.Sent), fmt.Sprintf("%d", n.Heard),
		lastPacket(n.LastSentMs, n.LastSentTo),
		lastPacket(n.LastHeardMs, n.LastHeardFrom),
		busyPct(n), fmt.Sprintf("%d", n.Spurious),
	}
}
