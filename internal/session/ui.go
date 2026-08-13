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
	"fmt"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
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
}

// SetUI attaches an interface. Safe to leave unset.
func (s *Sim) SetUI(u UI) { s.ui = u }

func (s *Sim) needUI() error {
	if s.ui == nil {
		return fmt.Errorf("this session has no interface attached, so there is nothing to show")
	}
	return nil
}

func registerUI(st *state.Store, s *Sim) {
	st.Handle("workspace.set", func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		name, _ := p.(string)
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
