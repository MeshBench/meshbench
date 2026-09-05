// What a session can ask of whatever is drawing it.
//
// Three of the old socket's verbs move the interface rather than the
// simulation: show me that view, what panels are there, close. They are still
// session verbs, because a script asks for them over the same socket as
// everything else and should not have to know which binary is listening.
//
// So the verb lives here and the drawing is delegated. A workbench registers
// itself; a headless driver registers nothing and the verbs say plainly that
// there is no interface, rather than being absent and looking like a version
// mismatch.
package session

import (
	"errors"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// UI is implemented by whatever is on screen.
type UI interface {
	// ShowView switches to a named view, returning an error naming the ones
	// that exist if it does not.
	ShowView(name string) error
	// PanelNames is every panel, in any order.
	PanelNames() []string
	// Quit closes the application, stopping firmware on the way out.
	Quit()
	// CentreMap points the camera at a position. Zoom of zero leaves the
	// current scale alone, so "look here" and "look here this close" are the
	// same verb rather than two.
	CentreMap(lat, lon, zoom float64)
	// FitMap frames every node, which is the only camera request that needs
	// no numbers and is the one somebody driving a capture usually wants.
	FitMap()
	// FitMapOnOpen frames a network that has just been loaded. Separate from
	// FitMap because it is the application's idea rather than somebody's: a
	// camera a capture flag placed deliberately keeps it, where an outright
	// fit overrules everything.
	FitMapOnOpen()
	// OpenNodeWindow gives one node a window of its own, opened on a named tab
	// where one was asked for, and reports which tab it landed on.
	//
	// The tab because the Hardware pane is where a board draws itself and it
	// could be reached only by clicking - so a capture could not show it and a
	// script could not open it. What it landed on, because a node whose board
	// declares no screen, lamps or buttons grows no Hardware tab, and
	// reporting one it does not have would be worse than refusing.
	OpenNodeWindow(node, tab string) (string, error)

	// OpenBoardView gives one node's board a window of its own: what its
	// profile declares, what the firmware left in the chip, where the two
	// differ, and the controls for everything the board has wired.
	//
	// A window rather than a tab because of the move it exists for - the log
	// says something, and the question is what the hardware did - which needs
	// the log and the table visible at once.
	OpenBoardView(node, tab string) (string, error)

	// OpenFirmwareWindow gives one build a window of its own: what it is,
	// where it lives, and the settings it runs under. All three names,
	// because a label can carry more than one build and acting on the wrong
	// one is a rename of somebody else's image.
	OpenFirmwareWindow(role, version, board string) error

	// OpenOutputWindow gives one node's one log a window of its own, so a
	// board's screen and two of its logs can be watched at once.
	OpenOutputWindow(node, source string) error

	// OpenPanel shows a panel. where is "" for in the layout, "window" for
	// its own window, or "dock" to bring it back.
	OpenPanel(name, where string) error
	// ClosePanel takes a panel out of the layout, and ResetLayout puts the
	// current view back to the shape it is declared with. A layout that can
	// be changed has to be one a script can change back.
	ClosePanel(name string) error
	ResetLayout()
	// CloseWindow closes a popped-out panel's window.
	CloseWindow(name string) error

	// Scale and SetScale are the interface's own size, which somebody on a
	// high-density screen changes once and then never thinks about again.
	Scale() float64
	SetScale(v float64)

	// SaveView, LoadView, ListViews and DeleteView are named arrangements.
	SaveView(name string) error
	LoadView(name string) error
	ListViews() []string
	DeleteView(name string) error

	// ZoomMap multiplies the current scale; FilterMap dims what does not
	// match; SetTool chooses what a click on the map does.
	ZoomMap(factor float64)
	FilterMap(query string)
	SetTool(name string) error
	// SetLayer turns a map layer on or off by the name the map shows, and
	// reports the ones there are when it does not know it. A layer that can
	// only be reached by clicking cannot be reached by a script, a capture or
	// a test - which is how coverage and terrain went unchecked.
	SetLayer(name string, on bool) error
	// Layers is what is drawn right now.
	Layers() map[string]bool

	// State is what the interface is showing, for a caller that has no eyes.
	State() map[string]any
}

// SetUI attaches an interface. Safe to leave unset.
func (s *Sim) SetUI(u UI) { s.ui = u }

func (s *Sim) needUI() error {
	if s.ui == nil {
		return errNoInterface
	}
	return nil
}

// errNoInterface is a window verb in a session with no window - headless,
// almost always. Built once: it is returned from 23 verbs and carries no
// detail, so there is nothing per-call to put in it.
var errNoInterface = control.WithCode(control.Unavailable, errors.New(
	"this session has no interface attached, so there is nothing to show"))

func registerUI(st *state.Store, s *Sim) {
	st.Handle("workspace.set", func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		name := soleString(p)
		if m, ok := p.(map[string]any); ok {
			name, _ = m["view"].(string)
		}
		if err := s.ui.ShowView(name); err != nil {
			return nil, err
		}
		w.Say("showing " + name)
		return map[string]any{"view": name}, nil
	})
	st.Handle("panels.list", func(_ *state.World, _ any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		names := s.ui.PanelNames()
		return map[string]any{"panels": names, "count": len(names)}, nil
	})
	st.Handle("app.quit", func(w *state.World, _ any) (any, error) {
		w.Say("closing")
		if s.ui != nil {
			go s.ui.Quit()
			return map[string]any{"closing": true}, nil
		}
		// No interface: still stop firmware, because a headless driver that
		// asks to quit means it.
		go s.Close()
		return map[string]any{"closing": true, "headless": true}, nil
	})
}

func findNode(nodes []state.Node, name string) (state.Node, bool) {
	for _, n := range nodes {
		if n.Name == name {
			return n, true
		}
	}
	return state.Node{}, false
}

// registerUIVerbs are the ones that move the interface rather than the model.
//
// They live here, not in the workbench, so a headless driver gets the same
// vocabulary and a clear "no interface attached" rather than an absence that
// looks like a version mismatch.
//
// Four groups, in four files, because one file held all seventeen and the
// descriptions took it past the length limit: what the session is doing, the
// panels and their windows, the saved arrangements, and the map's own
// controls.
func registerUIVerbs(st *state.Store, s *Sim) {
	registerSessionState(st, s)
	registerPanelVerbs(st, s)
	registerSavedViews(st, s)
}
