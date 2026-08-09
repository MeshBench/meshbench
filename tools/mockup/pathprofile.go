package main

import (
	"image/color"
	mm "math"

	m "github.com/A13xB0/meshcoresim/internal/mockup"
)

func sin(x float64) float64 { return mm.Sin(x) }
func cos(x float64) float64 { return mm.Cos(x) }

// noise gives a cheap deterministic speckle for the waterfall background.
func noise(x, y int) int {
	v := mm.Sin(float64(x)*12.9898+float64(y)*78.233) * 43758.5453
	return int((v - mm.Floor(v)) * 40)
}

// pathProfile renders the terrain cut-through — the panel that answers "why did
// it miss", and the one the brief specifically asked for.
func pathProfile() *m.Canvas {
	c := m.New(1180, 640)
	c.Fill(0, 0, 1180, 26, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(10, 18, "PATH PROFILE   GB7XYZ --> node-09     12.4 km     NO PATH", m.Text)
	c.Text(900, 18, "vertical exaggeration x8", m.Warn)

	left, right := 90, 1120
	base, top := 470, 90
	c.Rect(left-40, top-20, right-left+70, base-top+70, m.Border, m.Panel)

	// axes
	c.Line(left, top, left, base, m.Border, 0)
	c.Line(left, base, right, base, m.Border, 0)
	for i, lbl := range []string{"900", "700", "500", "300"} {
		y := top + i*(base-top)/4
		c.Text(left-38, y+4, lbl, m.Muted)
		c.Line(left, y, right, y, color.RGBA{0x22, 0x28, 0x2e, 0xff}, 4)
	}
	c.Text(left-38, base+4, "100", m.Muted)
	for i, lbl := range []string{"0 km", "4.2 km", "8.1 km", "12.4 km"} {
		x := left + i*(right-left)/3
		c.Text(x-16, base+22, lbl, m.Muted)
	}

	// terrain: two ridges, the first dominant
	ground := make([]int, right-left+1)
	for i := range ground {
		t := float64(i) / float64(len(ground)-1)
		h := 40 + 250*mm.Exp(-mm.Pow((t-0.34)/0.10, 2)) + 150*mm.Exp(-mm.Pow((t-0.66)/0.09, 2)) + 30*mm.Sin(t*18)
		ground[i] = base - int(h)
	}
	for i := 1; i < len(ground); i++ {
		x := left + i
		c.Line(x-1, ground[i-1], x, ground[i], m.Terrain, 0)
		c.Fill(x, ground[i], 1, base-ground[i], color.RGBA{0x24, 0x2e, 0x27, 0xff})
	}

	// antennas
	txY, rxY := base-70, base-60
	c.Line(left, txY, left, txY-26, m.Accent, 0)
	c.Text(left-30, txY-32, "TX 12 m", m.Accent)
	c.Line(right, rxY, right, rxY-20, m.Accent, 0)
	c.Text(right-58, rxY-26, "RX 8 m", m.Accent)

	// straight line of sight, and the k=4/3 curved path drawn separately
	c.Line(left, txY-26, right, rxY-20, m.Muted, 5)
	c.Text(560, 300, "straight line", m.Muted)
	prevX, prevY := left, txY-26
	for i := 0; i <= right-left; i += 6 {
		t := float64(i) / float64(right-left)
		y := float64(txY-26) + t*float64((rxY-20)-(txY-26))
		bulge := 26 * mm.Sin(t*mm.Pi) // k=4/3 earth bulge, exaggerated to match the vertical scale
		x := left + i
		yy := int(y + bulge)
		c.Line(prevX, prevY, x, yy, m.Accent, 0)
		prevX, prevY = x, yy
	}
	c.Text(560, 262, "line of sight (k=4/3)", m.Accent)

	// first Fresnel zone as an ellipse about the curved path
	for i := 0; i <= right-left; i += 3 {
		t := float64(i) / float64(right-left)
		y := float64(txY-26) + t*float64((rxY-20)-(txY-26)) + 26*mm.Sin(t*mm.Pi)
		r := 58 * mm.Sqrt(t*(1-t)) / 0.5 // r = sqrt(lambda d1 d2 / d)
		x := left + i
		c.Img.Set(x, int(y-r), m.Fresnel)
		c.Img.Set(x, int(y+r), m.Fresnel)
		// 60% clearance band
		c.Img.Set(x, int(y-r*0.6), color.RGBA{0x35, 0x55, 0x77, 0xff})
		c.Img.Set(x, int(y+r*0.6), color.RGBA{0x35, 0x55, 0x77, 0xff})
	}
	c.Text(300, 190, "1st Fresnel zone (60% band inner)", m.Fresnel)

	// diffracting edges, each labelled with its own loss
	e1 := left + int(0.34*float64(right-left))
	e2 := left + int(0.66*float64(right-left))
	c.Line(e1, top-10, e1, base, m.Bad, 3)
	c.Text(e1-70, top-16, "edge  -31.2 dB", m.Bad)
	c.Text(e1-58, top+6, "Meall Dubh 842 m", m.Text)
	c.Line(e2, top+60, e2, base, m.Warn, 3)
	c.Text(e2-62, top+54, "edge  -9.4 dB", m.Warn)

	// loss accumulation
	ly := base + 60
	c.Rect(left-40, ly, right-left+70, 96, m.Border, color.RGBA{0x18, 0x1d, 0x22, 0xff})
	c.Text(left-30, ly+20, "LOSS ACCUMULATION", m.Text)
	items := [][2]string{
		{"free space (12.4 km @ 869 MHz)", "-113.1 dB"},
		{"diffraction edge @ 4.2 km", "-31.2 dB"},
		{"diffraction edge @ 8.1 km", "-9.4 dB"},
	}
	for i, it := range items {
		c.Text(left-30, ly+42+i*16, it[0], m.Muted)
		c.Text(left+300, ly+42+i*16, it[1], m.Text)
	}
	c.Text(left+430, ly+42, "TOTAL PATH LOSS   153.7 dB", m.Text)
	c.Text(left+430, ly+58, "Rx -141.2 dBm   floor -117.0 dBm", m.Text)
	c.Text(left+430, ly+74, "MARGIN -24.2 dB    NO PATH", m.Bad)
	c.Text(left+760, ly+42, "Fix: move 400 m north", m.Good)
	c.Text(left+760, ly+58, "clears the 4.2 km ridge", m.Good)
	c.Text(left+760, ly+74, "predicted margin +6.8 dB", m.Good)
	return c
}
