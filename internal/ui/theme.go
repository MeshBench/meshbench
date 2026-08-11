package ui

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"
)

// The palette, named once. Every colour in the UI comes from here.
//
// Before this there were six ad-hoc vec4s scattered through a dozen files —
// four different oranges for "warning", three greens for "fine" — which is
// why the tool looked assembled rather than designed. Tokens also give a
// light theme somewhere to exist later without a hunt.
var (
	colBG0     = rgb(0x0E, 0x11, 0x16) // application background
	colBG1     = rgb(0x15, 0x1A, 0x21) // panels
	colBG2     = rgb(0x1C, 0x23, 0x2E) // inputs, headers, standard buttons
	colBG3     = rgb(0x27, 0x30, 0x3D) // hover
	colAccent  = rgb(0x4C, 0x8D, 0xFF) // selection, active view, primary action
	colAccentD = rgb(0x3A, 0x6F, 0xCC) // its pressed state
	colText    = rgb(0xD7, 0xDE, 0xE8)
	colTextDim = rgb(0x8B, 0x94, 0xA3)
	colOK      = rgb(0x46, 0xC5, 0x74)
	colWarn    = rgb(0xE8, 0xB3, 0x3D)
	colErr     = rgb(0xE0, 0x52, 0x52)
	colBorder  = rgba(0xFF, 0xFF, 0xFF, 0x14)
)

func rgb(r, g, b uint8) imgui.Vec4 { return rgba(r, g, b, 0xFF) }

func rgba(r, g, b, a uint8) imgui.Vec4 {
	return imgui.NewVec4(float32(r)/255, float32(g)/255, float32(b)/255, float32(a)/255)
}

// applyTheme sets the whole style: colours, rounding and the spacing rhythm.
//
// The difference between a 2010 tool and a current one is mostly corner
// radius, spacing and restraint about colour — none of it is clever, and all
// of it has to be done in one place or it drifts.
func applyTheme() {
	s := imgui.CurrentStyle()

	s.SetWindowRounding(6)
	s.SetChildRounding(4)
	s.SetFrameRounding(4)
	s.SetPopupRounding(6)
	s.SetScrollbarRounding(6)
	s.SetGrabRounding(4)
	s.SetTabRounding(4)

	s.SetWindowPadding(imgui.NewVec2(10, 10))
	s.SetFramePadding(imgui.NewVec2(8, 5))
	s.SetItemSpacing(imgui.NewVec2(8, 6))
	s.SetItemInnerSpacing(imgui.NewVec2(6, 5))
	s.SetCellPadding(imgui.NewVec2(8, 4))
	s.SetIndentSpacing(18)
	s.SetScrollbarSize(12)
	s.SetGrabMinSize(10)

	s.SetWindowBorderSize(1)
	s.SetChildBorderSize(1)
	s.SetPopupBorderSize(1)
	s.SetFrameBorderSize(0)
	s.SetTabBorderSize(0)

	s.SetWindowTitleAlign(imgui.NewVec2(0.0, 0.5))
	s.SetButtonTextAlign(imgui.NewVec2(0.5, 0.5))
	s.SetSelectableTextAlign(imgui.NewVec2(0, 0.5))

	// The binding exposes the colour array wholesale rather than per index,
	// so the palette is edited as a block and written back once.
	cols := s.Colors()
	set := func(id imgui.Col, c imgui.Vec4) {
		if int(id) < len(cols) {
			cols[id] = c
		}
	}
	set(imgui.ColText, colText)
	set(imgui.ColTextDisabled, colTextDim)
	set(imgui.ColWindowBg, colBG1)
	set(imgui.ColChildBg, imgui.NewVec4(0, 0, 0, 0))
	set(imgui.ColPopupBg, colBG2)
	set(imgui.ColBorder, colBorder)
	set(imgui.ColBorderShadow, imgui.NewVec4(0, 0, 0, 0))
	set(imgui.ColFrameBg, colBG2)
	set(imgui.ColFrameBgHovered, colBG3)
	set(imgui.ColFrameBgActive, colBG3)
	set(imgui.ColTitleBg, colBG0)
	set(imgui.ColTitleBgActive, colBG2)
	set(imgui.ColTitleBgCollapsed, colBG0)
	set(imgui.ColMenuBarBg, colBG0)
	set(imgui.ColScrollbarBg, imgui.NewVec4(0, 0, 0, 0))
	set(imgui.ColScrollbarGrab, colBG3)
	set(imgui.ColScrollbarGrabHovered, colTextDim)
	set(imgui.ColScrollbarGrabActive, colTextDim)
	set(imgui.ColCheckMark, colAccent)
	set(imgui.ColSliderGrab, colAccent)
	set(imgui.ColSliderGrabActive, colAccentD)
	set(imgui.ColButton, colBG2)
	set(imgui.ColButtonHovered, colBG3)
	set(imgui.ColButtonActive, colAccentD)
	set(imgui.ColHeader, colBG2)
	set(imgui.ColHeaderHovered, colBG3)
	set(imgui.ColHeaderActive, colAccentD)
	set(imgui.ColSeparator, colBorder)
	set(imgui.ColSeparatorHovered, colAccent)
	set(imgui.ColSeparatorActive, colAccent)
	set(imgui.ColResizeGrip, imgui.NewVec4(0, 0, 0, 0))
	set(imgui.ColResizeGripHovered, colBG3)
	set(imgui.ColResizeGripActive, colAccent)
	set(imgui.ColTab, colBG0)
	set(imgui.ColTabHovered, colBG3)
	set(imgui.ColTabSelected, colBG1)
	set(imgui.ColTabSelectedOverline, colAccent)
	set(imgui.ColTabDimmed, colBG0)
	set(imgui.ColTabDimmedSelected, colBG1)
	set(imgui.ColDockingPreview, rgba(0x4C, 0x8D, 0xFF, 0x66))
	set(imgui.ColDockingEmptyBg, colBG0)
	set(imgui.ColTableHeaderBg, colBG2)
	set(imgui.ColTableBorderStrong, colBorder)
	set(imgui.ColTableBorderLight, colBorder)
	set(imgui.ColTableRowBg, imgui.NewVec4(0, 0, 0, 0))
	set(imgui.ColTableRowBgAlt, rgba(0xFF, 0xFF, 0xFF, 0x08))
	set(imgui.ColTextSelectedBg, rgba(0x4C, 0x8D, 0xFF, 0x59))
	set(imgui.ColNavCursor, colAccent)
	s.SetColors(&cols)
}

// primaryButton is the one action a panel is for: fetch, commit, play.
// Exactly one per context, or it stops meaning anything.
func primaryButton(label string, size imgui.Vec2) bool {
	imgui.PushStyleColorVec4(imgui.ColButton, colAccent)
	imgui.PushStyleColorVec4(imgui.ColButtonHovered, rgb(0x63, 0x9D, 0xFF))
	imgui.PushStyleColorVec4(imgui.ColButtonActive, colAccentD)
	imgui.PushStyleColorVec4(imgui.ColText, rgb(0x0B, 0x0E, 0x12))
	clicked := imgui.ButtonV(label, size)
	imgui.PopStyleColorV(4)
	return clicked
}

// dangerButton is an armed destructive action - the second click of a
// restart or a delete, never the first.
func dangerButton(label string, size imgui.Vec2) bool {
	imgui.PushStyleColorVec4(imgui.ColButton, colErr)
	imgui.PushStyleColorVec4(imgui.ColButtonHovered, rgb(0xF0, 0x6A, 0x6A))
	imgui.PushStyleColorVec4(imgui.ColButtonActive, rgb(0xC4, 0x3F, 0x3F))
	imgui.PushStyleColorVec4(imgui.ColText, rgb(0x0B, 0x0E, 0x12))
	clicked := imgui.ButtonV(label, size)
	imgui.PopStyleColorV(4)
	return clicked
}

// symbolButtonSize keeps every glyph button the same square, so a row of
// transport controls reads as one instrument rather than five buttons that
// happen to sit together.
func symbolButtonSize() imgui.Vec2 {
	h := imgui.FrameHeight()
	return imgui.NewVec2(h, h)
}

// textColoured is the one-liner for coloured text, so a colour token is
// never re-derived inline.
func textColoured(c imgui.Vec4, s string) {
	imgui.PushStyleColorVec4(imgui.ColText, c)
	imgui.TextWrapped(s)
	imgui.PopStyleColor()
}

// text and textWrap print a *string*, not a format.
//
// imgui's Text family is printf-shaped, so any string containing a percent
// sign is reinterpreted: "lowest charge 95% on 26 December" came out as
// "lowest charge 9536040001700n 26 December". Every duty cycle, coverage
// figure and charge level in this application contains a percent sign, so
// these two helpers exist and the raw calls do not get used.
func textWrap(s string) {
	imgui.PushTextWrapPosV(0)
	imgui.TextUnformattedV(s)
	imgui.PopTextWrapPos()
}

// textDim does not wrap. Wrapping is measured against the *window's* width,
// and in a menu popup that is the width of the longest item so far - which
// turned the Help menu into one letter per line. Panels that want wrapped
// secondary prose ask for it explicitly.
func textDim(s string) {
	imgui.PushStyleColorVec4(imgui.ColText, colTextDim)
	imgui.TextUnformattedV(s)
	imgui.PopStyleColor()
}

// textDimWrap is the wrapped variant, for panel bodies where the width is
// the panel's own and wrapping is what is wanted.
func textDimWrap(s string) {
	imgui.PushStyleColorVec4(imgui.ColText, colTextDim)
	textWrap(s)
	imgui.PopStyleColor()
}

// numF64 is a typed number, not a slider.
//
// A slider is for a value you explore; a mast height, a transmit power and a
// battery capacity are values you *know*, and dragging a bar to land on 22
// dBm is a worse way to enter 22 than typing it. Drag still works for a
// quick sweep - imgui's input fields drag when you pull sideways - and the
// value is clamped to what the model accepts.
func numF64(label string, v *float64, lo, hi float64, format string) bool {
	f := float32(*v)
	imgui.SetNextItemWidth(110)
	changed := imgui.InputFloatV("##"+label, &f, 0, 0, format, imgui.InputTextFlagsCharsDecimal)
	if changed {
		d := float64(f)
		if d < lo {
			d = lo
		}
		if d > hi {
			d = hi
		}
		*v = d
	}
	imgui.SameLine()
	textDim(label)
	return changed
}

// numF32 is the same for the float32 fields the UI state holds directly.
func numF32(label string, v *float32, lo, hi float32, format string) bool {
	imgui.SetNextItemWidth(110)
	changed := imgui.InputFloatV("##"+label, v, 0, 0, format, imgui.InputTextFlagsCharsDecimal)
	if changed {
		if *v < lo {
			*v = lo
		}
		if *v > hi {
			*v = hi
		}
	}
	imgui.SameLine()
	textDim(label)
	return changed
}

// windowSize sizes a window in characters, not pixels.
//
// Every default size in this application was a pixel constant chosen against
// a 13 px bitmap font. Change the font, the scale or the DPI and each one is
// wrong in its own direction - which is why fixing them one at a time never
// converged: a window holding seventy characters of prose is seventy
// characters wide whatever the font is, and only the *measurement* changes.
//
// cols is how many characters of body text must fit; rows is how many lines.
// Both include the frame's own padding, so callers say what the content
// needs and nothing else.
func (a *App) windowSize(cols, rows float32) imgui.Vec2 {
	ch := imgui.CalcTextSize("0")
	if ch.X <= 0 {
		ch = imgui.NewVec2(8, 16)
	}
	style := imgui.CurrentStyle()
	padX := style.WindowPadding().X*2 + style.ScrollbarSize() + 8
	padY := style.WindowPadding().Y*2 + imgui.FrameHeight()*2
	return imgui.NewVec2(cols*ch.X+padX, rows*(ch.Y+style.ItemSpacing().Y)+padY)
}

// textBad and textGood carry a verdict, not a mood: a number that moved the
// wrong way and one that moved the right way read differently at a glance, and
// an experiment matrix is scanned rather than read.
func textBad(s string) {
	imgui.PushStyleColorVec4(imgui.ColText, colWarn)
	imgui.TextUnformattedV(s)
	imgui.PopStyleColor()
}

func textGood(s string) {
	imgui.PushStyleColorVec4(imgui.ColText, colOK)
	imgui.TextUnformattedV(s)
	imgui.PopStyleColor()
}

func textBadWrap(s string) {
	imgui.PushStyleColorVec4(imgui.ColText, colWarn)
	textWrap(s)
	imgui.PopStyleColor()
}

func textGoodWrap(s string) {
	imgui.PushStyleColorVec4(imgui.ColText, colOK)
	textWrap(s)
	imgui.PopStyleColor()
}

// textf formats and prints, without imgui re-reading the result as a format
// string.
//
// imgui.Text is printf-shaped, so a percentage - "79.4%" - is taken as a
// conversion and everything after it comes out as garbage. Every formatted
// string in this codebase goes through here.
func textf(format string, args ...any) {
	imgui.TextUnformattedV(fmt.Sprintf(format, args...))
}
