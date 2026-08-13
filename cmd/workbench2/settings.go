// Settings, and the dialog that changes them (P7, section 9).
//
// Applied live rather than at next launch. A theme somebody cannot see the
// effect of until they restart is a theme they set by trial and error, and
// density is the setting most likely to be wrong for a particular screen.
package main

import (
	"sync"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// settings is what the interface looks like, shared between windows.
//
// Guarded because the settings dialog may be its own window on its own
// goroutine, and the main frame loop reads these every frame.
type settings struct {
	mu      sync.Mutex
	mode    theme.Mode
	density theme.Density
	scale   float64
	gen     uint64
}

func newSettings(mode theme.Mode) *settings {
	return &settings{mode: mode, density: theme.Default, scale: 1, gen: 1}
}

// scale is the interface's own size. Kept beside the theme because changing it
// invalidates the same things a theme change does.
func (s *settings) getScale() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scale <= 0 {
		return 1
	}
	return s.scale
}

func (s *settings) setScale(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v > 0 && s.scale != v {
		s.scale, s.gen = v, s.gen+1
	}
}

func (s *settings) get() (theme.Mode, theme.Density, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode, s.density, s.gen
}

func (s *settings) setMode(m theme.Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != m {
		s.mode, s.gen = m, s.gen+1
	}
}

func (s *settings) setDensity(d theme.Density) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.density != d {
		s.density, s.gen = d, s.gen+1
	}
}

// settingsPanel is the dialog. A panel rather than a modal, so it can be
// popped into its own window like everything else and left open beside the
// thing it is changing - which is the only way to judge a density.
type settingsPanel struct {
	dark, light                comp.Button
	comfortable, normal, tight comp.Button
	init                       bool
	set                        *settings
}

func (p *settingsPanel) Draw(t *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.dark.Label, p.light.Label = "dark", "light"
		p.comfortable.Label = "comfortable"
		// Not "default": the zero value of Density is Comfortable, so a fresh
		// install opens on that one, and a button next to it claiming to be
		// the default is a small lie that costs somebody a minute working out
		// which of the two they are looking at.
		p.normal.Label = "standard"
		p.tight.Label = "compact"
		p.init = true
	}
	mode, density, _ := p.set.get()
	// The current choice is the primary button, so the setting is legible
	// without a separate indicator saying which one is on.
	weight := func(b *comp.Button, on bool) {
		b.Kind = comp.Secondary
		if on {
			b.Kind = comp.Primary
		}
	}
	weight(&p.dark, mode == theme.Dark)
	weight(&p.light, mode == theme.Light)
	weight(&p.comfortable, density == theme.Comfortable)
	weight(&p.normal, density == theme.Default)
	weight(&p.tight, density == theme.Compact)

	if p.dark.Click.Clicked(gtx) {
		p.set.setMode(theme.Dark)
	}
	if p.light.Click.Clicked(gtx) {
		p.set.setMode(theme.Light)
	}
	if p.comfortable.Click.Clicked(gtx) {
		p.set.setDensity(theme.Comfortable)
	}
	if p.normal.Click.Clicked(gtx) {
		p.set.setDensity(theme.Default)
	}
	if p.tight.Click.Clicked(gtx) {
		p.set.setDensity(theme.Compact)
	}

	btn := func(b *comp.Button) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return layout.Inset{Right: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(
				gtx, func(gtx layout.Context) layout.Dimensions {
					return b.Layout(t, gtx)
				})
		})
	}
	row := func(kids ...layout.FlexChild) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, kids...)
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "appearance")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"light is a design of its own, not the dark one inverted")),
		row(btn(&p.dark), btn(&p.light)),
		layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
		layout.Rigid(comp.SectionTitle(t, "density")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"how much fits on a screen, and how big a thing is to click; compact still meets the minimum hit target")),
		row(btn(&p.comfortable), btn(&p.normal), btn(&p.tight)),
		layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"changes apply immediately, in every window")),
	)
}
