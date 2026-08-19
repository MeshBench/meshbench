// Every panel the workbench offers, and the startup flags that open one on
// something.
//
// Lifted out of Run, which had grown past a thousand lines with this block the
// largest thing in it. The registrations themselves live in one file per
// family - panelsmap, panelssim, panelsstudy, panelsmesh, panelsapp - so each
// can be read end to end; this file is the wiring they share.
package workbench

import (
	"context"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// panelDeps is what the panel list needs from Run.
//
// A struct rather than thirty parameters. Everything in it is built by Run and
// borrowed here, never owned: this is wiring, not a component.
type panelDeps struct {
	sh  *shell.Shell
	st  *state.Store
	ctx context.Context
	mv  *comp.MapView
	// wbUI, wins and mapTop are the window and map machinery a panel reaches
	// to open a node window, post a prompt, or read the map's own toolbar.
	wbUI   *workbenchUI
	wins   *windows
	mapTop *mapTools
	nodes  *nodesPanel
	// do runs a verb and puts any failure in the status bar. withControls
	// puts an action bar above a panel's body, chooserIn posts a chooser in
	// whichever window the panel is currently in, and openPacket opens the
	// packet view on an id.
	do           Do
	withControls func(ctrl, body func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions) func(*theme.Theme, layout.Context, *state.Snapshot) layout.Dimensions
	chooserIn    func(panel string) func(string, []string, func(string))
	openPacket   func(id uint64)
	// The action bars, one per panel that has one.
	fleetCtl  *fleetControls
	schedCtl  *scheduleControls
	importCtl *importControls
	boundCtl  *boundaryControls
	validCtl  *validateControls
	planCtl   *planningControls
	benchCtl  *benchControls
	feedCtl   *feedControls
	sweepCtl  *sweepControls
	provCtl   *provisioningControls
	// The flags that ask for a panel to be open, or open on something, at
	// startup - so a screenshot can be driven rather than clicked.
	cfgSection, licSection            *string
	filterFlag, importFlag            *string
	nodeWinFlag, provFlag, openFwFlag *string
	openMenuFlag                      *string
	packetTabFlag, nodeTabFlag        *int
}

// addPanels registers every panel.
//
// It hands back the Configuration panel because Run keeps wiring it after
// this returns - the settings page and the menu both reach into it.
func addPanels(d panelDeps) *configPanel {
	addMapPanels(d)
	addSimPanels(d)
	addStudyPanels(d)
	cfg := addAppPanels(d)
	addMeshPanels(d)
	return cfg
}
