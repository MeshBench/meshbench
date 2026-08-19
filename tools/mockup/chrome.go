package main

import m "github.com/MeshBench/meshbench/tools/internal/mockup"

// card draws a titled surface with a subtle raised header, and returns the
// content origin. Every panel in the app uses this so spacing stays consistent.
func card(c *m.Canvas, x, y, w, h int, title, right string) (int, int) {
	c.RoundRect(x, y, w, h, 10, m.BgSurface, m.Border)
	c.Text(x+16, y+24, title, m.TextHi, m.SansBold, 11.5)
	if right != "" {
		c.TextRight(x+w-16, y+24, right, m.TextLo, m.Sans, 10.5)
	}
	c.Line(x+1, y+38, x+w-1, y+38, m.Border, 1, 0)
	return x + 16, y + 62
}

// pill draws a small status chip. Shape plus text carries the meaning; colour is
// secondary, because engineers include colourblind engineers.
func pill(c *m.Canvas, x, y int, text string, col m.NRGBA) int {
	w := c.Measure(text, m.SansBold, 9.5) + 18
	c.RoundRect(x, y, w, 18, 9, m.Alpha(col, 0x28), m.Alpha(col, 0x90))
	c.Text(x+9, y+13, text, col, m.SansBold, 9.5)
	return w
}

// kv draws a label/value row, the inspector's basic unit.
func kv(c *m.Canvas, x, y, labelW int, k, v string, col m.NRGBA) {
	c.Text(x, y, k, m.TextLo, m.Sans, 10.5)
	c.Text(x+labelW, y, v, col, m.Mono, 10.5)
}
