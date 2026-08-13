package main

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/A13xB0/meshcoresim/internal/gui/shell"
	"github.com/A13xB0/meshcoresim/internal/session"
)

// workbenchUI is how the session moves this window.
//
// Nothing here touches the shell directly on the caller's goroutine: sh.View
// is read while drawing, so a verb sets an intention and the frame loop
// applies it. Writing it from the store's goroutine would be the kind of race
// that shows up as a torn frame once a week rather than as a failing test.
type workbenchUI struct {
	sh  *shell.Shell
	sim *session.Sim
}

var _ session.UI = (*workbenchUI)(nil)

// pendingView carries a view from the store's goroutine to the frame loop.
// Zero means nothing pending; a view is stored as its index plus one.
var pendingView atomic.Int32

func (u *workbenchUI) ShowView(name string) error {
	for i := 0; i < int(shell.NumViews); i++ {
		if strings.EqualFold(shell.View(i).String(), name) {
			pendingView.Store(int32(i) + 1)
			return nil
		}
	}
	var have []string
	for i := 0; i < int(shell.NumViews); i++ {
		have = append(have, shell.View(i).String())
	}
	return fmt.Errorf("no view %q; there is: %s", name, strings.Join(have, ", "))
}

// PanelNames reads a map that is built at startup and never written again.
func (u *workbenchUI) PanelNames() []string {
	names := make([]string, 0, len(u.sh.Panels))
	for n := range u.sh.Panels {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (u *workbenchUI) Quit() { quit(u.sim) }
