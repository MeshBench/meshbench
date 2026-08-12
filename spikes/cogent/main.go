// The Plan view, in Cogent Core.
//
// Cogent Core is retained mode: a tree of widgets with CSS-like styling, laid
// out by the framework. The interesting comparisons against an immediate mode
// toolkit are that the layout is declared once rather than every frame, that
// styling is a function on the widget rather than a push and pop discipline,
// and that a table knows its rows rather than being redrawn from a slice.
package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"

	"cogentcore.org/core/colors"
	"cogentcore.org/core/core"
	"cogentcore.org/core/events"
	"cogentcore.org/core/paint"
	"cogentcore.org/core/styles"
	"cogentcore.org/core/styles/units"
)

var (
	cBg     = color.RGBA{0x0f, 0x12, 0x15, 0xff}
	cPanel  = color.RGBA{0x17, 0x1b, 0x20, 0xff}
	cMap    = color.RGBA{0x0b, 0x0e, 0x11, 0xff}
	cInk    = color.RGBA{0xe6, 0xe9, 0xee, 0xff}
	cDim    = color.RGBA{0x9a, 0xa4, 0xb2, 0xff}
	cRule   = color.RGBA{0x2a, 0x30, 0x38, 0xff}
	cAccent = color.RGBA{0x6e, 0xa8, 0xfe, 0xff}
	cWarn   = color.RGBA{0xf0, 0xb4, 0x29, 0xff}
)

var (
	sc       *scene
	selected = 0
	filter   = ""
)

func main() {
	fixture := "fixtures/fixture-fife-strict.json"
	if len(os.Args) > 1 {
		fixture = os.Args[1]
	}
	sc = loadScene(fixture)

	b := core.NewBody("MeshBench - Plan (Cogent Core spike)")
	b.Styler(func(s *styles.Style) {
		s.Direction = styles.Column
		s.Background = colors.Uniform(cBg)
		s.Grow.Set(1, 1)
	})

	menuBar(b)
	viewBar(b)
	toolBar(b)

	// The middle: map on the left, panels on the right. Two docked panels,
	// because that is the right answer for a node list beside a map, and one
	// button that takes the inspector out into its own window when it is not.
	mid := core.NewFrame(b)
	mid.Styler(func(s *styles.Style) {
		s.Direction = styles.Row
		s.Grow.Set(1, 1)
	})
	mapPanel(mid)
	side := core.NewFrame(mid)
	side.Styler(func(s *styles.Style) {
		s.Direction = styles.Column
		s.Min.X.Dp(340)
		s.Max.X.Dp(340)
		s.Grow.Set(0, 1)
		s.Background = colors.Uniform(cPanel)
		s.Border.Width.Left.Dp(1)
		s.Border.Color.Left = colors.Uniform(cRule)
	})
	nodesPanel(side)
	inspectorPanel(side)

	statusBar(b)
	b.RunMainWindow()
}

func menuBar(par core.Widget) {
	f := row(par, 34)
	for _, m := range []string{"File", "View", "Simulation", "Repeaters", "Planning", "Window", "Help"} {
		txt(f, m, 13, cDim)
	}
	grow(f)
	txt(f, "results are a best case: no multipath, bare earth, ideal demodulator", 12, cWarn)
}

func viewBar(par core.Widget) {
	f := row(par, 42)
	for _, v := range []string{"Plan", "Run", "Debug", "Verify", "Bench", "App"} {
		v := v
		bt := core.NewButton(f).SetText(v)
		bt.Styler(func(s *styles.Style) {
			s.Font.Size.Dp(13)
			s.Padding.Set(units.Dp(6), units.Dp(14))
			if v == "Plan" {
				s.Background = colors.Uniform(cAccent)
				s.Color = colors.Uniform(cBg)
			} else {
				s.Background = colors.Uniform(color.RGBA{0, 0, 0, 0})
				s.Color = colors.Uniform(cDim)
			}
		})
	}
	grow(f)
	txt(f, fmt.Sprintf("%d nodes   %d links   seed 9001", len(sc.Nodes), len(sc.Links)), 12.5, cDim)
}

func toolBar(par core.Widget) {
	f := row(par, 36)
	txt(f, "play", 13, cAccent)
	txt(f, "step", 13, cDim)
	txt(f, "reset", 13, cDim)
	txt(f, "real firmware", 13, cInk)
	txt(f, "1x", 13, cDim)
	txt(f, "t = 0.00 s", 13, cDim)
	grow(f)
	txt(f, "EU/UK (Narrow)  869.618 MHz  SF8", 12, cDim)
}

// mapPanel draws the network with the framework's painter. Retained mode does
// not mean it cannot draw: a custom widget owns its own render, and it is only
// asked to repaint when something changes.
func mapPanel(par core.Widget) {
	cv := core.NewCanvas(par)
	cv.Styler(func(s *styles.Style) {
		s.Grow.Set(1, 1)
		s.Background = colors.Uniform(cMap)
	})
	// A Cogent Core canvas is normalised: points are on a 0 to 1 scale rather
	// than in pixels, so the scene is projected into a nominal box and divided
	// through. It means the drawing survives a resize with no work.
	cv.SetDraw(func(pc *paint.Painter) {
		const W, H = 1000.0, 700.0
		sc.project(W, H, 40)
		nx := func(x float32) float32 { return x / W }
		ny := func(y float32) float32 { return y / H }
		for _, l := range sc.Links {
			a, b := sc.Nodes[l.A], sc.Nodes[l.B]
			pc.Stroke.Color = colors.Uniform(color.RGBA{0x6e, 0xa8, 0xfe, 0x40})
			pc.Stroke.Width.Dp(1)
			pc.Line(nx(a.X), ny(a.Y), nx(b.X), ny(b.Y))
			pc.Draw()
		}
		for i, n := range sc.Nodes {
			r, g, bb := kindColour(n.Kind)
			pc.Stroke.Color = colors.Uniform(color.RGBA{0, 0, 0, 0})
			pc.Fill.Color = colors.Uniform(color.RGBA{r, g, bb, 0xff})
			rad := float32(0.006)
			if i == selected {
				rad = 0.009
			}
			pc.Circle(nx(n.X), ny(n.Y), rad)
			pc.Draw()
		}
	})
}


func nodesPanel(par core.Widget) {
	f := core.NewFrame(par)
	f.Styler(func(s *styles.Style) {
		s.Direction = styles.Column
		s.Grow.Set(1, 1.6)
		s.Padding.Set(units.Dp(10))
		s.Overflow.Y = styles.OverflowAuto
	})
	title(f, "Nodes")
	tf := core.NewTextField(f).SetPlaceholder("filter by name or kind")
	tf.Styler(func(s *styles.Style) { s.Font.Size.Dp(13) })
	rows := core.NewFrame(f)
	rows.Styler(func(s *styles.Style) {
		s.Direction = styles.Column
		s.Grow.Set(1, 1)
		s.Overflow.Y = styles.OverflowAuto
	})
	build := func() {
		rows.DeleteChildren()
		want := strings.ToLower(strings.TrimSpace(filter))
		for i := range sc.Nodes {
			n := sc.Nodes[i]
			if want != "" && !strings.Contains(strings.ToLower(n.Name), want) &&
				!strings.Contains(strings.ToLower(n.Kind), want) {
				continue
			}
			i := i
			r := core.NewFrame(rows)
			r.Styler(func(s *styles.Style) {
				s.Direction = styles.Row
				s.Padding.Set(units.Dp(4))
				s.Align.Items = styles.Center
				if i == selected {
					s.Background = colors.Uniform(color.RGBA{0x23, 0x2c, 0x2a, 0xff})
				}
			})
			cr, cg, cb := kindColour(n.Kind)
			sw := core.NewFrame(r)
			sw.Styler(func(s *styles.Style) {
				s.Min.Set(units.Dp(8))
				s.Background = colors.Uniform(color.RGBA{cr, cg, cb, 0xff})
				s.Margin.Right.Dp(8)
			})
			txt(r, n.Name, 12.5, cInk)
			grow(r)
			txt(r, shortKind(n.Kind), 11, cDim)
			r.OnClick(func(e events.Event) {
				selected = i
				par.AsWidget().Scene.Update()
			})
		}
	}
	build()
	tf.OnChange(func(e events.Event) {
		filter = tf.Text()
		build()
		rows.Update()
	})
}

func inspectorPanel(par core.Widget) {
	f := core.NewFrame(par)
	f.Styler(func(s *styles.Style) {
		s.Direction = styles.Column
		s.Grow.Set(1, 1)
		s.Padding.Set(units.Dp(10))
		s.Border.Width.Top.Dp(1)
		s.Border.Color.Top = colors.Uniform(cRule)
	})
	head := core.NewFrame(f)
	head.Styler(func(s *styles.Style) { s.Direction = styles.Row; s.Align.Items = styles.Center })
	title(head, "Inspector")
	grow(head)
	pop := core.NewButton(head).SetText("open in its own window")
	pop.Styler(func(s *styles.Style) {
		s.Font.Size.Dp(11.5)
		s.Background = colors.Uniform(color.RGBA{0, 0, 0, 0})
		s.Color = colors.Uniform(cAccent)
	})
	body := core.NewFrame(f)
	body.Styler(func(s *styles.Style) { s.Direction = styles.Column; s.Grow.Set(1, 1) })
	fillInspector(body)

	// A real second window, from the same widget tree code. No docking
	// framework in the middle: the panel is simply built into another scene.
	pop.OnClick(func(e events.Event) {
		w := core.NewBody("Inspector - MeshBench")
		w.Styler(func(s *styles.Style) {
			s.Direction = styles.Column
			s.Background = colors.Uniform(cPanel)
			s.Padding.Set(units.Dp(14))
		})
		fillInspector(w)
		w.RunWindow()
	})
}

func fillInspector(par core.Widget) {
	if selected < 0 || selected >= len(sc.Nodes) {
		txt(par, "nothing selected", 12, cDim)
		return
	}
	n := sc.Nodes[selected]
	for _, kv := range [][2]string{
		{"name", n.Name},
		{"kind", shortKind(n.Kind)},
		{"latitude", fmt.Sprintf("%.5f", n.Lat)},
		{"longitude", fmt.Sprintf("%.5f", n.Lon)},
		{"height", fmt.Sprintf("%.0f m above ground", n.Height)},
		{"transmit power", fmt.Sprintf("%.0f dBm", n.TxDBm)},
		{"regions", strings.Join(n.Regions, ", ")},
	} {
		v := kv[1]
		if v == "" {
			v = "none"
		}
		r := core.NewFrame(par)
		r.Styler(func(s *styles.Style) { s.Direction = styles.Row; s.Padding.Set(units.Dp(3)) })
		k := core.NewText(r).SetText(kv[0])
		k.Styler(func(s *styles.Style) {
			s.Font.Size.Dp(12)
			s.Color = colors.Uniform(cDim)
			s.Min.X.Dp(110)
		})
		txt(r, v, 12.5, cInk)
	}
}

func statusBar(par core.Widget) {
	f := row(par, 28)
	txt(f, sc.Name, 11.5, cDim)
	grow(f)
	txt(f, "Cogent Core v0.3.39   one binary, WebGPU", 11.5, cDim)
}

// --- helpers ----------------------------------------------------------------

func row(par core.Widget, h float32) *core.Frame {
	f := core.NewFrame(par)
	f.Styler(func(s *styles.Style) {
		s.Direction = styles.Row
		s.Align.Items = styles.Center
		s.Min.Y.Dp(h)
		s.Max.Y.Dp(h)
		s.Grow.Set(1, 0)
		s.Padding.Set(units.Dp(2), units.Dp(10))
		s.Gap.X.Dp(14)
		s.Background = colors.Uniform(cPanel)
		s.Border.Width.Bottom.Dp(1)
		s.Border.Color.Bottom = colors.Uniform(cRule)
	})
	return f
}

func txt(par core.Widget, s string, size float32, c color.RGBA) *core.Text {
	t := core.NewText(par).SetText(s)
	t.Styler(func(st *styles.Style) {
		st.Font.Size.Dp(size)
		st.Color = colors.Uniform(c)
	})
	return t
}

func title(par core.Widget, s string) {
	t := core.NewText(par).SetText(strings.ToUpper(s))
	t.Styler(func(st *styles.Style) {
		st.Font.Size.Dp(12)
		st.Color = colors.Uniform(cDim)
		st.Margin.Bottom.Dp(6)
	})
}

func grow(par core.Widget) {
	f := core.NewFrame(par)
	f.Styler(func(s *styles.Style) { s.Grow.Set(1, 0) })
}

var _ = image.Pt
