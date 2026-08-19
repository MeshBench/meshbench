package comp

import (
	"image/color"

	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// material_List wraps Gio's list with the theme's scrollbar colouring, kept
// in one place so every scrolling surface matches.
func material_List(t *theme.Theme, l *widget.List) material.ListStyle {
	ls := material.List(t.M, l)
	ls.Track.Color = theme.Alpha(t.P.Ink, 0.05)
	ls.Indicator.Color = theme.Alpha(t.P.Ink, 0.25)
	ls.Indicator.HoverColor = theme.Alpha(t.P.Accent, 0.6)
	return ls
}

// List is the themed scrolling list, for callers outside Table.
func List(t *theme.Theme, l *widget.List, n int, el layout.ListElement) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material_List(t, l).Layout(gtx, n, el)
	}
}

func nrgba(v [4]uint8) color.NRGBA {
	return color.NRGBA{R: v[0], G: v[1], B: v[2], A: v[3]}
}

// Tint converts a theme colour into the table's swatch form.
func Tint(c color.NRGBA) [4]uint8 { return [4]uint8{c.R, c.G, c.B, c.A} }
