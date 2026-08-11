package ui

import (
	_ "embed"
	"unsafe"

	"github.com/AllenDang/cimgui-go/imgui"
)

// Three faces, all embedded: a font found on the developer's machine and
// missing on the operator's is a "?" that ships.
//
// Inter is the UI face - the modern neutral for tool interfaces, legible at
// small sizes and dense in tables. DejaVu Sans is merged over it purely for
// glyphs Inter lacks, so symbols land at the same optical size as the text
// around them. DejaVu Sans Mono is a separate font for consoles, hex dumps
// and the event ledger, where columns have to line up.
//
// Licences: fonts/LICENSE-Inter.txt (OFL), fonts/LICENSE (Bitstream Vera).

//go:embed fonts/Inter-Regular.ttf
var interRegular []byte

//go:embed fonts/DejaVuSans.ttf
var dejaVuSans []byte

//go:embed fonts/DejaVuSansMono.ttf
var dejaVuMono []byte

// monoFont is the fixed-width face, for anything that is really a machine's
// output rather than prose.
var monoFont *imgui.Font

// loadFonts builds the atlas. Sizes are the base; ctrl +/- scales from here
// and this imgui re-rasterises rather than stretching.
func loadFonts() {
	fonts := imgui.CurrentIO().Fonts()

	ui := addTTF(fonts, interRegular, 15, false)
	_ = ui
	// Merged into Inter: fills only the glyphs Inter has no answer for.
	addTTF(fonts, dejaVuSans, 0, true)

	monoFont = addTTF(fonts, dejaVuMono, 14, false)
}

func addTTF(fonts *imgui.FontAtlas, data []byte, size float32, merge bool) *imgui.Font {
	cfg := imgui.NewFontConfig()
	defer cfg.Destroy()
	if merge {
		cfg.SetMergeMode(true)
	}
	// imgui frees font data itself unless told otherwise; these bytes belong
	// to Go's embed and must outlive the atlas untouched.
	cfg.SetFontDataOwnedByAtlas(false)
	return fonts.AddFontFromMemoryTTFV(
		uintptr(unsafe.Pointer(&data[0])), int32(len(data)), size, cfg, nil)
}

// pushMono / popMono wrap machine output. A helper pair rather than raw
// PushFont calls so an unbalanced pop cannot leave the whole UI monospaced.
func pushMono() {
	if monoFont != nil {
		imgui.PushFont(monoFont, 0)
	}
}

func popMono() {
	if monoFont != nil {
		imgui.PopFont()
	}
}
