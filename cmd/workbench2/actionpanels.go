package main

import (
	"fmt"
	"strconv"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Do is how these panels reach the store. One function rather than a callback
// per control, because every one of them is "run this verb with these
// parameters and say what came back".
type Do func(verb string, params any)

// fleetControls sends one command to every node, or to a filtered subset.
type fleetControls struct {
	bar     actionBar
	command comp.Field
	// kind is a dropdown, not a box: the tokens are the scenario's own -
	// "simple-repeater", not "repeater" - and a box that wants the exact
	// token is a box that silently filters to nobody when given the word a
	// person would type.
	kind comp.Dropdown
	// kindTok is the chosen scenario token; empty sends to every kind.
	kindTok string
	// choose opens the shell's chooser, the one way anything picks from a list.
	choose  func(title string, opts []string, pick func(string))
	send    comp.Button
	regions comp.Field
	setReg  comp.Button
	allow   comp.Button
	// quick are the lines people send often enough that typing them is the
	// only thing standing between a mesh and saying something. The old
	// workbench put them behind a button each; the same idea, and they fill
	// the box rather than going out the door, so the exact line is read
	// before forty nodes are asked to obey it.
	quick [4]comp.Button
	do    Do
	built bool
}

// fleetKinds is the kind filter, in a person's words on the left and the
// scenario's own token on the right.
var fleetKinds = []struct{ label, token string }{
	{"every kind", ""},
	{"repeaters", "simple-repeater"},
	{"advanced repeaters", "advanced-repeater"},
	{"room servers", "room-server"},
	{"companions", "companion"},
	{"observers", "sdr-observer"},
	{"emitters", "emitter"},
}

// fleetQuick is what those buttons put in the box.
var fleetQuick = [4]struct{ label, cmd string }{
	{"advert", "advert"},
	{"flood advert", "floodadv"},
	{"what version", "ver"},
	{"how it is", "status"},
}

func (c *fleetControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.command.Hint = "a MeshCore command, sent to every node"
		c.command.Editor.SingleLine = true
		c.kind.Value = "every kind"
		c.kind.OnOpen = func() {
			if c.choose == nil {
				return
			}
			opts := make([]string, 0, len(fleetKinds))
			for _, k := range fleetKinds {
				opts = append(opts, k.label)
			}
			c.choose("Send to which kind?", opts, func(picked string) {
				for _, k := range fleetKinds {
					if k.label == picked {
						c.kindTok, c.kind.Value = k.token, k.label
					}
				}
			})
		}
		c.regions.Hint = "regions, space separated"
		c.regions.Editor.SingleLine = true
		c.send.Label, c.send.Kind = "send to the fleet", comp.Primary
		c.setReg.Label, c.setReg.Kind = "set regions", comp.Secondary
		c.allow.Label, c.allow.Kind = "allow any flood", comp.Secondary
		c.bar.fields = []*comp.Field{&c.command}
		c.bar.extras = []func(*theme.Theme, layout.Context) layout.Dimensions{
			func(t *theme.Theme, gtx layout.Context) layout.Dimensions {
				return c.kind.Layout(t, gtx)
			},
		}
		c.bar.buttons = []*comp.Button{&c.send}
		for i := range c.quick {
			c.quick[i].Label, c.quick[i].Kind = fleetQuick[i].label, comp.Quiet
		}
		c.bar.note = "a command that changes what the nodes are makes anything " +
			"already measured a different mesh; the reply says so before it is sent"
		c.built = true
	}
	if c.send.Click.Clicked(gtx) && c.do != nil {
		c.do("fleet.send", map[string]any{
			"command": fieldText(&c.command), "kind": c.kindTok,
		})
	}
	if c.setReg.Click.Clicked(gtx) && c.do != nil {
		var rs []any
		for _, r := range splitFields(fieldText(&c.regions)) {
			rs = append(rs, r)
		}
		c.do("nodes.regions", map[string]any{"regions": rs})
	}
	if c.allow.Click.Clicked(gtx) && c.do != nil {
		c.do("nodes.allow_flood", map[string]any{"on": true})
	}
	for i := range c.quick {
		if c.quick[i].Click.Clicked(gtx) {
			c.command.Editor.SetText(fleetQuick[i].cmd)
		}
	}
	second := actionBar{
		fields:  []*comp.Field{&c.regions},
		buttons: []*comp.Button{&c.setReg, &c.allow},
	}
	third := actionBar{
		buttons: []*comp.Button{&c.quick[0], &c.quick[1], &c.quick[2], &c.quick[3]},
		note: "nothing in a mesh speaks until it is asked to: MeshCore will not " +
			"advertise more often than hourly, so a short run is made to talk " +
			"with advert",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return third.layout(t, gtx) }),
	)
}

// scheduleControls adds sends and assertions to a scenario.
type scheduleControls struct {
	bar     actionBar
	node    comp.Field
	at      comp.Field
	every   comp.Field
	add     comp.Button
	clear   comp.Button
	kind    comp.Field
	atLeast comp.Field
	addAss  comp.Button
	check   comp.Button
	do      Do
	built   bool
}

func (c *scheduleControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.node.Hint = "node (blank: the selected one)"
		c.at.Hint = "at, seconds"
		c.every.Hint = "every, seconds (blank: once)"
		c.kind.Hint = "assert: delivered, sent"
		c.atLeast.Hint = "at least"
		for _, f := range []*comp.Field{&c.node, &c.at, &c.every, &c.kind, &c.atLeast} {
			f.Editor.SingleLine = true
		}
		c.add.Label, c.add.Kind = "add send", comp.Primary
		c.clear.Label, c.clear.Kind = "clear sends", comp.Quiet
		c.addAss.Label, c.addAss.Kind = "add assertion", comp.Secondary
		c.check.Label, c.check.Kind = "check now", comp.Primary
		c.bar.fields = []*comp.Field{&c.node, &c.at, &c.every}
		c.bar.buttons = []*comp.Button{&c.add, &c.clear}
		c.built = true
	}
	if c.add.Click.Clicked(gtx) && c.do != nil {
		node := fieldText(&c.node)
		if node == "" {
			node = selectedNodeName(s)
		}
		p := map[string]any{"node": node}
		if v, ok := num(&c.at); ok {
			p["at_ms"] = v * 1000
		}
		if v, ok := num(&c.every); ok {
			p["every_ms"] = v * 1000
		}
		c.do("schedule.add", p)
	}
	if c.clear.Click.Clicked(gtx) && c.do != nil {
		c.do("schedule.clear", nil)
	}
	if c.addAss.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{"kind": fieldText(&c.kind)}
		if v, ok := num(&c.atLeast); ok {
			p["at_least"] = v
		}
		c.do("assert.add", p)
	}
	if c.check.Click.Clicked(gtx) && c.do != nil {
		c.do("assert.check", nil)
	}
	second := actionBar{
		fields:  []*comp.Field{&c.kind, &c.atLeast},
		buttons: []*comp.Button{&c.addAss, &c.check},
		note:    "an assertion whose kind this build does not understand fails rather than passing quietly",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.layout(t, gtx) }),
	)
}

// importControls is the order that matters, as buttons in that order.
type importControls struct {
	bar    actionBar
	url    comp.Field
	fetch  comp.Button
	commit comp.Button
	infer  comp.Button
	apply  comp.Button
	do     Do
	built  bool
}

func (c *importControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.url.Hint = "a CoreScope deployment URL"
		c.url.Editor.SingleLine = true
		c.fetch.Label, c.fetch.Kind = "1. fetch", comp.Primary
		c.commit.Label, c.commit.Kind = "2. commit", comp.Secondary
		c.infer.Label, c.infer.Kind = "3. read traffic", comp.Secondary
		c.apply.Label, c.apply.Kind = "4. apply regions", comp.Primary
		c.bar.fields = []*comp.Field{&c.url}
		c.bar.buttons = []*comp.Button{&c.fetch, &c.commit, &c.infer, &c.apply}
		c.bar.note = "numbered because the order matters and every step has been " +
			"skipped: a mesh with regions inferred but not applied transmits " +
			"everything, relays nothing, and reports no error"
		c.built = true
	}
	if c.fetch.Click.Clicked(gtx) && c.do != nil {
		c.do("import.fetch", map[string]any{"url": fieldText(&c.url)})
	}
	if c.commit.Click.Clicked(gtx) && c.do != nil {
		c.do("import.commit", map[string]any{"strategy": "replace-all"})
	}
	if c.infer.Click.Clicked(gtx) && c.do != nil {
		c.do("infer.run", map[string]any{"hours": 168})
	}
	if c.apply.Click.Clicked(gtx) && c.do != nil {
		c.do("infer.apply", nil)
	}
	return c.bar.layout(t, gtx)
}

// boundaryControls chooses the study area.
type boundaryControls struct {
	bar    actionBar
	place  comp.Field
	margin comp.Field
	search comp.Button
	accept comp.Button
	prune  comp.Button
	do     Do
	built  bool
}

func (c *boundaryControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.place.Hint = "a place: Fife, Scotland, Perth and Kinross"
		c.margin.Hint = "margin km"
		c.place.Editor.SingleLine = true
		c.margin.Editor.SingleLine = true
		c.search.Label, c.search.Kind = "search", comp.Primary
		c.accept.Label, c.accept.Kind = "accept it", comp.Secondary
		c.prune.Label, c.prune.Kind = "delete what is outside", comp.Destructive
		c.bar.fields = []*comp.Field{&c.place, &c.margin}
		c.bar.buttons = []*comp.Button{&c.search, &c.accept, &c.prune}
		c.bar.note = "the study area decides which nodes are measured, not which " +
			"packets are forwarded - the margin keeps what interferes from outside it"
		c.built = true
	}
	if c.search.Click.Clicked(gtx) && c.do != nil {
		c.do("boundary.set", map[string]any{"query": fieldText(&c.place)})
	}
	if c.accept.Click.Clicked(gtx) && c.do != nil {
		c.do("boundary.accept", map[string]any{"name": fieldText(&c.place)})
	}
	if c.prune.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.margin); ok {
			p["margin_km"] = v
		}
		c.do("boundary.prune", p)
	}
	return c.bar.layout(t, gtx)
}

// validateControls is the calibration chain.
type validateControls struct {
	bar   actionBar
	hours comp.Field
	db    comp.Field
	fetch comp.Button
	cal   comp.Button
	uncal comp.Button
	do    Do
	built bool
}

func (c *validateControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.hours.Hint = "hours to look back"
		c.db.Hint = "excess loss dB (blank: what was measured)"
		c.hours.Editor.SingleLine = true
		c.db.Editor.SingleLine = true
		c.fetch.Label, c.fetch.Kind = "fetch and compare", comp.Primary
		c.cal.Label, c.cal.Kind = "apply calibration", comp.Secondary
		c.uncal.Label, c.uncal.Kind = "back to the default", comp.Quiet
		c.bar.fields = []*comp.Field{&c.hours, &c.db}
		c.bar.buttons = []*comp.Button{&c.fetch, &c.cal, &c.uncal}
		c.bar.note = "positive residual means the model predicted more signal than " +
			"was heard, so it is optimistic and the excess loss should go up"
		c.built = true
	}
	if c.fetch.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.hours); ok {
			p["hours"] = v
		}
		c.do("validate.fetch", p)
	}
	if c.cal.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.db); ok {
			p["db"] = v
		}
		c.do("validate.calibrate", p)
	}
	if c.uncal.Click.Clicked(gtx) && c.do != nil {
		c.do("validate.uncalibrate", nil)
	}
	return c.bar.layout(t, gtx)
}

// planningControls asks the three network-wide questions.
type planningControls struct {
	bar   actionBar
	best  comp.Button
	gaps  comp.Button
	red   comp.Button
	here  comp.Button
	do    Do
	built bool
}

func (c *planningControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.best.Label, c.best.Kind = "best server", comp.Primary
		c.gaps.Label, c.gaps.Kind = "gaps", comp.Secondary
		c.red.Label, c.red.Kind = "redundancy", comp.Secondary
		c.here.Label, c.here.Kind = "coverage from the selected node", comp.Secondary
		c.bar.buttons = []*comp.Button{&c.best, &c.gaps, &c.red, &c.here}
		c.bar.note = "for a person with a handheld at 1.5 m, which is the assumption " +
			"every one of these makes"
		c.built = true
	}
	for b, mode := range map[*comp.Button]string{
		&c.best: "best", &c.gaps: "gaps", &c.red: "redundancy", &c.here: "node",
	} {
		if b.Click.Clicked(gtx) && c.do != nil {
			c.do("coverage.start", map[string]any{"mode": mode})
		}
	}
	return c.bar.layout(t, gtx)
}

func splitFields(s string) []string {
	var out []string
	for _, f := range fieldsOf(s) {
		out = append(out, f)
	}
	return out
}

func fieldsOf(s string) []string {
	var out, cur []rune
	_ = out
	var res []string
	for _, r := range s {
		if r == ' ' || r == ',' || r == '\t' {
			if len(cur) > 0 {
				res = append(res, string(cur))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		res = append(res, string(cur))
	}
	return res
}

var _ = fmt.Sprintf

// benchControls is the companion bench: a mesh and an endpoint, then the
// faults a happy path never reaches.
type benchControls struct {
	bar    actionBar
	node   comp.Field
	tcp    comp.Button
	serial comp.Button
	drop   comp.Button
	stray  comp.Button
	conn   comp.Button
	msg    comp.Field
	send   comp.Button
	advert comp.Button
	do     Do
	built  bool
}

func (c *benchControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.node.Hint = "companion (blank: the selected node)"
		c.msg.Hint = "a message to send from it"
		c.node.Editor.SingleLine = true
		c.msg.Editor.SingleLine = true
		c.tcp.Label, c.tcp.Kind = "serve TCP", comp.Primary
		c.serial.Label, c.serial.Kind = "serve serial", comp.Secondary
		c.drop.Label, c.drop.Kind = "drop clients", comp.Destructive
		c.stray.Label, c.stray.Kind = "inject a stray frame", comp.Secondary
		c.conn.Label, c.conn.Kind = "connect as a client", comp.Primary
		c.send.Label, c.send.Kind = "send", comp.Secondary
		c.advert.Label, c.advert.Kind = "advert", comp.Secondary
		c.bar.fields = []*comp.Field{&c.node}
		c.bar.buttons = []*comp.Button{&c.tcp, &c.serial, &c.drop, &c.stray}
		c.bar.note = "both transports carry the firmware's own serial protocol byte " +
			"for byte; the faults are what an application that reconnects cleanly survives"
		c.built = true
	}
	who := func() string {
		if n := fieldText(&c.node); n != "" {
			return n
		}
		return selectedNodeName(s)
	}
	if c.tcp.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.serve", map[string]any{"node": who(), "kind": "tcp"})
	}
	if c.serial.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.serve", map[string]any{"node": who(), "kind": "serial"})
	}
	if c.drop.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.drop", nil)
	}
	if c.stray.Click.Clicked(gtx) && c.do != nil {
		c.do("bench.stray", map[string]any{"node": who()})
	}
	if c.conn.Click.Clicked(gtx) && c.do != nil {
		c.do("companion.connect", map[string]any{"node": who()})
	}
	if c.send.Click.Clicked(gtx) && c.do != nil {
		c.do("companion.send", map[string]any{"node": who(), "text": fieldText(&c.msg)})
	}
	if c.advert.Click.Clicked(gtx) && c.do != nil {
		c.do("companion.advert", map[string]any{"node": who()})
	}
	second := actionBar{
		fields:  []*comp.Field{&c.msg},
		buttons: []*comp.Button{&c.conn, &c.send, &c.advert},
		note:    "connecting claims the node's port, so its console goes quiet until you disconnect",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.layout(t, gtx) }),
	)
}

// feedControls replays the real network's traffic into the simulation.
type feedControls struct {
	bar   actionBar
	url   comp.Field
	start comp.Button
	stop  comp.Button
	do    Do
	built bool
}

func (c *feedControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.url.Hint = "a CoreScope deployment URL"
		c.url.Editor.SingleLine = true
		c.start.Label, c.start.Kind = "start live feed", comp.Primary
		c.stop.Label, c.stop.Kind = "stop", comp.Quiet
		c.bar.fields = []*comp.Field{&c.url}
		c.bar.buttons = []*comp.Button{&c.start, &c.stop}
		c.bar.note = "packets are taken at their first hop and re-transmitted by the " +
			"same-named node here, so what you watch is the simulated mesh relaying real traffic"
		c.built = true
	}
	if c.start.Click.Clicked(gtx) && c.do != nil {
		c.do("feed.pull", map[string]any{"url": fieldText(&c.url)})
	}
	if c.stop.Click.Clicked(gtx) && c.do != nil {
		c.do("feed.stop", nil)
	}
	return c.bar.layout(t, gtx)
}

// sweepControls defines an A/B experiment: arms, seeds, sender, timing.
type sweepControls struct {
	bar      actionBar
	versions comp.Field
	seeds    comp.Field
	sender   comp.Field
	runFor   comp.Field
	define   comp.Button
	start    comp.Button
	stop     comp.Button
	export   comp.Button
	do       Do
	built    bool
}

func (c *sweepControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.versions.Hint = "repeater versions, space separated"
		c.seeds.Hint = "seeds, e.g. 1 2 3 4"
		c.sender.Hint = "sender: a companion"
		c.runFor.Hint = "run for, seconds"
		for _, f := range []*comp.Field{&c.versions, &c.seeds, &c.sender, &c.runFor} {
			f.Editor.SingleLine = true
		}
		c.define.Label, c.define.Kind = "define", comp.Secondary
		c.start.Label, c.start.Kind = "run it", comp.Primary
		c.stop.Label, c.stop.Kind = "stop", comp.Destructive
		c.export.Label, c.export.Kind = "export", comp.Quiet
		c.bar.fields = []*comp.Field{&c.versions, &c.seeds}
		c.bar.buttons = []*comp.Button{&c.define, &c.start, &c.stop, &c.export}
		c.bar.note = "a message is originated by a companion, so the sender has to be " +
			"one; two seeds that agree exactly are one draw repeated, not a spread"
		c.built = true
	}
	if c.define.Click.Clicked(gtx) && c.do != nil {
		// Said aloud when nothing was defined.
		//
		// Every branch below is guarded on its box holding something, so
		// pressing this with the boxes empty did nothing at all and reported
		// nothing at all - which is indistinguishable from a button that is
		// not connected.
		asked := 0
		var vs []any
		for _, v := range splitFields(fieldText(&c.versions)) {
			vs = append(vs, v)
		}
		if len(vs) > 0 {
			asked++
			c.do("experiment.vary", map[string]any{
				"parameter": "repeater_version", "values": vs})
		}
		var ss []any
		for _, v := range splitFields(fieldText(&c.seeds)) {
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				ss = append(ss, n)
			}
		}
		if len(ss) > 0 {
			asked++
			c.do("experiment.seeds", map[string]any{"seeds": ss})
		}
		if n := fieldText(&c.sender); n != "" {
			asked++
			c.do("experiment.senders", map[string]any{"senders": []any{n}})
		}
		if v, ok := num(&c.runFor); ok {
			asked++
			c.do("experiment.base", map[string]any{"run_for_ms": v * 1000})
		}
		if asked == 0 {
			c.do("ui.said", "nothing to define: fill in the versions to compare, "+
				"the seeds to run each on, who sends, or how long a cell runs")
		}
	}
	if c.start.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.start", nil)
	}
	if c.stop.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.stop", nil)
	}
	if c.export.Click.Clicked(gtx) && c.do != nil {
		c.do("experiment.export", nil)
	}
	second := actionBar{
		fields:  []*comp.Field{&c.sender, &c.runFor},
		buttons: nil,
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.layout(t, gtx) }),
	)
}

// sweepResults is what the arms came back with, and whether it is a result.
type sweepResults struct {
	tb    comp.Table
	init  bool
	seq   uint64
	shown bool
}

func (p *sweepResults) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "arm", Width: 190, Sortable: true},
			{Title: "runs", Width: 60, Right: true, Mono: true},
			{Title: "tx", Width: 90, Right: true, Mono: true, Sortable: true},
			{Title: "rx", Width: 90, Right: true, Mono: true, Sortable: true},
			{Title: "delivered", Width: 100, Right: true, Mono: true},
			{Title: "redundant", Width: 100, Right: true, Mono: true},
			{Title: "rx spread", Right: true, Mono: true},
		}
		p.tb.SortCol, p.tb.SortDesc = 2, true
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	if !p.shown || s.Seq != p.seq {
		rows := make([]comp.Row, 0, len(s.Experiment))
		for _, a := range s.Experiment {
			rows = append(rows, comp.Row{
				Key: a.Arm,
				Cells: []string{
					a.Arm, fmt.Sprintf("%d", a.Runs),
					fmt.Sprintf("%.1f", a.TX), fmt.Sprintf("%.1f", a.RX),
					fmt.Sprintf("%.1f", a.Delivered), fmt.Sprintf("%.1f", a.Redundant),
					fmt.Sprintf("%.0f", a.RXSpread),
				},
			})
		}
		p.tb.SetRows(rows)
		p.seq, p.shown = s.Seq, true
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if s.ExperimentWarning == "" {
				return layout.Dimensions{}
			}
			// Said above the numbers, not below them: a warning underneath a
			// table is read after somebody has already believed it.
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				comp.OneLine(t, t.Sz.Body, t.P.Warn, "not a result yet: "+s.ExperimentWarning, false))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(s.Experiment) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"define arms and seeds above, then run it"))
			}
			return p.tb.Layout(t, gtx, func(string) {})
		}),
	)
}

// provisioningControls is what every node is told at boot.
//
// Each switch here is a failure somebody has had. A node without its name
// reports as its board type, so an event log names hardware rather than
// places. One without its position advertises none, and a client draws it at
// null island. One whose clock disagrees rejects messages as replays, which
// reads as a radio fault.
type provisioningControls struct {
	name, pos, clock comp.Check
	region, scope    comp.Check
	hops, stagger    comp.Field
	extra            comp.Field
	apply            comp.Button
	toRunning        comp.Button
	do               Do
	built            bool
	sent             bool
}

func (c *provisioningControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.name.Label, c.name.Bool.Value = "set each node's name", true
		c.pos.Label, c.pos.Bool.Value = "tell it where it is", true
		c.clock.Label, c.clock.Bool.Value = "give them all one clock", true
		c.region.Label = "define a region from the study area"
		c.scope.Label = "and make it the default scope"
		c.hops.Hint = "cap advert hops (blank: leave alone)"
		c.stagger.Hint = "stagger starts, ms"
		c.extra.Hint = "anything else this study needs, one command per line"
		c.hops.Editor.SingleLine = true
		c.stagger.Editor.SingleLine = true
		c.apply.Label, c.apply.Kind = "use for the next start", comp.Primary
		c.toRunning.Label, c.toRunning.Kind = "send to nodes already running", comp.Secondary
		c.built = true
	}
	settings := func() map[string]any {
		p := map[string]any{
			"set_name": c.name.Bool.Value, "set_position": c.pos.Bool.Value,
			"set_clock": c.clock.Bool.Value, "region_from_area": c.region.Bool.Value,
			"default_scope": c.scope.Bool.Value, "extra": fieldText(&c.extra),
		}
		if v, ok := num(&c.hops); ok {
			p["advert_hops"] = v
		}
		if v, ok := num(&c.stagger); ok {
			p["stagger_ms"] = v
		}
		return p
	}
	if c.apply.Click.Clicked(gtx) && c.do != nil {
		c.do("provisioning.set", settings())
	}
	if c.toRunning.Click.Clicked(gtx) && c.do != nil {
		c.do("provisioning.set", settings())
		c.do("provisioning.apply", nil)
	}

	checks := []*comp.Check{&c.name, &c.pos, &c.clock, &c.region, &c.scope}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var kids []layout.FlexChild
			for _, ch := range checks {
				ch := ch
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.M, Bottom: t.Sp.XS}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return ch.Layout(t, gtx)
						})
				}))
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bar := actionBar{
				fields:  []*comp.Field{&c.hops, &c.stagger},
				buttons: []*comp.Button{&c.apply, &c.toRunning},
				note: "these are the lines the node panel shows under \"is told, at boot\", " +
					"so what you read there and what a node is sent cannot drift apart",
			}
			return bar.layout(t, gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.extra.Layout(t, gtx)
			})
		}),
	)
}
