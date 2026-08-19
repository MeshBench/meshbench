package comp

import "gioui.org/f32"

// f32Scale is a scaling affine transform, named because op.Affine takes an
// f32.Affine2D and the call site reads better with the intent stated.
func f32Scale(sx, sy float32) f32.Affine2D {
	return f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(sx, sy))
}
