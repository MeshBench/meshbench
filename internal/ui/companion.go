package ui

import (
	"fmt"
	"net"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/engine"
)

// companionPorts is the set of listening companion sockets, keyed by node.
type companionState struct {
	ports map[string]*engine.CompanionLink
	// port is the number typed per node, so ten companions get ten ports and
	// each is remembered where it was set.
	port map[string]int32
	// lan opens the port on every interface rather than loopback. Off by
	// default and per node: a simulator that binds 0.0.0.0 without being asked
	// is a surprise, and a phone that cannot reach loopback is the whole reason
	// anyone would ask.
	lan map[string]bool
}

// firstCompanionPort is where the numbering starts. Above the privileged range
// and clear of MeshCore's own conventions.
const firstCompanionPort = 5680

// drawCompanionTab exposes one companion node over TCP.
//
// A phone app or meshcore-cli attaches to this exactly as it would to a USB
// device, because what crosses the socket is the companion_radio application's
// own serial frames — not a translation of them.
func (a *App) drawCompanionTab(name string) {
	if a.comp.ports == nil {
		a.comp.ports = map[string]*engine.CompanionLink{}
		a.comp.port = map[string]int32{}
		a.comp.lan = map[string]bool{}
	}
	if a.comp.port[name] == 0 {
		a.comp.port[name] = int32(firstCompanionPort + len(a.comp.port))
	}

	if p, live := a.comp.ports[name]; live {
		switch p.Kind {
		case "serial":
			imgui.Text("virtual serial device")
			imgui.SameLine()
			imgui.Text(p.Addr)
			textDim("point client software at this device; it cannot tell this from USB")
		default:
			imgui.Text("listening on " + p.Addr)
			// The address a client is actually pointed at. "0.0.0.0:5680" is
			// where it is bound, not somewhere anybody can type.
			if strings.HasPrefix(p.Addr, "[::]") || strings.HasPrefix(p.Addr, "0.0.0.0") {
				_, bound, _ := net.SplitHostPort(p.Addr)
				for _, h := range lanAddresses() {
					textDim("   " + net.JoinHostPort(h, bound))
				}
			}
			textDim("point a MeshCore client at this address")
		}
		imgui.Spacing()
		if imgui.Button("disconnect") {
			_ = p.Close()
			delete(a.comp.ports, name)
		}
		return
	}

	if a.eng == nil {
		return
	}
	n, ok := a.eng.NodeByName(name)
	if !ok || n.Firmware == nil {
		textWrap("This node runs no firmware, so there is no companion interface to " +
			"expose. Start firmware from the Simulation menu first.")
		return
	}

	port := a.comp.port[name]
	imgui.SetNextItemWidth(110)
	if imgui.InputIntV("port", &port, 0, 0, 0) {
		// Clamped rather than validated on submit: an out-of-range port is a
		// typo, and refusing it after the click is a worse way to say so.
		if port < 1024 {
			port = 1024
		}
		if port > 65535 {
			port = 65535
		}
		a.comp.port[name] = port
	}
	imgui.SameLine()
	lan := a.comp.lan[name]
	if imgui.Checkbox("reachable from the network", &lan) {
		a.comp.lan[name] = lan
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Off: loopback only, for a client on this machine.\n" +
			"On: every interface, so a phone or another computer can connect.")
	}

	imgui.SameLine()
	if imgui.Button("TCP") {
		host := "127.0.0.1"
		if lan {
			// Every interface. Deliberate and per node, because this puts a
			// node's command interface on the network with no authentication —
			// which is exactly what a USB cable does, and exactly why it should
			// be asked for rather than assumed.
			host = ""
		}
		p, err := a.eng.ServeCompanionTCP(name, fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			a.status = err.Error()
			return
		}
		a.comp.ports[name] = p
		a.status = name + " is listening on " + p.Addr
	}

	// Two transports, because clients differ. Some speak TCP; a great many only
	// know how to open a serial port, and telling them to use a socket instead
	// is telling them to use different software.
	if imgui.Button("virtual serial device") {
		p, err := a.eng.ServeCompanionSerial(name)
		if err != nil {
			a.status = err.Error()
			return
		}
		a.comp.ports[name] = p
		a.status = name + " is at " + p.Addr
	}
	textDim("Both carry the firmware's own serial protocol, byte for byte.")
}

// closeCompanions stops every listener, for when the engine is rebuilt.
func (a *App) closeCompanions() {
	for name, p := range a.comp.ports {
		_ = p.Close()
		delete(a.comp.ports, name)
	}
}

// lanAddresses lists this machine's usable addresses, so a phone can be told
// where to connect rather than being handed "0.0.0.0".
func lanAddresses() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var out []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			out = append(out, ip4.String())
		}
	}
	return out
}

// companionAttached reports whether any companion link has a client right now.
//
// The ADR-0008 pin keys off this rather than off "a listener exists": the pin
// engages the moment a client attaches to an already-open port — the case the
// pipe rewrite dropped — and releases when the client goes away.
func (a *App) companionAttached() bool {
	for _, p := range a.comp.ports {
		if p.Attached() {
			return true
		}
	}
	return false
}
