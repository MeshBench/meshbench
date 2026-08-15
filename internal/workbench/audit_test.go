package workbench

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"gioui.org/f32"
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// Every control, pressed.
//
// The tests beside this one press the controls somebody thought to name. This
// one finds them: it walks each panel's own struct for buttons and checkboxes,
// presses every one, and asks two questions that are not the same question.
//
//   - Wired: pressing it reaches a verb, or changes something. A control that
//     reaches nothing is a control that draws.
//   - Reachable: a pointer moved across the panel actually lands on it. The
//     firmware picker was wired perfectly and had zero height, which no amount
//     of pressing its handler would have found.
//
// A control can pass one and fail the other, and the two failures need
// different fixes, so they are reported apart.

// control is one widget found by walking a panel.
type control struct {
	name string
	btn  *comp.Button
	chk  *comp.Check
	fld  *comp.Field
}

// controlsOf walks a struct for the widgets it owns, including through
// embedded structs like actionBar.
//
// Through unsafe, because the widgets are unexported and the whole point is to
// find the ones nobody remembered to name: a list written by hand is a list of
// the controls somebody was already thinking about.
func controlsOf(v reflect.Value, prefix string, out *[]control) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	btnT := reflect.TypeOf(comp.Button{})
	chkT := reflect.TypeOf(comp.Check{})
	fldT := reflect.TypeOf(comp.Field{})
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		name := prefix + v.Type().Field(i).Name
		switch {
		case f.Type() == btnT && f.CanAddr():
			*out = append(*out, control{
				name: name,
				btn:  (*comp.Button)(unsafe.Pointer(f.UnsafeAddr())),
			})
		case f.Type() == chkT && f.CanAddr():
			*out = append(*out, control{
				name: name,
				chk:  (*comp.Check)(unsafe.Pointer(f.UnsafeAddr())),
			})
		case f.Type() == fldT && f.CanAddr():
			*out = append(*out, control{
				name: name,
				fld:  (*comp.Field)(unsafe.Pointer(f.UnsafeAddr())),
			})
		case f.Kind() == reflect.Struct:
			controlsOf(f, name+".", out)
		case f.Kind() == reflect.Slice && f.Type().Elem() == btnT:
			for j := 0; j < f.Len(); j++ {
				if e := f.Index(j); e.CanAddr() {
					*out = append(*out, control{
						name: fmt.Sprintf("%s[%d]", name, j),
						btn:  (*comp.Button)(unsafe.Pointer(e.UnsafeAddr())),
					})
				}
			}
		}
	}
}

// auditOne presses every control on one panel and reports what happened.
func auditOne(t *testing.T, panel string, ctrl any,
	draw func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions,
	snap *state.Snapshot, fired func() int, reset func(),
	skip map[string]string) (dead, unreachable []string, n int) {
	t.Helper()
	h := newPanelHarness(draw, snap)
	h.frame()
	h.frame() // a second frame, because most panels build their labels on the first

	var found []control
	controlsOf(reflect.ValueOf(ctrl), "", &found)
	n = len(found)

	// Fill the boxes first.
	//
	// Several controls read a field and do nothing when it is empty, which is
	// correct behaviour and would be reported here as a dead control. What is
	// being asked is whether pressing it reaches anything at all.
	for _, f := range boxesOf(ctrl) {
		if f.Editor.Text() == "" {
			f.Editor.SetText(fieldGuess(f.Hint))
		}
	}
	h.frame()

	// Wired: press the handler directly, so this is not a test of coordinates.
	for _, c := range found {
		if _, ok := skip[c.name]; ok {
			continue
		}
		if reset != nil {
			reset()
			h.frame()
		}
		if c.fld != nil {
			continue
		}
		before := fired()
		switch {
		case c.btn != nil:
			c.btn.Click.Click()
		case c.chk != nil:
			c.chk.Bool.Value = !c.chk.Bool.Value
		}
		h.frame()
		h.frame()
		if fired() == before && c.chk == nil {
			dead = append(dead, c.name+" ("+label(c)+")")
		}
	}

	// Reachable: sweep a pointer over the panel and see what it lands on.
	sweep := func(at f32.Point) {
		h.click(at)
		if reset != nil {
			reset()
			h.frame()
		}
	}
	for y := float32(2); y < float32(h.sz.Y); y += 5 {
		for x := float32(2); x < float32(h.sz.X); x += 5 {
			sweep(f32.Pt(x, y))
		}
		sweep(f32.Pt(float32(h.sz.X)-2, y))
	}
	if reset != nil {
		reset()
		h.frame()
	}
	for _, c := range found {
		if _, ok := skip[c.name]; ok {
			continue
		}
		switch {
		case c.fld != nil:
		case c.btn != nil && len(c.btn.Click.History()) == 0:
			unreachable = append(unreachable, c.name+" ("+label(c)+")")
		case c.chk != nil && len(c.chk.Bool.History()) == 0:
			unreachable = append(unreachable, c.name+" ("+label(c)+")")
		}
	}
	sort.Strings(dead)
	sort.Strings(unreachable)
	return dead, unreachable, n
}

// boxesOf is every text box a panel owns.
func boxesOf(ctrl any) []*comp.Field {
	var out []*comp.Field
	var walk func(v reflect.Value)
	fieldT := reflect.TypeOf(comp.Field{})
	walk = func(v reflect.Value) {
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Type() == fieldT && f.CanAddr() {
				out = append(out, (*comp.Field)(unsafe.Pointer(f.UnsafeAddr())))
				continue
			}
			if f.Kind() == reflect.Struct {
				walk(f)
			}
		}
	}
	walk(reflect.ValueOf(ctrl))
	return out
}

// fieldGuess is something plausible for a box, read off its own hint. A number
// where it wants a number, a node where it wants a node.
func fieldGuess(hint string) string {
	h := strings.ToLower(hint)
	switch {
	case strings.Contains(h, "seed"), strings.Contains(h, "how many"),
		strings.Contains(h, "seconds"), strings.Contains(h, "ms"),
		strings.Contains(h, "db"), strings.Contains(h, "hours"),
		strings.Contains(h, "km"), strings.Contains(h, "gb"),
		strings.Contains(h, "scale"):
		return "2"
	case strings.Contains(h, "url"):
		return "https://example.invalid"
	case strings.Contains(h, "path"):
		return "/tmp/meshbench-audit"
	case strings.Contains(h, "node"), strings.Contains(h, "sender"):
		return "Abernethy Repeater"
	case strings.Contains(h, "region"):
		return "sco"
	case strings.Contains(h, "role"):
		return "simple_repeater"
	case strings.Contains(h, "version"):
		return "repeater-v1.17.0"
	}
	return "audit"
}

func label(c control) string {
	switch {
	case c.btn != nil && c.btn.Label != "":
		return c.btn.Label
	case c.chk != nil && c.chk.Label != "":
		return c.chk.Label
	}
	return "no label"
}

type target struct {
	name string
	ctrl any
	draw func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions
	// snap overrides the shared one where a panel draws nothing without
	// something particular in it.
	snap *state.Snapshot
	// reset puts the panel back into the state a control needs, before
	// each press.
	reset func()
	// fired overrides how "it did something" is counted, for a panel that
	// changes something other than the simulation.
	fired func() int
	// skip names controls that are not expected to answer here, and why.
	// Every entry is a claim that has to be true, not a way of quieting
	// the test: a control listed for the wrong reason is a control nobody
	// is checking.
	skip map[string]string
}

// auditTargets is every panel that owns controls, wired to a recorder.
//
// Shared by the tests that press the buttons and the one that types into the
// boxes, so a panel added to one is audited by both.
func auditTargets(r *recorder) []target {
	// The file dialog is the platform's, not ours, so the audit cannot drive
	// it - but it can check the button reaches it. Without this the browse
	// buttons look like dead controls, which is the one thing this test
	// exists to catch, and it would have been silenced by exempting them.
	shell.Browse = func(title, _ string, _ shell.PathAsk) (string, error) {
		r.do("ui.browse", title)
		return "", nil
	}

	fleet := &fleetControls{do: r.do}
	fleet.choose = func(title string, _ []string, _ func(string)) { r.do("ui.choose", title) }
	sched := &scheduleControls{do: r.do}
	imp := &importControls{do: r.do}
	bound := &boundaryControls{do: r.do}
	valid := &validateControls{do: r.do}
	plan := &planningControls{do: r.do}
	bench := &benchControls{do: r.do}
	feed := &feedControls{do: r.do}
	sweep := &sweepControls{do: r.do}
	regr := &regressionsPanel{do: r.do}
	sweepRes := &sweepResults{do: r.do}
	prov := &provisioningControls{do: r.do}

	nodes := &nodesPanel{}
	nodes.OnSelect = func(string) { r.do("nodes.select", nil) }
	nv := &nodeViewPanel{}
	// The build list is an overlay: its buttons do not exist until a firmware
	// cell has been clicked, and auditing them shut only proves they are shut.
	nv.pickFor = "Abernethy Repeater"
	nv.OnAction = func(a string, n string) { r.do(a, n) }
	nv.OnFirmware = func(n, v string) { r.do("node.set_firmware", v) }
	nw := &nodeWindowPanel{node: "Abernethy Repeater"}
	snapWithConsole := auditSnapshot()
	snapWithConsole.ConsoleNode = "Abernethy Repeater"
	snapWithConsole.Console = []string{"   0.000  > advert"}
	nw.OnCommand = func(n, l string) { r.do("console.type", l) }
	nw.OnAction = func(a, n string) { r.do(a, n) }
	nw.OnServe = func(node, kind string) { r.do("bench.serve", kind) }
	nw.comp.OnCLI = func(n, l string) { r.do("console.cli", l) }
	cfgSets := &settings{}
	cfg := &configPanel{do: r.do, sets: cfgSets}
	// Choosing is the shell's overlay; what the audit can ask is whether
	// pressing the dropdown reaches the chooser at all.
	cfg.choose = func(title string, _ []string, _ func(string)) { r.do("ui.choose", title) }
	snapGPU := auditSnapshot()
	snapGPU.GPU = state.GPUState{Present: true, Enabled: true,
		Device: "Audit Graphics 3000", Backend: "vulkan"}
	cmpP := &comparePanel{OnSave: func() { r.do("run.save", nil) }}
	planP := &planPanel{OnRun: func() { r.do("plan.routes", nil) }}
	impP := &importPanel{OnFetch: func(u string) { r.do("import.describe", u) }}
	feedP := &feedPanel{OnPull: func() { r.do("feed.pull", nil) }}
	benchP := &benchPanel{OnAction: func(a, n string) { r.do(a, n) }}

	targets := []target{
		{"Nodes running", nv, nv.Draw, nil,
			// Choosing a build closes the list, so it is reopened before each.
			func() { nv.pickFor = "Abernethy Repeater" }, nil, buildSkips()},
		{"Node window", nw, nw.auditDraw, snapWithConsole,
			// The tab row is above everything, so a pointer moving down the
			// panel leaves the console before it reaches the send button.
			func() { nw.tab = 0 }, nil, map[string]string{
				// The companion tab belongs to a companion, and this node is a
				// repeater. Its controls are audited on their own below.
				"comp.applyRadio": "companion only", "comp.applyName": "companion only",
				"comp.getChans": "companion only", "comp.syncMsgs": "companion only",
				"comp.contacts": "companion only", "comp.takeOver": "companion only",
				"comp.sendMsg": "companion only",
				"comp.freq":    "companion only", "comp.bw": "companion only",
				"comp.sf": "companion only", "comp.cr": "companion only",
				"comp.txdbm": "companion only", "comp.name": "companion only",
				"comp.channel": "companion only", "comp.msg": "companion only",
				"comp.release": "companion only",
				// The node is running, so the head offers stop and not start.
				"start": "drawn only when the node is stopped",
			}},
		{"Node window: companion", &nw.comp, nw.comp.auditDraw, nil, nil, nil, nil},
		{"Compare", cmpP, cmpP.Draw, nil, nil, nil, nil},
		{"Planning (view)", planP, planP.Draw, nil, nil, nil, nil},
		{"Import (view)", impP, impP.Draw, nil, nil, nil, nil},
		{"Live feed (view)", feedP, feedP.Draw, nil, nil, nil, nil},
		{"Companion bench (view)", benchP, benchP.Draw, nil, nil, nil, nil},
		{"Fleet", fleet, fleet.Draw, nil, nil, nil, nil},
		{"Schedule", sched, sched.Draw, nil, nil, nil, nil},
		{"Import", imp, imp.Draw, nil, nil, nil, nil},
		{"Boundary", bound, bound.Draw, nil, nil, nil, nil},
		{"Validate", valid, valid.Draw, nil, nil, nil, nil},
		{"Planning", plan, plan.Draw, nil, nil, nil, nil},
		{"Companion bench", bench, bench.Draw, nil, nil, nil, nil},
		{"Live feed", feed, feed.Draw, nil, nil, nil, nil},
		{"Sweep", sweep, sweep.Draw, nil, nil, nil, nil},
		{"Sweep results", sweepRes, sweepRes.Draw, nil, nil, nil, nil},
		{"Regressions", regr, regr.Draw, nil, nil, nil, nil},
		{"Provisioning", prov, prov.Draw, nil, nil, nil, nil},
		// The flat layout, so every section's controls are on screen at once;
		// the sidebar's own switching is TestConfigurationSectionsSwitch.
		// Fired counts the settings generation too: the Interface controls
		// change the interface, not the simulation, and that is still doing
		// something.
		{"Configuration", cfg, cfg.auditDraw, snapGPU, nil,
			func() int {
				_, _, gen := cfgSets.get()
				return len(r.verbs)*1000 + int(gen)
			}, nil},
	}
	return targets
}

func TestEveryControlIsWiredAndReachable(t *testing.T) {
	snap := auditSnapshot()

	r := &recorder{}
	fired := func() int { return len(r.verbs) }

	targets := auditTargets(r)

	total, deadAll, unreachAll := 0, 0, 0
	var report []string
	for _, tg := range targets {
		count := fired
		if tg.fired != nil {
			count = tg.fired
		}
		use := snap
		if tg.snap != nil {
			use = tg.snap
		}
		dead, unreachable, n := auditOne(t, tg.name, tg.ctrl, tg.draw, use,
			count, tg.reset, tg.skip)
		total += n
		deadAll += len(dead)
		unreachAll += len(unreachable)
		line := fmt.Sprintf("%-16s %2d controls", tg.name, n)
		if len(dead) > 0 {
			line += "\n      reaches nothing: " + strings.Join(dead, ", ")
		}
		if len(unreachable) > 0 {
			line += "\n      no pointer lands on it: " + strings.Join(unreachable, ", ")
		}
		report = append(report, line)
	}
	for _, l := range report {
		t.Log(l)
	}
	t.Logf("%d controls across %d panels: %d reach nothing, %d cannot be clicked",
		total, len(targets), deadAll, unreachAll)
	if deadAll > 0 || unreachAll > 0 {
		t.Errorf("%d controls reach nothing and %d cannot be reached by a pointer",
			deadAll, unreachAll)
	}
}

// auditSnapshot is a network with something in every list, so a panel that
// draws its controls only when it has data draws them.
func auditSnapshot() *state.Snapshot {
	return &state.Snapshot{
		Nodes: []state.Node{
			{Name: "Abernethy Repeater", Kind: "repeater", Lat: 56.3, Lon: -3.3, Selected: true},
			{Name: "Bishop Hill", Kind: "repeater", Lat: 56.2, Lon: -3.2},
			{Name: "AngusOutlaw1", Kind: "companion", Lat: 56.5, Lon: -3.0},
		},
		Stats: []state.NodeStat{
			{Name: "Abernethy Repeater", Backend: "native", Running: true, RSSBytes: 4 << 20},
			{Name: "Bishop Hill", Backend: "native", Running: true, RSSBytes: 4 << 20},
		},
	}
}

// buildSkips names what the sweep is not expected to land on in the build list.
//
// Twenty-five builds do not fit in the overlay, and a list you have to scroll
// is not a list that is broken. The ones below the fold are reached by typing
// into the filter above them, which TestAnyBuildIsReachableByFiltering walks
// rather than assumes.
func buildSkips() map[string]string {
	skip := map[string]string{
		// Cancel closes the list. That is the whole of its job, so it reaches
		// no verb by design.
		"closePick": "closes the list rather than reaching a verb",
	}
	for i := 11; i < 25; i++ {
		skip[fmt.Sprintf("buildBtns[%d]", i)] = "below the fold; reached by filtering"
	}
	return skip
}

// The build you want is not always one of the eleven that fit.
func TestAnyBuildIsReachableByFiltering(t *testing.T) {
	nv := &nodeViewPanel{}
	nv.pickFor = "Abernethy Repeater"
	got := ""
	nv.OnFirmware = func(node, version string) { got = version }

	h := newPanelHarness(nv.Draw, auditSnapshot())
	h.frame()
	h.frame()
	if len(nv.builds) < 12 {
		t.Skipf("only %d builds installed, so nothing is below the fold", len(nv.builds))
	}
	want := nv.builds[len(nv.builds)-1]

	// Type enough of its name to leave it alone in the list.
	nv.pickFilter.Editor.SetText(want)
	h.frame()

	// Upward, because cancel sits at the top of the card and closing the list
	// on the way to the thing inside it proves nothing.
	for y := float32(h.sz.Y) - 2; y > 2 && got == ""; y -= 4 {
		for x := float32(2); x < float32(h.sz.X) && got == ""; x += 8 {
			h.click(f32.Pt(x, y))
		}
	}
	if got != want {
		t.Fatalf("filtered the build list to %q and clicking it reached %q; "+
			"a build that does not fit on screen has to be reachable by "+
			"narrowing the list", want, got)
	}
}

// Can you type into it?
//
// A text box draws whether or not it can hold a keystroke. The Nodes filter
// drew perfectly for weeks and accepted nothing, because it built its editor
// inside Draw and Gio keys focus on a widget's address: the click focused an
// editor that no longer existed by the time the characters arrived. So this
// clicks every box on every panel and types, which is the only way to tell.
func TestEveryTextBoxAcceptsTyping(t *testing.T) {
	r := &recorder{}
	var deaf []string
	total := 0
	for _, tg := range auditTargets(r) {
		snap := tg.snap
		if snap == nil {
			snap = auditSnapshot()
		}
		h := newPanelHarness(tg.draw, snap)
		h.frame()
		h.frame()
		if tg.reset != nil {
			tg.reset()
			h.frame()
		}

		var found []control
		controlsOf(reflect.ValueOf(tg.ctrl), "", &found)
		var boxes []control
		for _, c := range found {
			if _, ok := tg.skip[c.name]; ok {
				continue
			}
			if c.fld != nil {
				boxes = append(boxes, c)
				c.fld.Editor.SetText("")
			}
		}
		if len(boxes) == 0 {
			continue
		}
		total += len(boxes)
		typed := map[string]bool{}

		// Wide steps across, because a text box is a full row and a narrow
		// sweep only costs time.
		for y := float32(2); y < float32(h.sz.Y); y += 6 {
			for x := float32(20); x < float32(h.sz.X); x += 60 {
				h.click(f32.Pt(x, y))
				h.typeText("z")
				for _, c := range boxes {
					if c.fld.Editor.Text() != "" {
						typed[c.name] = true
						c.fld.Editor.SetText("")
					}
				}
				if tg.reset != nil {
					tg.reset()
					h.frame()
				}
			}
		}
		for _, c := range boxes {
			if !typed[c.name] {
				deaf = append(deaf, tg.name+": "+c.name+" ("+c.fld.Hint+")")
			}
		}
	}
	t.Logf("%d text boxes across the workbench", total)
	if len(deaf) > 0 {
		sort.Strings(deaf)
		t.Errorf("text boxes that took no keystroke:\n  %s", strings.Join(deaf, "\n  "))
	}
}
