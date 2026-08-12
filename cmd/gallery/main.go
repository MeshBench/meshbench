// The component gallery: every component in the library, in both themes and
// all three densities, in one window.
//
// This is P1's exit criterion from the redesign plan. It exists so a design
// change can be judged in one glance rather than by opening six views, and so
// the golden-image tests in 13.3 have a single stable subject.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

type gallery struct {
	t       *theme.Theme
	sh      *text.Shaper
	mode    theme.Mode
	density theme.Density

	modeBtn    comp.Button
	densityBtn comp.Button

	primary, secondary, quiet, destructive, disabled comp.Button
	name, freq                                       comp.Field
	check1, check2                                   comp.Check
	table                                            comp.Table
	scroll                                           widget.List
}

func main() {
	// Flags rather than only buttons, so the golden-image tests in 13.3 and
	// the progress screenshots can ask for a specific combination without an
	// input tool in the loop.
	modeFlag := flag.String("theme", "dark", "dark or light")
	densFlag := flag.String("density", "default", "comfortable, default or compact")
	flag.Parse()

	sh := text.NewShaper(text.WithCollection(withEmoji(gofont.Collection())))
	mode := theme.Dark
	if *modeFlag == "light" {
		mode = theme.Light
	}
	dens := theme.Default
	switch *densFlag {
	case "comfortable":
		dens = theme.Comfortable
	case "compact":
		dens = theme.Compact
	}
	g := &gallery{sh: sh, mode: mode, density: dens}
	g.t = theme.New(g.mode, g.density, sh)

	g.modeBtn = comp.Button{Kind: comp.Secondary}
	g.densityBtn = comp.Button{Kind: comp.Secondary}
	g.primary = comp.Button{Kind: comp.Primary, Label: "Start firmware"}
	g.secondary = comp.Button{Kind: comp.Secondary, Label: "Open project"}
	g.quiet = comp.Button{Kind: comp.Quiet, Label: "reset view"}
	g.destructive = comp.Button{Kind: comp.Destructive, Label: "Wipe node storage"}
	g.disabled = comp.Button{Kind: comp.Primary, Label: "Export report",
		Disabled: true, Reason: "no run to export yet"}
	g.name.Label, g.name.Hint = "Node name", "Abernethy Repeater"
	g.freq.Label, g.freq.Hint, g.freq.Suffix = "Frequency", "869.618", "MHz"
	g.check1 = comp.Check{Label: "run real firmware on every node"}
	g.check2 = comp.Check{Label: "forward flood traffic for any region"}
	g.check1.Bool.Value = true
	g.name.Editor.SetText("Abernethy Repeater")
	g.freq.Editor.SetText("869.618")
	g.freq.Editor.SingleLine = true
	g.name.Editor.SingleLine = true

	g.table.Cols = []comp.Column{
		{Title: "node", Sortable: true},
		{Title: "kind", Width: 110, Sortable: true},
		{Title: "regions", Width: 120, Mono: true},
		{Title: "tx dBm", Width: 80, Right: true, Mono: true, Sortable: true},
		{Title: "heard", Width: 70, Right: true, Mono: true, Sortable: true},
	}
	g.table.SetRows(demoRows(g.t))
	g.table.Selected = "Abernethy Repeater"
	g.scroll.Axis = layout.Vertical
	g.retheme()

	go func() {
		w := new(app.Window)
		w.Option(app.Title("MeshBench component gallery"),
			app.Size(unit.Dp(1180), unit.Dp(900)))
		if err := g.loop(w); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func (g *gallery) loop(w *app.Window) error {
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			if g.modeBtn.Click.Clicked(gtx) {
				if g.mode == theme.Dark {
					g.mode = theme.Light
				} else {
					g.mode = theme.Dark
				}
				g.retheme()
			}
			if g.densityBtn.Click.Clicked(gtx) {
				g.density = (g.density + 1) % 3
				g.retheme()
			}
			g.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (g *gallery) retheme() {
	g.t = theme.New(g.mode, g.density, g.sh)
	g.modeBtn.Label = "theme: " + map[theme.Mode]string{theme.Dark: "dark", theme.Light: "light"}[g.mode]
	g.densityBtn.Label = "density: " + map[theme.Density]string{
		theme.Comfortable: "comfortable", theme.Default: "default", theme.Compact: "compact",
	}[g.density]
	g.table.SetRows(demoRows(g.t))
}

func (g *gallery) layout(gtx layout.Context) layout.Dimensions {
	t := g.t
	comp.Fill(gtx, t.P.Ground)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.topBar(gtx) }),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return comp.Inset(t, t.Sp.L, g.leftColumn)(gtx)
				}),
				layout.Rigid(comp.VRule(t)),
				layout.Flexed(1.2, func(gtx layout.Context) layout.Dimensions {
					return comp.Inset(t, t.Sp.L, g.rightColumn)(gtx)
				}),
			)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return g.statusBar(gtx) }),
	)
}

func (g *gallery) topBar(gtx layout.Context) layout.Dimensions {
	t := g.t
	comp.Fill(gtx, t.P.Panel)
	return comp.Inset(t, t.Sp.M, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, "Component gallery")),
			layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
				"P1 of the Gio redesign")),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return g.modeBtn.Layout(t, gtx)
			}),
			layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return g.densityBtn.Layout(t, gtx)
			}),
		)
	})(gtx)
}

func demoRows(t *theme.Theme) []comp.Row {
	type d struct {
		name, kind, region, tx, heard string
		k                             theme.NodeKind
	}
	src := []d{
		{"Abernethy Repeater", "repeater", "#sco", "22", "412", theme.SimpleRepeater},
		{"AngusOutlaw2 \U0001F4E1", "companion", "#sco", "17", "88", theme.Companion},
		{"Bishop Hill ☀️\U0001F50B", "repeater", "#sco #fif", "22", "377", theme.SimpleRepeater},
		{"Largo Law Advanced", "advanced", "#sco", "22", "402", theme.AdvancedRepeater},
		{"St Andrews Room Server", "room server", "#sco", "20", "96", theme.RoomServer},
		{"Kirkcaldy SDR Observer", "observer", "", "0", "1204", theme.Observer},
		{"Dunfermline Interferer", "emitter", "", "20", "0", theme.Emitter},
		{"Cadham Village \U0001F3D8️", "repeater", "#fif", "22", "289", theme.SimpleRepeater},
		{"Duke Listen'em ☢️", "repeater", "#sco", "22", "341", theme.SimpleRepeater},
		{"Bathgate room", "repeater", "#sco", "22", "255", theme.SimpleRepeater},
		{"PeterB", "companion", "#sco", "17", "61", theme.Companion},
		{"Newarthill-01", "repeater", "#sco", "22", "198", theme.SimpleRepeater},
	}
	out := make([]comp.Row, 0, len(src))
	for _, s := range src {
		out = append(out, comp.Row{
			Key:   s.name,
			Cells: []string{s.name, s.kind, s.region, s.tx, s.heard},
			Tint:  comp.Tint(t.NodeColour(s.k)),
		})
	}
	return out
}

func nrgbaOf(v [4]uint8) color.NRGBA {
	return color.NRGBA{R: v[0], G: v[1], B: v[2], A: v[3]}
}

// withEmoji appends a system colour emoji face, so node names carrying emoji
// render as pictures rather than boxes.
func withEmoji(base []font.FontFace) []font.FontFace {
	for _, p := range []string{
		"/usr/share/fonts/noto/NotoColorEmoji.ttf",
		"/usr/share/fonts/truetype/noto/NotoColorEmoji.ttf",
		"/System/Library/Fonts/Apple Color Emoji.ttc",
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		faces, err := opentype.ParseCollection(b)
		if err != nil {
			continue
		}
		return append(base, faces...)
	}
	return base
}
