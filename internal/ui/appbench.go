package ui

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// The Companion bench: the front door for somebody writing an application
// against MeshCore rather than studying a network.
//
// Everything here already existed somewhere - firmware starts from a menu, a
// port is served from inside a node's own window, a frame is injected from the
// control socket. What did not exist was a place that assumes you want a mesh
// and an address to point your app at, and gets you there in one click. An
// application developer does not care about capture effect, and should not have
// to learn where the workbench keeps things in order to test a client.

// drawCompanionBench is the panel body.
func (a *App) drawCompanionBench() {
	imgui.SeparatorText("A mesh and an endpoint")

	comps := a.companionIndices()
	if len(comps) == 0 {
		textDimWrap("No companion in this scenario. Load one of the shipped fixtures - " +
			"every one of them carries several - or place a companion on the map.")
		return
	}

	running := 0
	if a.eng != nil {
		running = a.eng.FirmwareCount()
	}
	if imgui.Button("give me a mesh and an endpoint") {
		a.benchStartAndServe(comps[0])
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Starts firmware on every node if it is not already running,\n" +
			"then serves the first companion over TCP and prints the address.\n\n" +
			"The same two things the buttons below do separately.")
	}
	imgui.SameLine()
	if running == 0 {
		textDim("no firmware running yet")
	} else {
		textDim(fmt.Sprintf("%d nodes running firmware", running))
	}

	imgui.SeparatorText("Endpoints")
	textDimWrap("Both transports carry the firmware's own serial protocol, byte for byte. " +
		"TCP for a client that speaks sockets, a virtual serial device for the many " +
		"that only know how to open a port.")

	if !imgui.BeginTableV("##compbench", 4,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsSizingStretchProp,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	imgui.TableSetupColumn("companion")
	imgui.TableSetupColumn("serve")
	imgui.TableSetupColumn("address")
	imgui.TableSetupColumn("client")
	imgui.TableHeadersRow()
	for _, i := range comps {
		a.drawBenchRow(i)
	}
	imgui.EndTable()

	a.drawBenchFaults(comps)
	if a.status != "" {
		textDimWrap(a.status)
	}
}

// companionIndices is every companion in the scenario, in order.
func (a *App) companionIndices() []int {
	var out []int
	for i := range a.Nodes {
		if a.Nodes[i].Kind == scenario.Companion {
			out = append(out, i)
		}
	}
	return out
}

func (a *App) drawBenchRow(i int) {
	name := a.Nodes[i].Name
	imgui.TableNextRow()
	imgui.TableNextColumn()
	imgui.Text(name)

	imgui.TableNextColumn()
	if imgui.ButtonV("TCP##"+name, imgui.NewVec2(60, 0)) {
		a.serveBench(name, "tcp")
	}
	imgui.SameLine()
	if imgui.ButtonV("serial##"+name, imgui.NewVec2(70, 0)) {
		a.serveBench(name, "serial")
	}

	imgui.TableNextColumn()
	link := a.comp.ports[name]
	if link == nil {
		textDim("not served")
	} else {
		imgui.Text(link.Addr)
		imgui.SameLine()
		// Copy, because the whole point of an endpoint is that it goes into
		// somebody else's configuration file, and retyping a port from a
		// screenshot is how a digit gets lost.
		if imgui.SmallButton("copy##" + name) {
			imgui.SetClipboardText(link.Addr)
			a.status = "copied " + link.Addr
		}
	}

	imgui.TableNextColumn()
	switch {
	case link == nil:
		textDim("-")
	case link.Attached():
		imgui.Text("attached")
	default:
		textDim("waiting")
	}
}

// drawBenchFaults is the part a happy-path test never reaches.
//
// Two faults, because two are what the workbench can actually cause today, and
// a button that pretends to inject a fault it cannot is worse than an absent
// one. Radio-level faults are the RF model's business: move the node, drop its
// power, or place an emitter beside it.
func (a *App) drawBenchFaults(comps []int) {
	imgui.SeparatorText("Faults")
	if imgui.Button("drop every client connection") {
		// The listener goes with the connection, so this is "the device was
		// unplugged" rather than "the link glitched". Serving again is one
		// click. Names first: closing changes what Attached() reports, so
		// deciding and deleting in one pass leaves stale entries behind.
		var drop []string
		for name, p := range a.comp.ports {
			if p.Attached() {
				drop = append(drop, name)
			}
		}
		for _, name := range drop {
			_ = a.comp.ports[name].Close()
			delete(a.comp.ports, name)
		}
		a.status = fmt.Sprintf("dropped %d client connection(s)", len(drop))
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("The device vanishes, as a USB cable does.\n\n" +
			"An application that reconnects cleanly from this is one that\n" +
			"survives a phone going to sleep.")
	}
	imgui.SameLine()
	if imgui.Button("inject a stray frame") {
		if a.eng == nil {
			a.buildEngine()
		}
		a.eng.Inject(comps[0], []byte("msim-stray"))
		a.status = "injected an unexpected frame at " + a.Nodes[comps[0]].Name
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Traffic the client did not ask for and cannot parse,\n" +
			"arriving while it is doing something else.")
	}
}

// benchStartAndServe is the one click: firmware everywhere, then an address.
func (a *App) benchStartAndServe(i int) {
	if a.eng == nil {
		a.buildEngine()
	}
	if a.eng.FirmwareCount() == 0 {
		a.startFirmware()
		// Starting is asynchronous - it spawns a process per node - so the
		// serve below would race it. Saying so beats serving a port that
		// answers nothing for the next twenty seconds.
		a.status = "starting firmware; press again once it is running"
		return
	}
	a.serveBench(a.Nodes[i].Name, "tcp")
}

func (a *App) serveBench(name, kind string) {
	if a.eng == nil {
		a.status = "no simulation yet"
		return
	}
	if n, ok := a.eng.NodeByName(name); !ok || n.Firmware == nil {
		a.status = name + " runs no firmware yet, so it has no companion interface"
		return
	}
	var link *engine.CompanionLink
	var err error
	if kind == "serial" {
		link, err = a.eng.ServeCompanionSerial(name)
	} else {
		// Port 0: the operating system picks a free one and the link reports
		// it. A fixed default collides with the last run that has not finished
		// dying, and the error for that reads like a permissions problem.
		link, err = a.eng.ServeCompanionTCP(name, "127.0.0.1:0")
	}
	if err != nil {
		a.status = err.Error()
		return
	}
	a.comp.ports[name] = link
	a.status = name + " is at " + link.Addr
}
