// Package theme is the design system: every colour, size and spacing value the
// interface uses, named once.
//
// Nothing outside this package writes a literal colour or a literal pixel
// count. That is the rule that makes a second theme, a second density and a
// colour-blind-safe palette changes to one file rather than an archaeology
// expedition through twenty thousand lines.
package theme

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Mode is which palette is in use.
type Mode int

const (
	Dark Mode = iota
	Light
)

// Density trades information for legibility. A 311-node table is unusable at
// Comfortable; a settings form is unreadable at Compact.
type Density int

const (
	Comfortable Density = iota
	Default
	Compact
)

// Palette is every semantic colour. Named by role, never by appearance: there
// is no "grey", there is Dim, and it is whatever grey the mode needs.
type Palette struct {
	// Ground is the window behind everything; Panel is a surface on it; Sunk
	// is a well inside a surface, for inputs and code.
	Ground, Panel, Sunk color.NRGBA
	// Ink is primary text, Dim is secondary, Faint is for things present but
	// not being read.
	Ink, Dim, Faint color.NRGBA
	// Rule is a border or divider. Accent is the one colour that means
	// "interactive or selected".
	Rule, Accent color.NRGBA
	// AccentInk is what sits legibly on top of Accent.
	AccentInk color.NRGBA
	// Semantic states. Good is not always green in meaning: it means the
	// direction the operator wants.
	Good, Warn, Bad color.NRGBA
	// Selection background, distinct from Accent so a selected row does not
	// shout as loudly as a button.
	Selected color.NRGBA
	// MapInk and MapInkDark are for text drawn on the basemap itself, and
	// MapPlate is the dark ring under markers. The same in both modes,
	// deliberately: text on the map contends with the map's own colours, not
	// the theme's ground - the map picks MapInk over a dark layer and
	// MapInkDark over a light one.
	MapInk, MapInkDark, MapPlate color.NRGBA
	// ScreenGround and ScreenLit are a board's own display, and are the same
	// in both modes for the same reason MapInk is: a panel is a physical
	// object, not a surface of this interface. An OLED reads as light on
	// black whichever way the workbench is set, and painting it in theme
	// colours would draw something the board cannot do.
	ScreenGround, ScreenLit color.NRGBA
}

// dark is the primary palette. Neutrals carry a slight blue-green bias rather
// than being pure grey, so the interface reads as chosen rather than defaulted,
// and so the teal accent sits in the same family as its surroundings.
var dark = Palette{
	Ground:       rgb(0x0d, 0x11, 0x13),
	Panel:        rgb(0x14, 0x1a, 0x1d),
	Sunk:         rgb(0x0a, 0x0e, 0x10),
	Ink:          rgb(0xe4, 0xea, 0xe8),
	Dim:          rgb(0x93, 0xa1, 0x9e),
	Faint:        rgb(0x68, 0x76, 0x74),
	Rule:         rgb(0x23, 0x2c, 0x2f),
	Accent:       rgb(0x4a, 0xc4, 0xa8),
	AccentInk:    rgb(0x06, 0x0d, 0x0c),
	Good:         rgb(0x7f, 0xc9, 0x7f),
	Warn:         rgb(0xe0, 0xa8, 0x5c),
	Bad:          rgb(0xe0, 0x7a, 0x6a),
	Selected:     rgb(0x1d, 0x2c, 0x2c),
	ScreenGround: rgb(0x08, 0x0c, 0x0e),
	ScreenLit:    rgb(0x7f, 0xd8, 0xee),
	MapInk:       rgb(0xf2, 0xf5, 0xff),
	MapInkDark:   rgb(0x22, 0x28, 0x2c),
	MapPlate:     color.NRGBA{R: 0x0d, G: 0x0f, B: 0x14, A: 0xb3},
}

// light is a real design rather than an inversion: the accent darkens so it
// still reads on a pale ground, and the neutrals keep the same bias.
var light = Palette{
	Ground:    rgb(0xf7, 0xf8, 0xf7),
	Panel:     rgb(0xff, 0xff, 0xff),
	Sunk:      rgb(0xed, 0xf0, 0xef),
	Ink:       rgb(0x16, 0x1b, 0x1a),
	Dim:       rgb(0x53, 0x5f, 0x5d),
	Faint:     rgb(0x7d, 0x89, 0x87),
	Rule:      rgb(0xd6, 0xdc, 0xda),
	Accent:    rgb(0x0f, 0x6b, 0x57),
	AccentInk: rgb(0xff, 0xff, 0xff),
	Good:      rgb(0x2f, 0x6b, 0x33),
	Warn:      rgb(0x8a, 0x5a, 0x16),
	Bad:       rgb(0x9c, 0x3b, 0x2c),
	Selected:  rgb(0xdd, 0xeb, 0xe6),
	// The same as dark's, on purpose: see the field comment.
	ScreenGround: rgb(0x08, 0x0c, 0x0e),
	ScreenLit:    rgb(0x7f, 0xd8, 0xee),
	MapInk:       rgb(0xf2, 0xf5, 0xff),
	MapInkDark:   rgb(0x22, 0x28, 0x2c),
	MapPlate:     color.NRGBA{R: 0x0d, G: 0x0f, B: 0x14, A: 0xb3},
}

// NodeKind colours the six node kinds. Chosen to stay distinguishable under
// deuteranopia and protanopia, which rules out the obvious red-green pairing
// the old palette used for repeater against emitter.
type NodeKind int

const (
	SimpleRepeater NodeKind = iota
	AdvancedRepeater
	Companion
	RoomServer
	Observer
	Emitter
)

func (t *Theme) NodeColour(k NodeKind) color.NRGBA {
	if t.Mode == Light {
		switch k {
		case AdvancedRepeater:
			return rgb(0x2b, 0x5c, 0xa8)
		case Companion:
			return rgb(0x0f, 0x6b, 0x57)
		case RoomServer:
			return rgb(0x8a, 0x5a, 0x16)
		case Observer:
			return rgb(0x5d, 0x53, 0x8a)
		case Emitter:
			return rgb(0x9c, 0x3b, 0x2c)
		default:
			return rgb(0x3a, 0x76, 0xc9)
		}
	}
	switch k {
	case AdvancedRepeater:
		return rgb(0x8f, 0xb8, 0xf5)
	case Companion:
		return rgb(0x4a, 0xc4, 0xa8)
	case RoomServer:
		return rgb(0xe0, 0xa8, 0x5c)
	case Observer:
		return rgb(0xa9, 0x9f, 0xd6)
	case Emitter:
		return rgb(0xe0, 0x7a, 0x6a)
	default:
		return rgb(0x5f, 0x9e, 0xe8)
	}
}

// Scale is the type scale, in named roles rather than sizes, so a heading is
// never "the 22 one".
type Scale struct {
	Display, Title, Section, Body, Label, Caption, Data unit.Sp
}

// Space is the spacing scale. Everything is a multiple of four, and nothing
// outside this file invents a gap.
type Space struct {
	XXS, XS, S, M, L, XL, XXL unit.Dp
}

// Theme is the whole system, resolved for one mode and one density.
type Theme struct {
	Mode    Mode
	Density Density
	P       Palette
	Sz      Scale
	Sp      Space
	// M is Gio's own theme, for the widgets that take one. Its palette is
	// kept in step with P so a stray material widget cannot look foreign.
	M *material.Theme
	// Mono is the face for numbers, identifiers and anything that lines up in
	// a column.
	Mono font.Font
}

// New builds a theme. Shaper is supplied by the caller because font loading is
// an application concern, not a design-system one.
func New(mode Mode, density Density, sh *text.Shaper) *Theme {
	t := &Theme{Mode: mode, Density: density}
	if mode == Light {
		t.P = light
	} else {
		t.P = dark
	}
	switch density {
	case Comfortable:
		t.Sz = Scale{Display: 30, Title: 21, Section: 15, Body: 14.5, Label: 13.5, Caption: 12.5, Data: 13.5}
		t.Sp = Space{XXS: 2, XS: 4, S: 8, M: 12, L: 18, XL: 26, XXL: 38}
	case Compact:
		t.Sz = Scale{Display: 24, Title: 17, Section: 12, Body: 12, Label: 11, Caption: 10.5, Data: 11.5}
		t.Sp = Space{XXS: 1, XS: 2, S: 4, M: 6, L: 10, XL: 14, XXL: 20}
	default:
		t.Sz = Scale{Display: 27, Title: 19, Section: 13.5, Body: 13, Label: 12, Caption: 11.5, Data: 12.5}
		t.Sp = Space{XXS: 2, XS: 3, S: 6, M: 9, L: 14, XL: 20, XXL: 28}
	}
	t.M = material.NewTheme()
	t.M.Shaper = sh
	t.M.Fg = t.P.Ink
	t.M.Bg = t.P.Ground
	t.M.ContrastBg = t.P.Accent
	t.M.ContrastFg = t.P.AccentInk
	t.Mono = font.Font{Typeface: "Go Mono"}
	return t
}

// RowHeight is the height of one table row at this density, so every table
// agrees without each one choosing.
func (t *Theme) RowHeight() unit.Dp {
	switch t.Density {
	case Comfortable:
		return 32
	case Compact:
		return 20
	default:
		return 26
	}
}

// Alpha returns c at a fraction of its opacity, for hover and disabled states
// that should not need a second colour token each.
func Alpha(c color.NRGBA, f float32) color.NRGBA {
	c.A = uint8(float32(c.A) * f)
	return c
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 0xff} }
