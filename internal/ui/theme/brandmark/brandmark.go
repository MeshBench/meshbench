// The wordmark, carried in the binary, in the two grounds it has to sit on.
//
// Two files rather than one recoloured at runtime: the mark is not a shape
// with a fill, it is a lit relief whose highlight and shadow are part of the
// art, and tinting that would flatten the thing the mark exists to show. The
// brand ships both exports for exactly this reason, so the choice here is
// which one to draw rather than what colour to make it.
//
// Regenerating is a change to marks.py in the brand repository and a rebuild;
// nothing here is hand-edited, and these are copies of its 400px exports.
package brandmark

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"
)

// The light file carries Ink, for a pale ground; the dark file carries the
// paper tone and the brighter Signal, for a dark one. The suffix names the
// ground it goes on, not the colour it is drawn in, which is the way round
// the brand's own table reads.
var (
	//go:embed wordmark-light.png
	lightPNG []byte
	//go:embed wordmark-dark.png
	darkPNG []byte
)

// Wordmark returns the mark for a dark or a light ground.
//
// Decoded on demand rather than at init: a package that costs two PNG decodes
// to import is a package every test pays for, and only the one panel that
// draws it needs either.
func Wordmark(onDark bool) (image.Image, error) {
	b := lightPNG
	if onDark {
		b = darkPNG
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}
