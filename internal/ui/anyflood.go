package ui

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"
)

// Forwarding flood traffic for any region, across the fleet.
//
// A freshly imported mesh holds no regions, and a mesh whose regions were
// inferred but not applied holds none either. Either way every repeater
// receives a scoped flood packet, derives a key it was never given, and
// declines to forward it. There is no error: the senders transmit, the ledger
// fills with "first time this node heard it", and nothing relays. It reads
// exactly like a network with no propagation.
//
// This is the way out of that, and it is a real change to the network rather
// than a display option, so it is stored on the nodes, saved with the scenario,
// and said out loud wherever it is on.

// anyFloodState reports how many transmitting nodes there are and how many of
// them forward any region's flood traffic.
func (a *App) anyFloodState() (on, total int) {
	for i := range a.Nodes {
		if !a.Nodes[i].Kind.Transmits() {
			continue
		}
		total++
		if a.Nodes[i].AllowAnyFlood {
			on++
		}
	}
	return on, total
}

// setAnyFlood turns it on or off across every transmitting node and returns how
// many changed. Nodes that do not transmit are left alone: an observer has no
// forwarding to permit, and setting it there would make the count lie.
func (a *App) setAnyFlood(on bool) int {
	n := 0
	for i := range a.Nodes {
		if !a.Nodes[i].Kind.Transmits() || a.Nodes[i].AllowAnyFlood == on {
			continue
		}
		a.Nodes[i].AllowAnyFlood = on
		n++
	}
	// Told to the nodes that are already up, so the switch does something now
	// as well as at the next start. With nothing running there is nobody to
	// tell, and the stored field is the whole of the change.
	if n > 0 && a.eng != nil && a.eng.FirmwareCount() > 0 {
		a.applyRegionsToFleet()
	}
	return n
}

// drawAnyFloodSwitch is the operator's half, drawn under the inference results
// because that is where somebody arrives holding a mesh that will not relay.
func (a *App) drawAnyFloodSwitch() {
	on, total := a.anyFloodState()
	all := total > 0 && on == total
	if imgui.Checkbox("forward flood traffic for any region", &all) {
		a.setAnyFlood(all)
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("region allowf *\n\n" +
			"Every repeater forwards a flood packet whatever region it is scoped\n" +
			"to, including regions it does not hold.\n\n" +
			"This is more permissive than any real network. A reach question\n" +
			"answered with it on is answered more generously than reality, so\n" +
			"turn it off before believing a result.\n\n" +
			"It exists because a mesh with no regions relays nothing and reports\n" +
			"no error at all, which is indistinguishable from bad RF.")
	}
	if on > 0 && on < total {
		imgui.SameLine()
		textDim(fmt.Sprintf("%d of %d transmitting nodes", on, total))
	}
}
