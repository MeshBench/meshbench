// The panels that talk to something outside the simulation: the companion
// bench, and the live feed of observed traffic.
package workbench

import (
	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

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
