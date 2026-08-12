// The Plan view, in Gio.
//
// What this spike is testing, in order of how much it matters:
//
//  1. Layout without hand-placed pixels.
//  2. Custom drawing: the map, its links and its labels.
//  3. A table with stable row identity and a filter.
//  4. One panel that can leave the layout and become a real OS window, while
//     the rest stay docked where docking is the right answer.
package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var (
	bg      = color.NRGBA{R: 0x0f, G: 0x12, B: 0x15, A: 0xff}
	panelBg = color.NRGBA{R: 0x17, G: 0x1b, B: 0x20, A: 0xff}
	mapBg   = color.NRGBA{R: 0x0b, G: 0x0e, B: 0x11, A: 0xff}
	ink     = color.NRGBA{R: 0xe6, G: 0xe9, B: 0xee, A: 0xff}
	dim     = color.NRGBA{R: 0x9a, G: 0xa4, B: 0xb2, A: 0xff}
	rule    = color.NRGBA{R: 0x2a, G: 0x30, B: 0x38, A: 0xff}
	accent  = color.NRGBA{R: 0x6e, G: 0xa8, B: 0xfe, A: 0xff}
	warn    = color.NRGBA{R: 0xf0, G: 0xb4, B: 0x29, A: 0xff}
)

type ui struct {
	th       *material.Theme
	sc       *scene
	filter   widget.Editor
	list     widget.List
	rows     []widget.Clickable
	selected int
	popOut   widget.Clickable
	views    []viewTab
	popped   bool
}

type viewTab struct {
	name  string
	click widget.Clickable
	on    bool
}

func main() {
	fixture := "fixtures/fixture-fife-strict.json"
	if len(os.Args) > 1 {
		fixture = os.Args[1]
	}
	u := &ui{
		th:       material.NewTheme(),
		sc:       loadScene(fixture),
		selected: -1,
	}
	u.th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	u.filter.SingleLine = true
	for _, n := range []string{"Plan", "Run", "Debug", "Verify", "Bench", "App"} {
		u.views = append(u.views, viewTab{name: n, on: n == "Plan"})
	}
	u.rows = make([]widget.Clickable, len(u.sc.Nodes))
	u.list.Axis = layout.Vertical
	if len(u.sc.Nodes) > 0 {
		u.selected = 0
	}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("MeshBench - Plan (Gio spike)"), app.Size(unit.Dp(1500), unit.Dp(900)))
		if err := u.loop(w); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func (u *ui) loop(w *app.Window) error {
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			if u.popOut.Clicked(gtx) && !u.popped {
				u.popped = true
				go u.inspectorWindow()
			}
			u.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// inspectorWindow is the point of the spike: a panel that stops being a dock
// and becomes a window the operating system knows about, with its own title
// bar, its own place on a second monitor, and no docking framework involved.
func (u *ui) inspectorWindow() {
	var ops op.Ops
	w := new(app.Window)
	w.Option(app.Title("Inspector - MeshBench"), app.Size(unit.Dp(420), unit.Dp(620)))
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			u.popped = false
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			fill(gtx, panelBg)
			layout.UniformInset(unit.Dp(14)).Layout(gtx, u.inspectorBody)
			e.Frame(gtx.Ops)
		}
	}
}

func (u *ui) layout(gtx layout.Context) layout.Dimensions {
	fill(gtx, bg)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(u.menuBar),
		layout.Rigid(u.viewBar),
		layout.Rigid(u.toolBar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, u.mapPanel),
				layout.Rigid(vRule),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(320)
					gtx.Constraints.Max.X = gtx.Dp(320)
					return u.sidePanels(gtx)
				}),
			)
		}),
		layout.Rigid(u.statusBar),
	)
}

func (u *ui) menuBar(gtx layout.Context) layout.Dimensions {
	return bar(gtx, 34, func(gtx layout.Context) layout.Dimensions {
		items := []string{"File", "View", "Simulation", "Repeaters", "Planning", "Window", "Help"}
		ch := make([]layout.FlexChild, 0, len(items)+2)
		for _, it := range items {
			ch = append(ch, layout.Rigid(label(u.th, it, 13, dim, unit.Dp(11))))
		}
		ch = append(ch, layout.Flexed(1, spacer))
		ch = append(ch, layout.Rigid(label(u.th,
			"results are a best case: no multipath, bare earth, ideal demodulator", 12, warn, unit.Dp(11))))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, ch...)
	})
}

func (u *ui) viewBar(gtx layout.Context) layout.Dimensions {
	return bar(gtx, 40, func(gtx layout.Context) layout.Dimensions {
		ch := make([]layout.FlexChild, 0, len(u.views)+2)
		for i := range u.views {
			v := &u.views[i]
			ch = append(ch, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						b := material.Button(u.th, &v.click, v.name)
						b.TextSize = unit.Sp(13)
						b.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6),
							Left: unit.Dp(14), Right: unit.Dp(14)}
						if v.on {
							b.Background = accent
							b.Color = color.NRGBA{R: 0x0f, G: 0x12, B: 0x15, A: 0xff}
						} else {
							b.Background = color.NRGBA{A: 0}
							b.Color = dim
						}
						return b.Layout(gtx)
					})
			}))
		}
		ch = append(ch, layout.Flexed(1, spacer))
		ch = append(ch, layout.Rigid(label(u.th,
			fmt.Sprintf("%d nodes   %d links   seed 9001", len(u.sc.Nodes), len(u.sc.Links)),
			12.5, dim, unit.Dp(12))))
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, ch...)
	})
}

func (u *ui) toolBar(gtx layout.Context) layout.Dimensions {
	return bar(gtx, 36, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(label(u.th, "play", 13, accent, unit.Dp(12))),
			layout.Rigid(label(u.th, "step", 13, dim, unit.Dp(10))),
			layout.Rigid(label(u.th, "reset", 13, dim, unit.Dp(10))),
			layout.Rigid(label(u.th, "|", 13, rule, unit.Dp(8))),
			layout.Rigid(label(u.th, "real firmware", 13, ink, unit.Dp(10))),
			layout.Rigid(label(u.th, "1x", 13, dim, unit.Dp(14))),
			layout.Rigid(label(u.th, "t = 0.00 s", 13, dim, unit.Dp(10))),
			layout.Flexed(1, spacer),
			layout.Rigid(label(u.th, "EU/UK (Narrow)  869.618 MHz  SF8", 12, dim, unit.Dp(12))),
		)
	})
}

// mapPanel is the custom drawing test: links, then nodes, then labels, every
// frame, with no retained scene graph to keep in step.
func (u *ui) mapPanel(gtx layout.Context) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: mapBg}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	u.sc.project(float32(sz.X), float32(sz.Y), 46)

	for _, l := range u.sc.Links {
		a, b := u.sc.Nodes[l.A], u.sc.Nodes[l.B]
		line(gtx.Ops, f32.Pt(a.X, a.Y), f32.Pt(b.X, b.Y),
			color.NRGBA{R: 0x6e, G: 0xa8, B: 0xfe, A: 0x33}, 1)
	}
	for i, n := range u.sc.Nodes {
		r, g, b := kindColour(n.Kind)
		c := color.NRGBA{R: r, G: g, B: b, A: 0xff}
		rad := float32(5)
		if i == u.selected {
			ring(gtx.Ops, f32.Pt(n.X, n.Y), 11, accent)
			rad = 7
		}
		dot(gtx.Ops, f32.Pt(n.X, n.Y), rad, c)
	}
	// Labels only for what is worth reading at this density, which is what the
	// real map does too.
	for i, n := range u.sc.Nodes {
		if n.Kind == "simple-repeater" && i%3 != 0 && i != u.selected {
			continue
		}
		off := op.Offset(image.Pt(int(n.X)+10, int(n.Y)-8)).Push(gtx.Ops)
		l := material.Label(u.th, unit.Sp(11), n.Name)
		l.Color = dim
		if i == u.selected {
			l.Color = ink
		}
		l.Layout(gtx)
		off.Pop()
	}
	scale := op.Offset(image.Pt(18, sz.Y-30)).Push(gtx.Ops)
	sl := material.Label(u.th, unit.Sp(11), "20 km    Elevation: AWS terrarium    (c) OpenStreetMap")
	sl.Color = color.NRGBA{R: 0x78, G: 0x82, B: 0x7f, A: 0xff}
	sl.Layout(gtx)
	scale.Pop()
	return layout.Dimensions{Size: sz}
}

func (u *ui) sidePanels(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1.6, u.nodesPanel),
		layout.Rigid(hRule),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if u.popped {
				return u.poppedNotice(gtx)
			}
			return u.inspectorPanel(gtx)
		}),
	)
}

func (u *ui) nodesPanel(gtx layout.Context) layout.Dimensions {
	fill(gtx, panelBg)
	return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(sectionTitle(u.th, "Nodes")),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				e := material.Editor(u.th, &u.filter, "filter by name or kind")
				e.TextSize = unit.Sp(13)
				e.Color = ink
				e.HintColor = color.NRGBA{R: 0x78, G: 0x82, B: 0x7f, A: 0xff}
				return inset(gtx, 6, func(gtx layout.Context) layout.Dimensions {
					return border(gtx, func(gtx layout.Context) layout.Dimensions {
						return inset(gtx, 6, e.Layout)
					})
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				want := strings.ToLower(strings.TrimSpace(u.filter.Text()))
				idx := make([]int, 0, len(u.sc.Nodes))
				for i, n := range u.sc.Nodes {
					if want == "" || strings.Contains(strings.ToLower(n.Name), want) ||
						strings.Contains(strings.ToLower(n.Kind), want) {
						idx = append(idx, i)
					}
				}
				return material.List(u.th, &u.list).Layout(gtx, len(idx),
					func(gtx layout.Context, row int) layout.Dimensions {
						i := idx[row]
						if u.rows[i].Clicked(gtx) {
							u.selected = i
						}
						return u.nodeRow(gtx, i)
					})
			}),
		)
	})
}

func (u *ui) nodeRow(gtx layout.Context, i int) layout.Dimensions {
	n := u.sc.Nodes[i]
	return u.rows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if i == u.selected {
			fillRect(gtx, image.Pt(gtx.Constraints.Max.X, gtx.Dp(26)),
				color.NRGBA{R: 0x23, G: 0x2c, B: 0x2a, A: 0xff})
		}
		return inset(gtx, 4, func(gtx layout.Context) layout.Dimensions {
			r, g, b := kindColour(n.Kind)
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return inset(gtx, 3, func(gtx layout.Context) layout.Dimensions {
						d := image.Pt(gtx.Dp(8), gtx.Dp(8))
						fillRect(gtx, d, color.NRGBA{R: r, G: g, B: b, A: 0xff})
						return layout.Dimensions{Size: d}
					})
				}),
				layout.Flexed(1, label(u.th, n.Name, 12.5, ink, unit.Dp(6))),
				layout.Rigid(label(u.th, shortKind(n.Kind), 11, dim, unit.Dp(4))),
			)
		})
	})
}

func (u *ui) inspectorPanel(gtx layout.Context) layout.Dimensions {
	fill(gtx, panelBg)
	return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, sectionTitle(u.th, "Inspector")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						b := material.Button(u.th, &u.popOut, "open in its own window")
						b.TextSize = unit.Sp(11.5)
						b.Background = color.NRGBA{A: 0}
						b.Color = accent
						b.Inset = layout.UniformInset(unit.Dp(4))
						return b.Layout(gtx)
					}),
				)
			}),
			layout.Flexed(1, u.inspectorBody),
		)
	})
}

func (u *ui) inspectorBody(gtx layout.Context) layout.Dimensions {
	if u.selected < 0 || u.selected >= len(u.sc.Nodes) {
		return label(u.th, "nothing selected", 12, dim, unit.Dp(0))(gtx)
	}
	n := u.sc.Nodes[u.selected]
	rows := [][2]string{
		{"name", n.Name},
		{"kind", shortKind(n.Kind)},
		{"latitude", fmt.Sprintf("%.5f", n.Lat)},
		{"longitude", fmt.Sprintf("%.5f", n.Lon)},
		{"height", fmt.Sprintf("%.0f m above ground", n.Height)},
		{"transmit power", fmt.Sprintf("%.0f dBm", n.TxDBm)},
		{"regions", strings.Join(n.Regions, ", ")},
	}
	ch := make([]layout.FlexChild, 0, len(rows))
	for _, r := range rows {
		k, v := r[0], r[1]
		if v == "" {
			v = "none"
		}
		ch = append(ch, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return inset(gtx, 3, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(110)
						return label(u.th, k, 12, dim, unit.Dp(0))(gtx)
					}),
					layout.Flexed(1, label(u.th, v, 12.5, ink, unit.Dp(0))),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, ch...)
}

func (u *ui) poppedNotice(gtx layout.Context) layout.Dimensions {
	fill(gtx, panelBg)
	return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(sectionTitle(u.th, "Inspector")),
			layout.Rigid(label(u.th, "now a separate window", 12.5, accent, unit.Dp(6))),
			layout.Rigid(label(u.th, "close it and this panel comes back", 11.5, dim, unit.Dp(4))),
		)
	})
}

func (u *ui) statusBar(gtx layout.Context) layout.Dimensions {
	return bar(gtx, 28, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(label(u.th, u.sc.Name, 11.5, dim, unit.Dp(12))),
			layout.Flexed(1, spacer),
			layout.Rigid(label(u.th, "Gio "+giover+"   one binary, no system toolkit", 11.5, dim, unit.Dp(12))),
		)
	})
}

const giover = "v0.10.2"

// --- small helpers, so the layout above reads as layout ---------------------

func fill(gtx layout.Context, c color.NRGBA) {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

func fillRect(gtx layout.Context, sz image.Point, c color.NRGBA) {
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

func bar(gtx layout.Context, h int, w layout.Widget) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(h))
	gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(h))
	fill(gtx, panelBg)
	d := layout.UniformInset(unit.Dp(4)).Layout(gtx, w)
	d.Size.Y = gtx.Dp(unit.Dp(h))
	return d
}

func label(th *material.Theme, s string, size float32, c color.NRGBA, left unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: left, Right: unit.Dp(2)}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				l := material.Label(th, unit.Sp(size), s)
				l.Color = c
				return l.Layout(gtx)
			})
	}
}

func sectionTitle(th *material.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(th, unit.Sp(12), strings.ToUpper(s))
		l.Color = dim
		return l.Layout(gtx)
	}
}

func spacer(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
}

func inset(gtx layout.Context, dp int, w layout.Widget) layout.Dimensions {
	return layout.UniformInset(unit.Dp(dp)).Layout(gtx, w)
}

func border(gtx layout.Context, w layout.Widget) layout.Dimensions {
	d := w(gtx)
	r := clip.Rect{Max: d.Size}
	paint.FillShape(gtx.Ops, rule, clip.Stroke{Path: r.Path(), Width: 1}.Op())
	return d
}

func hRule(gtx layout.Context) layout.Dimensions {
	d := image.Pt(gtx.Constraints.Max.X, 1)
	fillRect(gtx, d, rule)
	return layout.Dimensions{Size: d}
}

func vRule(gtx layout.Context) layout.Dimensions {
	d := image.Pt(1, gtx.Constraints.Max.Y)
	fillRect(gtx, d, rule)
	return layout.Dimensions{Size: d}
}

func line(ops *op.Ops, a, b f32.Point, c color.NRGBA, w float32) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(a)
	p.LineTo(b)
	paint.FillShape(ops, c, clip.Stroke{Path: p.End(), Width: w}.Op())
}

func dot(ops *op.Ops, at f32.Point, r float32, c color.NRGBA) {
	rr := clip.RRect{
		Rect: image.Rect(int(at.X-r), int(at.Y-r), int(at.X+r), int(at.Y+r)),
		NE:   int(r), NW: int(r), SE: int(r), SW: int(r),
	}
	paint.FillShape(ops, c, rr.Op(ops))
}

func ring(ops *op.Ops, at f32.Point, r float32, c color.NRGBA) {
	rr := clip.RRect{
		Rect: image.Rect(int(at.X-r), int(at.Y-r), int(at.X+r), int(at.Y+r)),
		NE:   int(r), NW: int(r), SE: int(r), SW: int(r),
	}
	paint.FillShape(ops, c, clip.Stroke{Path: rr.Path(ops), Width: 1.6}.Op())
}
