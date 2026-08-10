package ui

import (
	_ "embed"
	"unsafe"

	"github.com/AllenDang/cimgui-go/imgui"
)

// DejaVu Sans, embedded and merged over the default font for the glyphs it
// lacks — real media symbols for the run strip instead of ASCII stand-ins,
// and proper dashes wherever they appear. Embedded, because a font found on
// the developer's machine and missing on the operator's is a "?" that ships.
// Licence: Bitstream Vera / public domain (fonts/LICENSE).
//
//go:embed fonts/DejaVuSans.ttf
var dejaVuSans []byte

// mergeSymbolFont adds DejaVu as a fallback for glyphs the default font has
// no answer to. Merge mode keeps the default font's look for ASCII; DejaVu
// only fills the gaps, rasterised on demand by this imgui's dynamic atlas.
func mergeSymbolFont() {
	fonts := imgui.CurrentIO().Fonts()
	fonts.AddFontDefault()
	cfg := imgui.NewFontConfig()
	defer cfg.Destroy()
	cfg.SetMergeMode(true)
	// imgui frees merged font data itself unless told otherwise; the bytes
	// belong to Go's embed, so it must not.
	cfg.SetFontDataOwnedByAtlas(false)
	// Size zero: implicit, matching AddFontDefault. An explicit size on a
	// merged font when the base used an implicit one is an imgui assertion.
	fonts.AddFontFromMemoryTTFV(
		uintptr(unsafe.Pointer(&dejaVuSans[0])), int32(len(dejaVuSans)),
		0, cfg, nil)
}
