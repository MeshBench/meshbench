package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
)

// A view is a whole saved arrangement: which panels are open and where every
// one of them sits, including the ones popped out to other monitors.
//
// The four workspace presets are opinions about how to work; a view is the
// operator's own. Saving one is how "the layout I like" survives a workspace
// switch, a restart, and me changing the presets underneath them.
type savedView struct {
	name  string
	saved time.Time
}

func viewDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		return "views"
	}
	return filepath.Join(base, "meshcoresim", "views")
}

func viewPath(name string) string {
	return filepath.Join(viewDir(), name+".ini")
}

// openPanelsPath is the companion file: an ini remembers geometry, not which
// panels the operator had open, and a view that restores the furniture without
// the windows is not the layout they saved.
func openPanelsPath(name string) string {
	return filepath.Join(viewDir(), name+".panels")
}

func listViews() []savedView {
	entries, err := os.ReadDir(viewDir())
	if err != nil {
		return nil
	}
	var out []savedView
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ini") {
			continue
		}
		v := savedView{name: strings.TrimSuffix(e.Name(), ".ini")}
		if info, err := e.Info(); err == nil {
			v.saved = info.ModTime()
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].saved.After(out[j].saved) })
	return out
}

func (a *App) saveView(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "view-" + time.Now().Format("2006-01-02-1504")
	}
	if err := os.MkdirAll(viewDir(), 0o755); err != nil {
		a.status = err.Error()
		return
	}
	imgui.SaveIniSettingsToDisk(viewPath(name))
	var open []string
	for _, p := range a.panelRegistry() {
		if p.open {
			open = append(open, p.name)
		}
	}
	if err := os.WriteFile(openPanelsPath(name),
		[]byte(strings.Join(open, "\n")), 0o644); err != nil {
		a.status = err.Error()
		return
	}
	a.status = fmt.Sprintf("view %q saved - %d panels", name, len(open))
}

func (a *App) loadView(name string) {
	if _, err := os.Stat(viewPath(name)); err != nil {
		a.status = err.Error()
		return
	}
	// Panels first: imgui only restores geometry for windows that are
	// submitted, so a panel that is closed when the ini loads keeps no place.
	if b, err := os.ReadFile(openPanelsPath(name)); err == nil {
		want := map[string]bool{}
		for _, n := range strings.Split(string(b), "\n") {
			if n = strings.TrimSpace(n); n != "" {
				want[n] = true
			}
		}
		for _, p := range a.panelRegistry() {
			p.open = want[p.name]
		}
	}
	imgui.LoadIniSettingsFromDisk(viewPath(name))
	// A loaded view is the operator's arrangement, not a preset: the workspace
	// rebuild must not run afterwards and overwrite it.
	a.wsRebuild = false
	a.status = "view " + name + " loaded"
}

func (a *App) deleteView(name string) {
	_ = os.Remove(viewPath(name))
	_ = os.Remove(openPanelsPath(name))
	a.status = "view " + name + " deleted"
}

// drawViewsMenu is the Views menu: save this arrangement, go back to one,
// throw one away.
func (a *App) drawViewsMenu() {
	if !imgui.BeginMenu("View") {
		return
	}
	// The four activities first: the menu and the tab strip are the same
	// thing, and a shortcut has to be discoverable somewhere.
	for w := workspace(0); w < workspaceCount; w++ {
		if imgui.MenuItemBoolV(w.String(), fmt.Sprintf("ctrl+%d", w+1), w == a.ws, true) {
			a.switchWorkspace(w)
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip(w.purpose())
		}
	}
	imgui.Separator()
	textDim("UI scale")
	if imgui.MenuItemBoolV("larger", "ctrl+=", false, true) {
		a.requestUIScale(a.uiScale * 1.1)
	}
	if imgui.MenuItemBoolV("smaller", "ctrl+-", false, true) {
		a.requestUIScale(a.uiScale / 1.1)
	}
	if imgui.MenuItemBoolV("automatic", "ctrl+0", false, true) {
		a.cfg.uiScale = 0
		a.saveConfig()
		a.requestUIScale(1)
	}
	imgui.Separator()
	textDim("saved layouts - every panel's place, popped-out ones included")
	imgui.SetNextItemWidth(200)
	imgui.InputTextWithHint("##viewname", "name this arrangement", &a.viewName, 0, nil)
	imgui.SameLine()
	if imgui.Button("save view") {
		a.saveView(a.viewName)
		a.viewName = ""
	}
	imgui.Separator()

	views := listViews()
	if len(views) == 0 {
		textDim("nothing saved yet")
	}
	for i, v := range views {
		if imgui.MenuItemBool(fmt.Sprintf("%s  -  %s##v%d", v.name, age(v.saved), i)) {
			a.loadView(v.name)
		}
		imgui.SameLine()
		// Two clicks to delete, on the same row: the first arms it. A
		// single-click delete beside a load item is a misclick with
		// consequences.
		lbl := "x"
		if a.confirmView == v.name {
			lbl = "sure?"
		}
		if imgui.SmallButton(lbl + "##dv" + fmt.Sprint(i)) {
			if a.confirmView == v.name {
				a.deleteView(v.name)
				a.confirmView = ""
			} else {
				a.confirmView = v.name
			}
		}
	}

	imgui.Separator()
	if imgui.MenuItemBool("reset this view to its preset") {
		a.wsRebuild = true
	}
	imgui.EndMenu()
}
