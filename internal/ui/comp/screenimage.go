// A board's panel, drawn as one image rather than as a fill per pixel.
//
// It was a fill per pixel, in two places. A T-Deck's panel is 320 by 240, so
// that is seventy-six thousand eight hundred fill operations to draw one board,
// on every frame, and a pop-out window invalidates every frame. With the node
// window and the board view both open on one node the machine was asked for a
// hundred and fifty thousand of them a frame, for a picture that changes when
// the firmware draws and not otherwise - and it had an emulator to run at the
// same time.
//
// So the picture is built into an image when it changes and blitted when it has
// not, which is what the waterfall and the map's coverage layer already do with
// theirs. The sequence number the panel socket has always carried is what says
// whether it changed; it was being thrown away one line above the interface.
package comp

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// ScreenImage holds one board panel's last picture, ready to blit.
//
// One per drawn panel, so two windows on one node keep an image each: they may
// be drawn at different scales, and sharing would rebuild on every frame as
// each asked for its own.
type ScreenImage struct {
	op  paint.ImageOp
	img *image.RGBA
	// what the held image was built from, so it is rebuilt when any of it
	// moves and not otherwise.
	seq       uint64
	blk       int
	w, h      int
	lit       color.NRGBA
	built     bool
	everBuilt bool
}

// Layout draws the panel at blk framebuffer pixels per panel pixel.
//
// blk rather than a scale, because that is the number a pixel is actually
// drawn at: one panel pixel is a whole number of framebuffer pixels and the box
// is derived from it, which is what keeps the picture the same size as the
// frame around it on a display at any density.
func (s *ScreenImage) Layout(t *theme.Theme, gtx layout.Context,
	pic *state.Screen, blk int) layout.Dimensions {

	if pic == nil || blk < 1 || pic.Width <= 0 || pic.Height <= 0 {
		return layout.Dimensions{}
	}
	size := image.Pt(pic.Width*blk, pic.Height*blk)
	s.rebuild(t, pic, blk)
	if !s.everBuilt {
		return layout.Dimensions{Size: size}
	}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	s.op.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}

// rebuild makes the image again, but only when what it is made of has moved.
func (s *ScreenImage) rebuild(t *theme.Theme, pic *state.Screen, blk int) {
	lit := t.P.ScreenLit
	if s.built && s.seq == pic.Seq && s.blk == blk &&
		s.w == pic.Width && s.h == pic.Height && s.lit == lit {
		return
	}
	s.seq, s.blk, s.w, s.h, s.lit, s.built = pic.Seq, blk, pic.Width, pic.Height, lit, true

	size := image.Pt(pic.Width*blk, pic.Height*blk)
	if s.img == nil || s.img.Rect.Max != size {
		s.img = image.NewRGBA(image.Rectangle{Max: size})
	}
	ground := t.P.ScreenGround
	for y := 0; y < pic.Height; y++ {
		for x := 0; x < pic.Width; x++ {
			c, on := screenPixel(t, pic, x, y)
			if !on {
				c = ground
			}
			// One panel pixel, filled as a block. Written straight into the
			// image rather than through a draw call: this is the loop that was
			// costing a fill operation each time round.
			for by := 0; by < blk; by++ {
				row := (y*blk + by) * s.img.Stride
				for bx := 0; bx < blk; bx++ {
					i := row + (x*blk+bx)*4
					s.img.Pix[i] = c.R
					s.img.Pix[i+1] = c.G
					s.img.Pix[i+2] = c.B
					s.img.Pix[i+3] = 0xFF
				}
			}
		}
	}
	s.op = paint.NewImageOp(s.img)
	s.everBuilt = true
}

// screenPixel is what to paint at one point, and whether the panel is lit
// there.
//
// A colour panel carries its own colour; drawing that in a theme colour would
// be inventing a picture the firmware did not send. A monochrome one carries a
// bit, and the theme decides what lit looks like.
func screenPixel(t *theme.Theme, pic *state.Screen, x, y int) (color.NRGBA, bool) {
	if pic.BPP == 16 {
		r, g, b, ok := pic.At(x, y)
		if !ok || (r == 0 && g == 0 && b == 0) {
			return color.NRGBA{}, false
		}
		return color.NRGBA{R: r, G: g, B: b, A: 0xff}, true
	}
	if !pic.Lit(x, y) {
		return color.NRGBA{}, false
	}
	return t.P.ScreenLit, true
}
