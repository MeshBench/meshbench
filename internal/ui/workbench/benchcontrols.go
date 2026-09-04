// The Bench controls: the companion bench - a mesh and an endpoint, then the
// faults a happy path never reaches.
package workbench

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// benchControls is the companion bench: a mesh and an endpoint, then the
// faults a happy path never reaches.
type benchControls struct {
	bar    comp.ActionBar
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
		c.msg.Hint = "a message to send from it"
		c.msg.Editor.SingleLine = true
		c.tcp.Label, c.tcp.Kind = "serve TCP", comp.Primary
		c.serial.Label, c.serial.Kind = "serve serial", comp.Secondary
		c.drop.Label, c.drop.Kind = "drop clients", comp.Destructive
		c.stray.Label, c.stray.Kind = "inject a stray frame", comp.Secondary
		c.conn.Label, c.conn.Kind = "connect as a client", comp.Primary
		c.send.Label, c.send.Kind = "send", comp.Secondary
		c.advert.Label, c.advert.Kind = "advert", comp.Secondary
		c.bar.Buttons = []*comp.Button{&c.tcp, &c.serial, &c.drop, &c.stray}
		c.bar.Note = "both transports carry the firmware's own serial protocol byte " +
			"for byte; the faults are what an application that reconnects cleanly survives"
		c.built = true
	}
	// Whichever companion is selected: the bench's table is what selects
	// one now, so a box asking for the name again was a second way to say
	// the same thing, and nobody typed into it.
	who := func() string { return comp.SelectedNodeName(s) }
	// Serving needs firmware. A companion's port is the firmware's own
	// serial interface, so with nothing running there is nothing to expose -
	// the verb refuses, correctly, and this says so before the press rather
	// than after it.
	running := s != nil && s.FirmwareRunning > 0
	c.bar.Note = "both transports carry the firmware's own serial protocol byte " +
		"for byte; the faults are what an application that reconnects cleanly survives"
	if !running {
		c.bar.Note = "nothing is running: a companion's port is its firmware's own " +
			"serial interface, so start the firmware - Simulation, then start " +
			"firmware on every node - before serving one"
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
		c.do("companion.send", map[string]any{"node": who(), "text": comp.FieldText(&c.msg)})
	}
	if c.advert.Click.Clicked(gtx) && c.do != nil {
		c.do("companion.advert", map[string]any{"node": who()})
	}
	second := comp.ActionBar{
		Fields:  []*comp.Field{&c.msg},
		Buttons: []*comp.Button{&c.conn, &c.send, &c.advert},
		Note:    "connecting claims the node's port, so its console goes quiet until you disconnect",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.bar.Layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return second.Layout(t, gtx) }),
	)
}
