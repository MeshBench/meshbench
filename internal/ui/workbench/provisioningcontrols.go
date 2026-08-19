// The Provisioning controls: what every node is told at boot.
package workbench

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

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
