package brandmark

import (
	"image"
	"testing"
)

// Both grounds decode, and they are not the same picture.
//
// The failure this guards against is not a crash: it is shipping one export
// twice, which nobody sees on the ground it was drawn for and everybody sees
// on the other one. A mark carrying Ink on a dark panel is invisible, and an
// assertion that both files merely decode would pass while that happened.
func TestEachGroundGetsItsOwnMark(t *testing.T) {
	dark, err := Wordmark(true)
	if err != nil {
		t.Fatalf("the dark-ground mark did not decode: %v", err)
	}
	light, err := Wordmark(false)
	if err != nil {
		t.Fatalf("the light-ground mark did not decode: %v", err)
	}
	if dark.Bounds() != light.Bounds() {
		t.Errorf("the two marks are different sizes, %v and %v: they are the same "+
			"wordmark and a layout measured on one would be wrong for the other",
			dark.Bounds(), light.Bounds())
	}
	if sameImage(dark, light) {
		t.Error("both grounds got the same picture, so one of them is the wrong " +
			"export and will be drawn in a colour its ground cannot show")
	}
}

// The mark has to have a size to divide by: the drawing scales height against
// width, so a zero would be a division by zero rather than a small logo.
func TestTheMarkHasABoundToScaleFrom(t *testing.T) {
	for _, onDark := range []bool{true, false} {
		img, err := Wordmark(onDark)
		if err != nil {
			t.Fatalf("onDark=%v: %v", onDark, err)
		}
		if b := img.Bounds(); b.Dx() == 0 || b.Dy() == 0 {
			t.Errorf("onDark=%v has bounds %v", onDark, b)
		}
	}
}

func sameImage(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.At(x, y) != b.At(x, y) {
				return false
			}
		}
	}
	return true
}
