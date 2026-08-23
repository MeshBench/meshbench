// The three faces the identity is set in, carried in the binary.
//
// Space Grotesk for display, Inter for everything read as prose, JetBrains
// Mono for anything that is data: a console, a hex dump, a column of figures.
// The brand names those three, and a workbench set in whatever the machine
// happened to have installed is one that looks different on every desk.
//
// Embedded rather than loaded from beside the binary, which is what the colour
// emoji font does. That one is several megabytes and only wanted when a node
// name has an emoji in it; these three are the interface itself, and an
// interface that falls back to a different face because a file is missing is
// worse than one that is simply larger. Together they are about a megabyte and a
// half again.
//
// All three are SIL Open Font Licence 1.1, with the licence text beside them
// here and in the inventory the workbench shows.
package brandfont

import (
	_ "embed"
	"fmt"
	"sync"

	"gioui.org/font"
	"gioui.org/font/opentype"
)

//go:embed SpaceGrotesk.ttf
var spaceGrotesk []byte

//go:embed Inter.ttf
var inter []byte

//go:embed JetBrainsMono-Regular.ttf
var jetBrainsMono []byte

//go:embed JetBrainsMono-Bold.ttf
var jetBrainsMonoBold []byte

// The typeface names the theme asks for. Gio matches a face by name, so these
// are the strings a caller sets on a text style rather than an enum.
const (
	Display = "Space Grotesk"
	Body    = "Inter"
	Mono    = "JetBrains Mono"
)

var (
	once   sync.Once
	faces  []font.FontFace
	parsed error
)

// Collection is the three faces, parsed once.
//
// Bold is registered for the mono face and synthesised for the others: the
// brand sets display at 700 and Space Grotesk is vendored at that weight, so
// asking for bold display gets the real thing, while bold body text is rare
// enough in this interface that a synthesised weight is not worth another
// three hundred kilobytes.
func Collection() []font.FontFace {
	once.Do(func() {
		for _, f := range []struct {
			name string
			b    []byte
			font font.Font
		}{
			{Display, spaceGrotesk, font.Font{Typeface: Display, Weight: font.Bold}},
			{Body, inter, font.Font{Typeface: Body}},
			{Mono, jetBrainsMono, font.Font{Typeface: Mono}},
			{Mono, jetBrainsMonoBold, font.Font{Typeface: Mono, Weight: font.Bold}},
		} {
			face, err := opentype.Parse(f.b)
			if err != nil {
				// Embedded and therefore known good at build time, so this is
				// a corrupted binary rather than a missing file. Said rather
				// than swallowed: an interface silently in the wrong face is
				// the thing this package exists to prevent.
				parsed = fmt.Errorf("brandfont: %s: %w", f.name, err)
				return
			}
			faces = append(faces, font.FontFace{Font: f.font, Face: face})
		}
	})
	return faces
}

// Err is what went wrong parsing them, if anything did.
func Err() error { Collection(); return parsed }
