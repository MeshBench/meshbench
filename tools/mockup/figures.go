package main

import m "github.com/MeshBench/meshbench/internal/mockup"

// noise gives deterministic speckle for the waterfall background.
func noise(x, y int) int {
	v := m.Sin(float64(x)*12.9898+float64(y)*78.233) * 43758.5453
	return int((v - m.Floor(v)) * 42)
}

func pathProfile() *m.Canvas {
	c := m.New(1320, 760)
	c.Fill(0, 0, 1320, 44, m.BgSurface)
	c.Line(0, 44, 1320, 44, m.Border, 1, 0)
	c.Text(20, 27, "PATH PROFILE", m.TextHi, m.SansBold, 12)
	c.Text(140, 27, "GB7XYZ → node-09", m.TextMid, m.Sans, 11.5)
	c.Text(300, 27, "12.4 km", m.TextMid, m.Mono, 11)
	pill(c, 380, 13, "NO PATH", m.Bad)
	c.TextRight(1300, 27, "vertical exaggeration ×8", m.TextLo, m.Sans, 10.5)

	px, py := card(c, 16, 60, 1288, 480, "TERRAIN CUT-THROUGH", "")
	left, right := px+52, px+1216
	top, base := py+10, py+380

	for i, lbl := range []string{"900 m", "700 m", "500 m", "300 m", "100 m"} {
		y := top + i*(base-top)/4
		c.TextRight(left-12, y+4, lbl, m.TextLo, m.Mono, 9.5)
		c.Line(left, y, right, y, m.Alpha(m.Border, 0x80), 1, 6)
	}
	for i, lbl := range []string{"0 km", "4.2 km", "8.1 km", "12.4 km"} {
		x := left + i*(right-left)/3
		c.Text(x-18, base+24, lbl, m.TextLo, m.Mono, 9.5)
	}

	ground := [][2]int{}
	for i := 0; i <= right-left; i += 3 {
		t := float64(i) / float64(right-left)
		h := 60 + 250*m.Exp(-m.Pow((t-0.34)/0.10, 2)) + 150*m.Exp(-m.Pow((t-0.66)/0.09, 2)) + 24*m.Sin(t*16)
		ground = append(ground, [2]int{left + i, base - int(h)})
	}
	c.Polyline(ground, m.Alpha(m.Terrain, 0xff), 2, base, m.Alpha(m.Terrain, 0x66))

	txY, rxY := base-96, base-84
	c.Line(left, base-60, left, txY, m.Accent, 2, 0)
	c.Dot(left, txY, 4, m.Accent)
	c.Text(left+8, txY-8, "TX 12 m AGL", m.Accent, m.Sans, 9.5)
	c.Line(right, base-52, right, rxY, m.Accent, 2, 0)
	c.Dot(right, rxY, 4, m.Accent)
	c.TextRight(right-8, rxY-8, "RX 8 m AGL", m.Accent, m.Sans, 9.5)

	c.Line(left, txY, right, rxY, m.Alpha(m.TextLo, 0xaa), 1.4, 8)
	c.Text(left+430, txY-70, "straight line", m.TextLo, m.Sans, 9.5)

	los := [][2]int{}
	fUp, fLo := [][2]int{}, [][2]int{}
	for i := 0; i <= right-left; i += 4 {
		t := float64(i) / float64(right-left)
		y := float64(txY) + t*float64(rxY-txY) + 30*m.Sin(t*3.14159)
		r := 66 * m.Sqrt(t*(1-t)) / 0.5
		los = append(los, [2]int{left + i, int(y)})
		fUp = append(fUp, [2]int{left + i, int(y - r)})
		fLo = append(fLo, [2]int{left + i, int(y + r)})
	}
	c.Polyline(fUp, m.Alpha(m.Accent, 0x50), 1.2, 0, nil)
	c.Polyline(fLo, m.Alpha(m.Accent, 0x50), 1.2, 0, nil)
	c.Polyline(los, m.Accent, 2, 0, nil)
	c.Text(left+500, txY-38, "line of sight (k = 4/3)", m.Accent, m.SansBold, 10)
	c.Text(left+180, txY-104, "1st Fresnel zone", m.Alpha(m.Accent, 0xaa), m.Sans, 9.5)

	e1 := left + int(0.34*float64(right-left))
	e2 := left + int(0.66*float64(right-left))
	c.Line(e1, top-6, e1, base, m.Bad, 1.4, 5)
	c.RoundRect(e1-66, top-30, 132, 22, 6, m.Alpha(m.Bad, 0x22), m.Alpha(m.Bad, 0x88))
	c.Text(e1-54, top-14, "edge  −31.2 dB", m.Bad, m.MonoBold, 10)
	c.Text(e1-52, top+16, "Meall Dubh 842 m", m.TextHi, m.Sans, 9.5)
	c.Line(e2, top+70, e2, base, m.Warn, 1.4, 5)
	c.RoundRect(e2-58, top+48, 120, 22, 6, m.Alpha(m.Warn, 0x22), m.Alpha(m.Warn, 0x88))
	c.Text(e2-46, top+64, "edge  −9.4 dB", m.Warn, m.MonoBold, 10)

	bx, by := card(c, 16, 556, 1288, 188, "LOSS ACCUMULATION", "")
	items := [][2]string{
		{"free space  12.4 km @ 869 MHz", "−113.1 dB"},
		{"diffraction edge @ 4.2 km", "−31.2 dB"},
		{"diffraction edge @ 8.1 km", "−9.4 dB"},
	}
	for i, it := range items {
		c.Text(bx, by+i*26, it[0], m.TextMid, m.Sans, 10.5)
		c.TextRight(bx+430, by+i*26, it[1], m.TextHi, m.Mono, 10.5)
	}
	c.Line(bx+470, by-16, bx+470, by+82, m.Border, 1, 0)
	c.Text(bx+500, by, "TOTAL PATH LOSS", m.TextLo, m.Sans, 10)
	c.Text(bx+500, by+24, "153.7 dB", m.TextHi, m.MonoBold, 15)
	c.Text(bx+660, by, "RECEIVED", m.TextLo, m.Sans, 10)
	c.Text(bx+660, by+24, "−141.2 dBm", m.TextHi, m.MonoBold, 15)
	c.Text(bx+660, by+50, "floor −117.0 dBm", m.TextLo, m.Mono, 9.5)
	c.Text(bx+850, by, "MARGIN", m.TextLo, m.Sans, 10)
	c.Text(bx+850, by+24, "−24.2 dB", m.Bad, m.MonoBold, 15)

	c.RoundRect(bx+1000, by-24, 250, 108, 8, m.Alpha(m.Good, 0x12), m.Alpha(m.Good, 0x60))
	c.Text(bx+1016, by-2, "SUGGESTED FIX", m.Good, m.SansBold, 10)
	c.Text(bx+1016, by+22, "Move 400 m north", m.TextHi, m.Sans, 10.5)
	c.Text(bx+1016, by+42, "clears the 4.2 km ridge", m.TextMid, m.Sans, 10)
	c.Text(bx+1016, by+64, "predicted margin +6.8 dB", m.Good, m.Mono, 10)
	return c
}
