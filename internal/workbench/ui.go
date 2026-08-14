package workbench

import (
	"sync"

	"fmt"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/MeshBench/meshbench/internal/gui/shell"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
	"github.com/MeshBench/meshbench/internal/session"
)

// workbenchUI is how the session moves this window.
//
// Nothing here touches the shell directly on the caller's goroutine: sh.View
// is read while drawing, so a verb sets an intention and the frame loop
// applies it. Writing it from the store's goroutine would be the kind of race
// that shows up as a torn frame once a week rather than as a failing test.
type workbenchUI struct {
	sh    *shell.Shell
	sim   *session.Sim
	mv    *comp.MapView
	nodes *nodeWindows
	store *state.Store
	// newTheme gives each window a shaper of its own: Gio's is not safe for
	// concurrent use and two frame loops sharing one corrupts its glyph
	// buffer.
	newTheme func() *theme.Theme
	// onCommand and onAction carry a node window's controls back to the store.
	onCommand func(node, line string)
	onAction  func(action, node string)
	onCLI     func(node, line string)
	// onServe serves a companion to a real client; onOpenPacket opens the
	// packet view from an activity row.
	onServe      func(node, kind string)
	onOpenPacket func(id uint64)
	// dock, closeWin, scale and setScale are the pieces of the window and
	// settings machinery a verb needs to reach.
	dock     func(name string)
	closeWin func(name string) error
	scale    func() float64
	setScale func(float64)

	// camera is the next camera request, applied by the frame loop. The
	// MapView's own fields are read while drawing, so a verb must not write
	// them from the store's goroutine.
	camMu   sync.Mutex
	camWant *cameraWant
}

type cameraWant struct {
	fit            bool
	lat, lon, zoom float64
	// zoomBy multiplies the current scale rather than setting it, which is
	// what a zoom button does and what a caller without the current value can
	// ask for.
	zoomBy float64
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

func (u *workbenchUI) CentreMap(lat, lon, zoom float64) {
	u.camMu.Lock()
	defer u.camMu.Unlock()
	u.camWant = &cameraWant{lat: lat, lon: lon, zoom: zoom}
}

func (u *workbenchUI) FitMap() {
	u.camMu.Lock()
	defer u.camMu.Unlock()
	u.camWant = &cameraWant{fit: true}
}

// applyCamera runs on the frame goroutine, before the map draws.
func (u *workbenchUI) applyCamera() {
	u.camMu.Lock()
	want := u.camWant
	u.camWant = nil
	u.camMu.Unlock()
	if want == nil || u.mv == nil {
		return
	}
	if want.fit {
		u.mv.FitNext = true
		return
	}
	if want.zoomBy > 0 {
		u.mv.Zoom *= want.zoomBy
		return
	}
	u.mv.CentreLat, u.mv.CentreLon = want.lat, want.lon
	if want.zoom > 0 {
		u.mv.Zoom = want.zoom
	}
}

func (u *workbenchUI) OpenNodeWindow(node string) {
	if u.nodes == nil || u.newTheme == nil {
		return
	}
	u.nodes.openFor(node, u.newTheme, u.store, u.onCommand, u.onAction, u.onCLI,
		u.onServe, u.onOpenPacket)
}
