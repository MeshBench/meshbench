// The live arrangement of each view: what is actually on screen, as opposed
// to the shape the view starts in.
//
// Presets used to be the whole story, and the consequence was that a panel
// named in no preset could only be reached by leaving the window - twenty of
// the thirty-three - with no route back in. A region now holds tabs, docking
// puts a panel in one, and undocking takes it out again.
package shell

// regionRef addresses one region of a view: a rail entry, or a column of a
// row. Row -1 means the rail.
type regionRef struct {
	Row, Col int
}

func (r regionRef) valid() bool { return r.Col >= 0 }

// noRegion is "nowhere", for a panel that is not on screen.
var noRegion = regionRef{Row: 0, Col: -1}

// arrangement is the current view's live arrangement, seeded from its preset
// the first time it is asked for.
func (sh *Shell) arrangement() *Arrangement {
	return sh.arrangementOf(sh.View)
}

func (sh *Shell) arrangementOf(v View) *Arrangement {
	if int(v) < 0 || int(v) >= int(numViews) {
		v = Plan
	}
	if sh.layouts[v] == nil {
		a := presetFor(v).clone()
		sh.layouts[v] = &a
	}
	return sh.layouts[v]
}

// regionAt returns the region at a reference, or nil.
func (sh *Shell) regionAt(v View, r regionRef) *Col {
	a := sh.arrangementOf(v)
	if r.Col < 0 {
		return nil
	}
	if r.Row < 0 {
		if r.Col < len(a.Rail) {
			return &a.Rail[r.Col]
		}
		return nil
	}
	if r.Row < len(a.Rows) && r.Col < len(a.Rows[r.Row].Cols) {
		return &a.Rows[r.Row].Cols[r.Col]
	}
	return nil
}

// find is where a panel is in the current view, or noRegion.
func (sh *Shell) find(name string) (regionRef, int) {
	a := sh.arrangement()
	for i := range a.Rows {
		for j := range a.Rows[i].Cols {
			if k := indexOf(a.Rows[i].Cols[j].Tabs, name); k >= 0 {
				return regionRef{Row: i, Col: j}, k
			}
		}
	}
	for j := range a.Rail {
		if k := indexOf(a.Rail[j].Tabs, name); k >= 0 {
			return regionRef{Row: -1, Col: j}, k
		}
	}
	return noRegion, -1
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

// Visible reports whether a panel is on screen in the current view - in a
// region, and the tab that region is showing. A panel behind another panel's
// tab is *in* the layout but not visible, and the menu says so by ticking it
// either way: it is one click from being looked at, which is the question the
// tick answers.
func (sh *Shell) Visible(name string) bool {
	r, _ := sh.find(name)
	return r.valid()
}

// VisiblePanels is every panel in the current view's live arrangement.
func (sh *Shell) VisiblePanels() []string { return sh.arrangement().panels() }

// reveal fires a panel's OnReveal, if it has one. Called wherever a panel is
// brought into view, so "opened" means the same thing from a menu and a tab.
func (sh *Shell) reveal(name string) {
	if p := sh.Panels[name]; p != nil && p.OnReveal != nil {
		p.OnReveal()
	}
}

// Dock puts a panel on screen in the current view and shows it.
//
// Into the region last pressed, so "open the waterfall" lands where the
// operator is working rather than somewhere the layout decided. With nothing
// pressed yet it goes to the biggest flexed region, which is the one with
// room for it.
func (sh *Shell) Dock(name string) {
	if sh.Panels[name] == nil {
		return
	}
	// Already here: raise its tab rather than adding a second copy.
	if r, k := sh.find(name); r.valid() {
		if c := sh.regionAt(sh.View, r); c != nil {
			c.Active = k
		}
		sh.focus = r
		sh.reveal(name)
		return
	}
	target := sh.focus
	if sh.regionAt(sh.View, target) == nil {
		target = sh.defaultRegion()
	}
	c := sh.regionAt(sh.View, target)
	if c == nil {
		// A view whose every region was closed still has to accept a panel.
		a := sh.arrangement()
		a.Rows = append(a.Rows, Row{Weight: 1, Cols: []Col{col(name, 0)}})
		sh.focus = regionRef{Row: len(a.Rows) - 1, Col: 0}
		sh.reveal(name)
		return
	}
	c.Tabs = append(c.Tabs, name)
	c.Active = len(c.Tabs) - 1
	sh.focus = target
	sh.reveal(name)
}

// defaultRegion is where a panel goes when nothing has been pressed: the
// first flexed column of the first row, else the first region there is.
func (sh *Shell) defaultRegion() regionRef {
	a := sh.arrangement()
	for i := range a.Rows {
		for j := range a.Rows[i].Cols {
			if a.Rows[i].Cols[j].WidthDp == 0 {
				return regionRef{Row: i, Col: j}
			}
		}
	}
	if len(a.Rows) > 0 && len(a.Rows[0].Cols) > 0 {
		return regionRef{Row: 0, Col: 0}
	}
	if len(a.Rail) > 0 {
		return regionRef{Row: -1, Col: 0}
	}
	return noRegion
}

// Undock takes a panel off screen in the current view.
//
// The region stays even when its last tab goes: an empty region draws as an
// invitation to put something in it, which is a better answer than a layout
// that silently reflows every time something is closed.
func (sh *Shell) Undock(name string) {
	r, k := sh.find(name)
	if !r.valid() {
		return
	}
	c := sh.regionAt(sh.View, r)
	if c == nil {
		return
	}
	c.Tabs = append(c.Tabs[:k], c.Tabs[k+1:]...)
	if c.Active >= len(c.Tabs) {
		c.Active = len(c.Tabs) - 1
	}
	if c.Active < 0 {
		c.Active = 0
	}
}

// ResetLayout puts a view back to the shape it is declared with.
func (sh *Shell) ResetLayout(v View) {
	a := presetFor(v).clone()
	sh.layouts[v] = &a
	sh.focus = noRegion
}

// SaveLayouts is every view's live arrangement, for persisting between runs.
func (sh *Shell) SaveLayouts() map[string]Arrangement {
	out := map[string]Arrangement{}
	for i := 0; i < int(numViews); i++ {
		if sh.layouts[i] != nil {
			out[View(i).String()] = sh.layouts[i].clone()
		}
	}
	return out
}

// LoadLayouts restores what SaveLayouts wrote. A name that is no longer a
// view is ignored rather than refused: a layout file older than a rename
// should cost the layout, not the launch.
func (sh *Shell) LoadLayouts(in map[string]Arrangement) {
	for i := 0; i < int(numViews); i++ {
		a, ok := in[View(i).String()]
		if !ok {
			continue
		}
		// Panels that no longer exist are dropped here rather than drawn as
		// "not built yet" forever.
		cleaned := a.clone()
		cleaned.dropUnknown(sh.Panels)
		sh.layouts[i] = &cleaned
	}
}

// dropUnknown removes tabs naming panels this build does not have.
func (a *Arrangement) dropUnknown(known map[string]*Panel) {
	keep := func(c *Col) {
		var tabs []string
		for _, n := range c.Tabs {
			if known[n] != nil {
				tabs = append(tabs, n)
			}
		}
		c.Tabs = tabs
		if c.Active >= len(c.Tabs) {
			c.Active = len(c.Tabs) - 1
		}
		if c.Active < 0 {
			c.Active = 0
		}
	}
	for i := range a.Rows {
		for j := range a.Rows[i].Cols {
			keep(&a.Rows[i].Cols[j])
		}
	}
	for j := range a.Rail {
		keep(&a.Rail[j])
	}
}
