// The wordmark at the head of the Configuration overview, and the version
// beside it.
//
// It sits here because Configuration is the page somebody opens to find out
// what this build assumes, and "which build" is the first of those questions.
// The licence window already answered it, which is a strange place to have to
// go looking, and a version nobody can find is a version nobody quotes in a
// bug report.
package workbench

import (
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/theme/brandmark"
)

// markCache holds the uploaded wordmark and the mode it was decoded for.
//
// Widget identity is address and an ImageOp is an upload: rebuilding it every
// frame would re-upload a bitmap sixty times a second for a thing that changes
// only when somebody switches theme.
type markCache struct {
	op   paint.ImageOp
	size image.Point
	dark bool
	got  bool
}

// wordmark draws the mark for the theme's own ground, or nothing at all.
//
// Nothing, rather than a placeholder, if the mark cannot be decoded: this is
// an ornament on a page whose job is the cards below it, and a broken image
// where a logo should be says less than an empty space does.
func (m *markCache) wordmark(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		dark := t.Mode == theme.Dark
		if !m.got || m.dark != dark {
			img, err := brandmark.Wordmark(dark)
			if err != nil {
				return layout.Dimensions{}
			}
			m.op = paint.NewImageOp(img)
			m.size = img.Bounds().Size()
			m.dark, m.got = dark, true
		}
		if m.size.X == 0 {
			return layout.Dimensions{}
		}
		// Height comes from the type scale so the mark rises and falls with
		// the text beside it rather than sitting at a pixel count of its own.
		h := gtx.Dp(t.Sp.L)
		w := m.size.X * h / m.size.Y
		sc := float32(h) / float32(m.size.Y)
		defer op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(sc, sc))).Push(gtx.Ops).Pop()
		m.op.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
}

// brandCard is the wordmark and the version, as the overview's first card.
//
// The version is Mono because it is an identifier somebody copies into a bug
// report, and the build detail sits under it quietly: a tagged release says
// its tag and nothing else, while a working copy says which commit it is,
// which is the difference that matters when a screenshot arrives.
func (m *markCache) brandCard(t *theme.Theme) layout.Widget {
	return comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(m.wordmark(t)),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
			}),
			layout.Rigid(comp.Mono(t, t.Sz.Body, t.P.Dim, version.String())),
		)
	})
}
