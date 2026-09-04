// Every firmware window that is open, and the one goroutine each of them runs.
//
// The same shape as nodeview.WindowSet and for the same reason: a window belongs to
// its own event loop, so another goroutine asking for one either opens it or
// leaves a wish for the loop to pick up. Nothing here knows what a firmware
// window looks like.
package workbench

import (
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// firmwareWindowSet tracks which builds have a window, so a second request
// raises rather than opening a duplicate.
type firmwareWindowSet struct {
	*shell.WindowRegistry
}

func newFirmwareWindowSet() *firmwareWindowSet {
	return &firmwareWindowSet{WindowRegistry: shell.NewWindowRegistry()}
}

// buildWindowKey identifies a build across all three of its names.
//
// All three, because a label alone is not unique: the same image imported for
// two boards, or one label carrying a repeater and a companion, would share a
// window and each would show the other's settings.
func buildWindowKey(role, version, board string) string {
	return role + "\x00" + version + "\x00" + board
}

func (w *firmwareWindowSet) openFor(role, version, board string,
	newTheme func() *theme.Theme, st *state.Store, do Do) {
	key := buildWindowKey(role, version, board)
	if !w.Claim(key) {
		return
	}
	p := &firmwareWindowPanel{role: role, version: version, board: board, OnDo: do}
	go shell.RunPopout(w.WindowRegistry, key, "MeshBench - "+version,
		shell.PopoutSize{W: 760, H: 680}, p, newTheme, st)
}
