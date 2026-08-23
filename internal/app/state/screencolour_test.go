package state

import "testing"

// Is a colour pixel read the way the panel wrote it?
//
// The frame carries each pixel low byte first. Reading it the other way round
// leaves black and white exactly as they should be and turns everything
// between them into a different colour - so a boot logo looks perfect and an
// interface looks like a fault in the panel. That is how this survived being
// looked at.
func TestAColourPixelIsReadLowByteFirst(t *testing.T) {
	for _, c := range []struct {
		name    string
		v       uint16
		r, g, b uint8
	}{
		{"red", 0xF800, 0xFF, 0, 0},
		{"green", 0x07E0, 0, 0xFF, 0},
		{"blue", 0x001F, 0, 0, 0xFF},
		{"white", 0xFFFF, 0xFF, 0xFF, 0xFF},
		{"black", 0x0000, 0, 0, 0},
	} {
		s := &Screen{Width: 1, Height: 1, BPP: 16,
			Bits: []byte{byte(c.v), byte(c.v >> 8)}}
		r, g, b, ok := s.At(0, 0)
		if !ok || r != c.r || g != c.g || b != c.b {
			t.Errorf("%s (%#04x) read back as %d,%d,%d - want %d,%d,%d",
				c.name, c.v, r, g, b, c.r, c.g, c.b)
		}
	}
}
