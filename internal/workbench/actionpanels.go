package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
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

var _ = fmt.Sprintf

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
