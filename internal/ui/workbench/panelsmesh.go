// The panels about the nodes themselves: what is running, what it is told at
// boot, its console, its firmware, and the companions being served.
package workbench

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/ui/shell"
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
	nv.OnFirmware = func(node, version string) {
		go func() {
			_, _ = d.st.Do(d.ctx, "node.set_firmware",
				map[string]any{"node": node, "version": version})
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
	bench := &benchPanel{}
	bench.OnAction = func(action, node string) {
		go func() {
			switch action {
			case "serve.tcp":
				_, _ = d.st.Do(d.ctx, "bench.serve",
					map[string]any{"node": node, "kind": "tcp"})
			case "serve.serial":
				_, _ = d.st.Do(d.ctx, "bench.serve",
					map[string]any{"node": node, "kind": "serial"})
			default:
				_, _ = d.st.Do(d.ctx, action, nil)
			}
		}()
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
		// Both questions go to the Firmware panel's own window: asked from a
		// pop-out, answered in the pop-out.
		fwAsk := d.wins.promptFor("Firmware", &d.sh.Ask)
		fwAsk.Post(func(ask *shell.Prompt) {
			ask.OpenPath("Import a build from", "path to a binary", "",
				shell.PathAsk{Kind: shell.PathFile}, func(path string) {
					if strings.TrimSpace(path) == "" {
						return
					}
					fwAsk.Post(func(ask *shell.Prompt) {
						ask.Choose("Import it as which role?", "filter", []string{
							"simple_repeater", "advanced_repeater", "companion_radio",
							"simple_room_server",
						}, func(role string) {
							go func() {
								if _, err := d.st.Do(d.ctx, "firmware.import",
									map[string]any{"path": path, "role": role}); err != nil {
									_, _ = d.st.Do(d.ctx, "ui.said", "import: "+err.Error())
								}
								_, _ = d.st.Do(d.ctx, "firmware.library", nil)
							}()
						})
					})
				})
		})
	}
	d.sh.Add(homed(&shell.Panel{Name: "Firmware", Windowable: true, Draw: fw.Draw}))
}
