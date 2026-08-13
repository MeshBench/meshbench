// What the interface can be asked to do.
//
// Each of these is a verb the old control socket answered, so a script written
// against that build keeps working. They are here rather than in the session
// package because they are about this window; the session owns the vocabulary
// and delegates the doing.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/A13xB0/meshcoresim/internal/gui/shell"
)

func (u *workbenchUI) OpenPanel(name, where string) error {
	if _, ok := u.sh.Panels[name]; !ok {
		return fmt.Errorf("no panel %q; there is: %s", name,
			strings.Join(u.PanelNames(), ", "))
	}
	switch where {
	case "window":
		if u.sh.OnPopOut != nil {
			u.sh.OnPopOut(name)
		}
	case "dock":
		if u.dock != nil {
			u.dock(name)
		}
	}
	// Showing a panel means showing the view that contains it, because a
	// panel that is "open" in a view nobody is looking at has not been shown.
	for v := 0; v < int(shell.NumViews); v++ {
		for _, n := range shell.PanelsIn(shell.View(v)) {
			if n == name {
				pendingView.Store(int32(v) + 1)
				return nil
			}
		}
	}
	return nil
}

func (u *workbenchUI) CloseWindow(name string) error {
	if u.closeWin == nil {
		return fmt.Errorf("no window manager")
	}
	return u.closeWin(name)
}

func (u *workbenchUI) Scale() float64 {
	if u.scale == nil {
		return 1
	}
	return u.scale()
}

func (u *workbenchUI) SetScale(v float64) {
	if u.setScale != nil {
		u.setScale(v)
	}
}

// Views are named arrangements, kept beside the projects so they survive a
// restart. A view nobody can reload is a layout somebody has to rebuild.
func viewsDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "meshcoresim", "views"), nil
}

type savedView struct {
	View int `json:"view"`
	// Popped is which panels were in their own windows, so loading a view
	// restores the second monitor as well as the first.
	Popped []string `json:"popped"`
}

func (u *workbenchUI) SaveView(name string) error {
	if name == "" {
		return fmt.Errorf("a view needs a name")
	}
	dir, err := viewsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	v := savedView{View: int(u.sh.View)}
	for _, n := range u.PanelNames() {
		if u.sh.PoppedOut != nil && u.sh.PoppedOut(n) {
			v.Popped = append(v.Popped, n)
		}
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".json"), b, 0o644)
}

func (u *workbenchUI) LoadView(name string) error {
	dir, err := viewsDir()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return fmt.Errorf("no saved view %q", name)
	}
	var v savedView
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.View >= 0 && v.View < int(shell.NumViews) {
		pendingView.Store(int32(v.View) + 1)
	}
	for _, n := range v.Popped {
		if u.sh.OnPopOut != nil {
			u.sh.OnPopOut(n)
		}
	}
	return nil
}

func (u *workbenchUI) ListViews() []string {
	dir, err := viewsDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(out)
	return out
}

func (u *workbenchUI) DeleteView(name string) error {
	dir, err := viewsDir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name+".json")); err != nil {
		return fmt.Errorf("no saved view %q", name)
	}
	return nil
}

func (u *workbenchUI) ZoomMap(factor float64) {
	u.camMu.Lock()
	defer u.camMu.Unlock()
	u.camWant = &cameraWant{zoomBy: factor}
}

func (u *workbenchUI) FilterMap(query string) {
	if u.mv != nil {
		u.mv.Filter = query
	}
}

func (u *workbenchUI) SetTool(name string) error {
	if u.mv == nil {
		return fmt.Errorf("no map")
	}
	switch name {
	case "select", "move", "place", "link", "measure":
		u.mv.Tool = name
		return nil
	}
	return fmt.Errorf("no tool %q; there is select, move, place, link and measure", name)
}

func (u *workbenchUI) State() map[string]any {
	popped := []string{}
	for _, n := range u.PanelNames() {
		if u.sh.PoppedOut != nil && u.sh.PoppedOut(n) {
			popped = append(popped, n)
		}
	}
	return map[string]any{
		"view":   u.sh.View.String(),
		"popped": popped,
		"scale":  u.Scale(),
		"tool":   u.toolName(),
	}
}

func (u *workbenchUI) toolName() string {
	if u.mv == nil || u.mv.Tool == "" {
		return "select"
	}
	return u.mv.Tool
}
