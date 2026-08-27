// The panels about the nodes themselves: what is running, what it is told at
// boot, its console, its firmware, and the companions being served.
package workbench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func addMeshPanels(d panelDeps) {
	d.sh.Add(homed(&shell.Panel{Name: "Provisioning", Windowable: true,
		Draw: d.withControls(d.provCtl.Draw, shell.EmptyPanel("Provisioning",
			"what every node is told when it starts").Draw)}))
	nv := &nodeViewPanel{}
	nv.OnAction = func(action, node string) {
		if action == "nodes.stats" {
			d.do(action, nil)
			return
		}
		d.do(action, node)
	}
	nv.OnFirmware = func(node string, b buildChoice) {
		go func() {
			_, _ = d.st.Do(d.ctx, "node.set_firmware", map[string]any{
				"node": node, "version": b.Version,
				"board": b.Board, "role": b.Role})
		}()
	}
	if *d.filterFlag != "" {
		nv.SetFilter(*d.filterFlag)
	}
	openOnTab = nodeTab(*d.nodeTabFlag)
	packetOpenOnTab = *d.packetTabFlag
	if *d.nodeWinFlag != "" {
		go func() {
			time.Sleep(4 * time.Second)
			if _, err := d.st.Do(d.ctx, "node.window", *d.nodeWinFlag); err != nil {
				fmt.Fprintln(os.Stderr, "node.window:", err)
			}
		}()
	}
	if *d.provFlag != "" {
		go func() {
			if _, err := d.st.Do(d.ctx, "node.provisioning", *d.provFlag); err != nil {
				fmt.Fprintln(os.Stderr, "provisioning:", err)
			}
		}()
	}
	if *d.openFwFlag != "" {
		nv.OpenFirmware(*d.openFwFlag)
	}
	if *d.openMenuFlag != "" {
		go func() {
			// After the first snapshot, so the menu knows whether the
			// node is running and can offer stop rather than start.
			time.Sleep(30 * time.Second)
			nv.OpenMenu(*d.openMenuFlag, d.st.Snapshot())
		}()
	}
	d.sh.Add(homed(&shell.Panel{Name: "Nodes running", Windowable: true, Draw: nv.Draw}))
	// Keep the node view live while somebody is looking at it.
	//
	// Sampling costs a /proc read per node, so it is driven by the panel
	// having drawn rather than by a timer that runs whether or not the view
	// is open - and it stops as soon as the panel does.
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-d.ctx.Done():
				return
			case <-t.C:
				if nv.watched.Swap(false) {
					d.do("nodes.stats", nil)
				}
			}
		}
	}()
	fleet := &fleetPanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Fleet", Windowable: true,
		Draw: d.withControls(d.fleetCtl.Draw, fleet.Draw)}))
	boards := &boardsPanel{do: d.do}
	d.sh.Add(homed(&shell.Panel{Name: "Boards", Windowable: true, Draw: boards.Draw}))
	bench := &benchPanel{}
	// Clicking a companion is the App view's only way of saying which one
	// everything else is about: the view carries neither the map nor the
	// node list, and those were the only two things that selected anything.
	bench.OnSelect = func(node string) { d.do("nodes.select", node) }
	bench.OnAction = func(action, node string) {
		switch action {
		case "serve.tcp":
			d.do("bench.serve", map[string]any{"node": node, "kind": "tcp"})
		case "serve.serial":
			d.do("bench.serve", map[string]any{"node": node, "kind": "serial"})
		case "node.window":
			d.do("node.window", node)
		default:
			d.do(action, nil)
		}
	}
	d.sh.Add(homed(&shell.Panel{Name: "Companion bench", Windowable: true,
		Draw: d.withControls(d.benchCtl.Draw, bench.Draw)}))
	console := &consolePanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Console", Windowable: true, Draw: console.Draw}))
	fw := &firmwarePanel{choose: d.chooserIn("Firmware")}
	// The library asks for itself, and asks again after anything that changes
	// it. A panel that reads the cache directly cannot know when a download
	// has landed.
	fw.Refresh = func() {
		go func() { _, _ = d.st.Do(d.ctx, "firmware.library", nil) }()
	}
	fw.OnAction = func(verb string, params map[string]any) {
		go func() {
			if _, err := d.st.Do(d.ctx, verb, params); err != nil {
				_, _ = d.st.Do(d.ctx, "ui.said", verb+": "+err.Error())
			}
			_, _ = d.st.Do(d.ctx, "firmware.library", nil)
		}()
	}
	// Importing needs a path and a role, which a button cannot carry: ask for
	// both through the shell, then refresh so the new build appears.
	fw.OnImport = func() {
		// Four questions, all to the Firmware panel's own window: asked from a
		// pop-out, answered in the pop-out.
		fwAsk := d.wins.promptFor("Firmware", &d.sh.Ask)
		fwAsk.Post(func(ask *shell.Prompt) {
			ask.OpenPath("Import a build from", "path to a binary", "",
				shell.PathAsk{Kind: shell.PathFile, FilterName: "Firmware images",
					Extensions: []string{"bin", "uf2", "elf"}}, func(path string) {
					if strings.TrimSpace(path) == "" {
						return
					}
					fwAsk.Post(func(ask *shell.Prompt) {
						ask.Choose("Import it as which role?", "filter",
							importRoles(), func(role string) {
								// And which board it was compiled for. The verb
								// has always taken one and nothing ever asked, so
								// every build imported here became a host build -
								// which meant an image built for a board could
								// not be pointed at one, and firmware somebody
								// had just compiled could not be run at all.
								fwAsk.Post(func(ask *shell.Prompt) {
									ask.Choose("Which board was it built for?", "filter",
										importBoards(), func(board string) {
											if board == hostBuildChoice {
												board = ""
											}
											d.askImportName(fwAsk, path, role, board)
										})
								})
							})
					})
				})
		})
	}
	d.sh.Add(homed(&shell.Panel{Name: "Firmware", Windowable: true, Draw: fw.Draw}))
}

// askImportName is the last question, and the one the library is read by.
//
// Asked rather than defaulted, because the default was a timestamp: every
// import called itself imported-20260824-142530, so a library of four local
// builds said nothing about which was which and pinning one was guesswork.
// Seeded from the file's own name, so somebody who has nothing to add presses
// Enter and still gets "mesh-rs" rather than a clock reading.
func (d panelDeps) askImportName(fwAsk *shell.Prompt, path, role, board string) {
	fwAsk.Post(func(ask *shell.Prompt) {
		ask.Open("Call this build what?", "a name and version, e.g. mesh-rs 1.2.0",
			importNameFrom(path), func(label string) {
				go func() {
					p := map[string]any{"path": path, "role": role}
					if board != "" {
						p["board"] = board
					}
					if s := strings.TrimSpace(label); s != "" {
						p["label"] = s
					}
					if _, err := d.st.Do(d.ctx, "firmware.import", p); err != nil {
						_, _ = d.st.Do(d.ctx, "ui.said", "import: "+err.Error())
					}
					_, _ = d.st.Do(d.ctx, "firmware.library", nil)
				}()
			})
	})
}

// importNameFrom is what to call a build when nobody has said yet.
//
// The file's own name with the parts that are about how it was packaged taken
// off: "firmware-heltec-v3-2.7.26.factory.bin" is a Meshtastic release and
// "-merged" is how an image was assembled, neither of which is what somebody
// would call the thing in a list beside three others.
func importNameFrom(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	for _, suffix := range []string{".factory", "-merged", "-factory", ".merged"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// importRoles is what a build may be imported as.
//
// From the scenario's own list rather than a literal beside it, which had
// drifted: it offered "advanced_repeater", which is not a role anywhere else
// in the tree and produced builds the runner would never select, and it did
// not offer the companion transports at all - so a companion build could only
// be imported under the name of the transport-less role and its transport was
// then guessed downstream.
//
// Bluetooth is left out on purpose and the entry says why: an emulated node
// has no Bluetooth, so a companion_radio_ble image imported for a board is one
// that can never be run here.
func importRoles() []string {
	return []string{
		string(scenario.RoleSimpleRepeater),
		string(scenario.RoleCompanionRadioUSB),
		string(scenario.RoleCompanionRadio),
		string(scenario.RoleSimpleRoomServer),
	}
}

// hostBuildChoice is the first option, and the one most imports want: a build
// compiled for this machine rather than for a board.
const hostBuildChoice = "host build - not for a board"

// importBoards is what an imported build can be pointed at.
//
// Every board with emulation wiring, not only the ones already verified: an
// image somebody has just compiled for a board is exactly the thing that
// establishes whether that board works, so refusing to import it would make
// the list impossible to grow.
func importBoards() []string {
	out := []string{hostBuildChoice}
	for _, b := range hw.Boards() {
		if b.QEMU != nil || b.Renode != nil {
			out = append(out, b.Name)
		}
	}
	return out
}
