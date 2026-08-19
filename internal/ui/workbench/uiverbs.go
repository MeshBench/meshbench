// What the interface can be asked to do.
//
// Each of these is a verb the old control socket answered, so a script written
// against that build keeps working. They are here rather than in the session
// package because they are about this window; the session owns the vocabulary
// and delegates the doing.
package workbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/ui/shell"
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
		return nil
	case "dock":
		if u.dock != nil {
			u.dock(name)
		}
	}
	// Asked for with no destination: put it in the layout here.
	//
	// This used only to switch to whichever view happened to declare the
	// panel, and returned silently doing nothing at all for the twenty that
	// no view declared - "open the waterfall" was a verb that could not.
	// Switching first, so the panel lands in the view it belongs to when it
	// has one, and in the one on screen when it does not.
	for v := 0; v < int(shell.NumViews); v++ {
		for _, n := range shell.PanelsIn(shell.View(v)) {
			if n == name {
				pendingView.Store(int32(v) + 1)
				u.sh.View = shell.View(v)
				u.sh.Dock(name)
				return nil
			}
		}
	}
	u.sh.Dock(name)
	return nil
}

func (u *workbenchUI) ClosePanel(name string) error {
	if _, ok := u.sh.Panels[name]; !ok {
		return fmt.Errorf("no panel %q; there is: %s", name,
			strings.Join(u.PanelNames(), ", "))
	}
	if !u.sh.Visible(name) {
		return fmt.Errorf("%s is not in this view's layout", name)
	}
	u.sh.Undock(name)
	return nil
}

func (u *workbenchUI) ResetLayout() { u.sh.ResetLayout(u.sh.View) }

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
	// Layouts is each view's arrangement: which panels are docked where, and
	// which tab each region is showing. Absent in a file written before
	// docking existed, which then loads as "the presets" rather than as an
	// error - a layout file older than a feature should cost the layout, not
	// the launch.
	Layouts map[string]shell.Arrangement `json:"layouts,omitempty"`
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
	v := savedView{View: int(u.sh.View), Layouts: u.sh.SaveLayouts()}
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
	if len(v.Layouts) > 0 {
		u.sh.LoadLayouts(v.Layouts)
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

// layerFields maps what the map calls a layer to the flag behind it, so the
// verb, the panel and the key all name the same things.
func (u *workbenchUI) layerFields() map[string]*bool {
	if u.mv == nil {
		return nil
	}
	l := &u.mv.Layers
	return map[string]*bool{
		"basemap": &l.Basemap, "boundaries": &l.Boundaries,
		"links": &l.Links, "nodes": &l.Nodes, "labels": &l.Labels,
		"traffic": &l.Traffic, "coverage": &l.Coverage, "terrain": &l.Terrain,
		"regions": &l.Regions, "antenna": &l.Pattern, "measure": &l.Measure,
	}
}

func (u *workbenchUI) SetLayer(name string, on bool) error {
	fields := u.layerFields()
	if fields == nil {
		return fmt.Errorf("no map")
	}
	want := strings.ToLower(strings.TrimSpace(name))
	f, ok := fields[want]
	if !ok {
		names := make([]string, 0, len(fields))
		for n := range fields {
			names = append(names, n)
		}
		sort.Strings(names)
		return fmt.Errorf("no layer %q; there is %s", name, strings.Join(names, ", "))
	}
	was := *f
	*f = on
	// Turning one on can be a request for something that has to be computed.
	// The map says so rather than computing it, and so does this.
	if on && !was && u.mv.OnLayerOn != nil {
		u.mv.OnLayerOn(strings.ToUpper(want[:1]) + want[1:])
	}
	return nil
}

func (u *workbenchUI) Layers() map[string]bool {
	out := map[string]bool{}
	for name, f := range u.layerFields() {
		out[name] = *f
	}
	return out
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
