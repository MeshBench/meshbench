// The Fleet controls: sending one command to every node, or to a filtered
// subset.
package workbench

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

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
