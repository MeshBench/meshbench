// The Configuration panel's interface cards, the shared field row, and
// the small formatters every card leans on. Split from configcards.go at
// the file limit.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// interfaceCards is the interface's own settings: theme, density, scale.
// Applied live, in every window - a theme somebody cannot see the effect of
// until they restart is a theme they set by trial and error.
func (p *configPanel) interfaceCards(t *theme.Theme, s *state.Snapshot) []layout.Widget {
	if p.sets == nil {
		return []layout.Widget{comp.Card(t, "Interface",
			comp.Text(t, t.Sz.Caption, t.P.Faint, "no interface settings here"))}
	}
	mode, density, _ := p.sets.get()
	p.themeDD.Value = "Dark"
	if mode == theme.Light {
		p.themeDD.Value = "Light"
	}
	p.themeDD.OnOpen = func() {
		if p.choose == nil {
			return
		}
		p.choose("Theme", []string{"Dark", "Light"}, func(picked string) {
			m := theme.Dark
			if picked == "Light" {
				m = theme.Light
			}
			p.sets.setMode(m)
		})
	}
	switch density {
	case theme.Comfortable:
		p.densityDD.Value = "Comfortable"
	case theme.Compact:
		p.densityDD.Value = "Compact"
	default:
		p.densityDD.Value = "Standard"
	}
	p.densityDD.OnOpen = func() {
		if p.choose == nil {
			return
		}
		p.choose("Density", []string{"Comfortable", "Standard", "Compact"},
			func(picked string) {
				switch picked {
				case "Comfortable":
					p.sets.setDensity(theme.Comfortable)
				case "Compact":
					p.sets.setDensity(theme.Compact)
				default:
					p.sets.setDensity(theme.Default)
				}
			})
	}
	return []layout.Widget{
		comp.Card(t, "Appearance", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: t.Sp.M}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return p.themeDD.Layout(t, gtx)
								})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return p.densityDD.Layout(t, gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Faint,
							"light is a design of its own, not the dark one inverted; "+
								"density trades what fits on screen against how big a "+
								"thing is to click - changes apply immediately, in every window"))
				}),
			)
		}),
		comp.Card(t, "Scale", p.fieldRow(t, &p.scale, &p.setScale,
			"the whole interface's size; for a screen whose pixels and viewing "+
				"distance disagree with the platform's guess")),
	}
}

// fieldRow is a field, its button and the reason underneath - the shape every
// settable value here takes.
func (p *configPanel) fieldRow(t *theme.Theme, f *comp.Field, b *comp.Button, why string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.End}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(280))
						return f.Layout(t, gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Only the field that wants a directory gets a browse
						// button; the others take numbers.
						if f != &p.cacheDir {
							return layout.Dimensions{}
						}
						return layout.Inset{Left: t.Sp.S, Bottom: t.Sp.XXS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return p.browseCache.Layout(t, gtx)
							})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// Nil means the row shares a button with its
						// neighbours - the realism knobs apply as one.
						if b == nil {
							return layout.Dimensions{}
						}
						return layout.Inset{Left: t.Sp.S, Bottom: t.Sp.XXS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return b.Layout(t, gtx)
							})
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, t.P.Faint, why))
			}),
		)
	}
}

// gpuNote is why the last warm did not use the GPU, when it did not.
func gpuNote(g state.GPUState) string {
	switch {
	case !g.Present:
		return "no graphics device: " + g.Why
	case g.Enabled && g.Why != "":
		return "the last warm used the processor: " + g.Why
	case g.Enabled:
		return "the kernel is held to its processor twin by an equivalence " +
			"test, and refuses a grid too coarse to be the same answer"
	}
	return "off: every link is measured on the processor, across every core"
}

func gpuOrCPU(g state.GPUState) string {
	if g.Used {
		return "the GPU"
	}
	return "the processor"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orUnset(s string) string {
	if s == "" {
		return "the default cache directory"
	}
	return s
}

func trim0(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%d", int(f))
	}
	return fmt.Sprintf("%g", f)
}

// cacheGB is the bound as the session reports it, or the default before it
// has said anything.
func cacheGB(s *state.Snapshot) float64 {
	if s != nil && s.TileCacheGB > 0 {
		return s.TileCacheGB
	}
	return 10
}

// pickResult is a browse answer on its way back to the frame loop.
type pickResult struct {
	path string
	err  error
}
