package main

import m "github.com/A13xB0/meshcoresim/internal/mockup"

func workbench() *m.Canvas {
	c := m.New(1440, 940)

	// ── title bar ────────────────────────────────────────────────────────
	c.Fill(0, 0, 1440, 44, m.BgSurface)
	c.Line(0, 44, 1440, 44, m.Border, 1, 0)
	c.Dot(26, 22, 6, m.Accent)
	c.Text(42, 26, "MeshcoreSim", m.TextHi, m.SansBold, 13)
	x := 150
	for _, item := range []string{"File", "Scenario", "Firmware", "View", "Run"} {
		c.Text(x, 26, item, m.TextMid, m.Sans, 11)
		x += c.Measure(item, m.Sans, 11) + 26
	}
	c.TextRight(1290, 26, "Cairngorms winter", m.TextMid, m.Sans, 11)
	c.RoundRect(1300, 12, 122, 22, 6, m.BgRaised, m.BorderLit)
	c.Text(1312, 27, "Debug RF", m.TextHi, m.Sans, 10.5)
	c.Text(1400, 27, "v", m.TextLo, m.Sans, 10.5)

	// ── map ──────────────────────────────────────────────────────────────
	mx, my := card(c, 16, 60, 880, 520, "MAP / SCENE", "Terrain · Links · Patterns · Region")
	c.RoundRect(mx-8, my-12, 864, 452, 8, m.Sky, nil)

	// terrain bands
	for i := 0; i < 6; i++ {
		pts := [][2]int{}
		for px := 0; px <= 864; px += 8 {
			h := my + 40 + i*66 + int(30*m.Sin(float64(px)/150+float64(i)*1.3))
			pts = append(pts, [2]int{mx - 8 + px, h})
		}
		c.Polyline(pts, m.Alpha(m.Terrain, 0x70), 1.2, my+440, m.Alpha(m.Terrain, 0x14))
	}

	// region boundary
	c.RoundRect(mx+16, my+8, 800, 410, 12, nil, m.Alpha(m.Violet, 0x50))
	c.Text(mx+28, my+30, "region: Scotland  ·  +30 km RF margin", m.Alpha(m.Violet, 0xcc), m.Sans, 10)

	type nd struct {
		x, y int
		name string
		col  m.NRGBA
	}
	gb := nd{mx + 150, my + 150, "GB7XYZ", m.Good}
	n4 := nd{mx + 470, my + 230, "node-04", m.Good}
	n7 := nd{mx + 300, my + 350, "node-07", m.Warn}
	n9 := nd{mx + 700, my + 390, "node-09", m.Bad}

	// antenna pattern lobe
	lobe := [][2]int{}
	for a := 0; a <= 360; a += 5 {
		r := 46.0 + 30*m.Cos(float64(a)*3.14159/180)
		lobe = append(lobe, [2]int{
			gb.x + int(r*m.Cos(float64(a)*3.14159/180)),
			gb.y + int(r*m.Sin(float64(a)*3.14159/180)*0.6),
		})
	}
	c.Polyline(lobe, m.Alpha(m.Accent, 0x80), 1.4, 0, nil)
	c.Text(gb.x-64, gb.y-62, "antenna pattern", m.Alpha(m.Accent, 0xcc), m.Sans, 9.5)

	c.Line(gb.x, gb.y, n4.x, n4.y, m.Good, 2, 0)
	c.Line(gb.x, gb.y, n7.x, n7.y, m.Warn, 2, 7)
	c.Line(gb.x, gb.y, n9.x, n9.y, m.Bad, 1.6, 3)
	c.Text((gb.x+n4.x)/2-20, (gb.y+n4.y)/2-10, "−88 dBm", m.Good, m.Mono, 9.5)
	c.Text((gb.x+n7.x)/2-56, (gb.y+n7.y)/2+4, "marginal +2.5 dB", m.Warn, m.Mono, 9.5)
	c.Text((gb.x+n9.x)/2-30, (gb.y+n9.y)/2+16, "no path −24 dB", m.Bad, m.Mono, 9.5)

	for _, n := range []nd{gb, n4, n7, n9} {
		c.Dot(n.x, n.y, 9, m.Alpha(n.col, 0x40))
		c.Dot(n.x, n.y, 5, n.col)
		c.Text(n.x+14, n.y+4, n.name, m.TextHi, m.SansBold, 10)
	}
	// emitter
	c.Dot(mx+600, my+70, 6, m.Violet)
	c.Text(mx+614, my+74, "Cairn Gorm mast · 25 W PMR · +6 dB floor", m.Alpha(m.Violet, 0xdd), m.Sans, 9.5)

	// legend
	ly := my + 402
	c.Line(mx+30, ly, mx+58, ly, m.Good, 2, 0)
	c.Text(mx+64, ly+4, "decoded", m.TextMid, m.Sans, 9.5)
	c.Line(mx+140, ly, mx+168, ly, m.Warn, 2, 7)
	c.Text(mx+174, ly+4, "marginal", m.TextMid, m.Sans, 9.5)
	c.Line(mx+252, ly, mx+280, ly, m.Bad, 1.6, 3)
	c.Text(mx+286, ly+4, "no path", m.TextMid, m.Sans, 9.5)

	// ── inspector ────────────────────────────────────────────────────────
	ix, iy := card(c, 912, 60, 512, 520, "NODE INSPECTOR", "GB7XYZ")
	pill(c, ix, iy-16, "REPEATER", m.Accent)
	pill(c, ix+86, iy-16, "SOLAR", m.Good)
	rows := [][2]string{
		{"Board", "RAK 4631 · nRF52840 + SX1262"},
		{"Firmware", "repeater 1.17.0 (727fc05)"},
		{"Backend", "native"},
		{"Radio", "869.525 MHz · BW 250k · SF10 · CR5"},
		{"Antenna", "6 dBi collinear · az 0° · tilt 0°"},
		{"Position", "57.1204, −3.6712  ±60 m"},
		{"Height", "12.0 m AGL · ground 842 m"},
	}
	for i, r := range rows {
		kv(c, ix, iy+22+i*23, 78, r[0], r[1], m.TextHi)
	}
	// battery
	by := iy + 22 + len(rows)*23 + 10
	c.Text(ix, by+11, "Battery", m.TextLo, m.Sans, 10.5)
	c.RoundRect(ix+78, by, 200, 14, 7, m.BgInset, m.Border)
	c.RoundRect(ix+80, by+2, 140, 10, 5, m.Good, nil)
	c.Text(ix+290, by+11, "71%", m.TextHi, m.MonoBold, 10.5)
	c.Text(ix+330, by+11, "solar 12 W", m.TextLo, m.Sans, 10)

	// console
	cy := by + 34
	c.Text(ix, cy, "CONSOLE", m.TextLo, m.SansBold, 10)
	c.RoundRect(ix, cy+10, 480, 208, 8, m.BgInset, m.Border)
	logs := []struct {
		t, s string
		col  m.NRGBA
	}{
		{"12.401", "advert sent (zerohop)", m.TextMid},
		{"12.440", "rx node-04  −92.1 dBm  snr +2.1", m.TextHi},
		{"12.610", "tx ack → node-04", m.TextMid},
		{"12.812", "rx node-07  −104.3 dBm  snr −7.5", m.Warn},
		{"12.813", "dedup HIT — not relaying", m.TextLo},
	}
	for i, l := range logs {
		c.Text(ix+14, cy+34+i*22, l.t, m.TextLo, m.Mono, 9.5)
		c.Text(ix+70, cy+34+i*22, l.s, l.col, m.Mono, 9.5)
	}
	c.Text(ix+14, cy+34+len(logs)*22+4, "›", m.Accent, m.MonoBold, 10.5)

	// ── waterfall ────────────────────────────────────────────────────────
	wx, wy := card(c, 16, 596, 880, 296, "WATERFALL", "rx: node-07   ·   REC armed · 12 MB")
	c.RoundRect(wx-8, wy-12, 864, 196, 8, m.BgInset, nil)
	for px := 0; px < 856*m.Scale; px++ {
		for py := 0; py < 188*m.Scale; py++ {
			sx, sy := px/m.Scale, py/m.Scale
			v := noise(sx, sy)
			if sx > 150 && sx < 400 && sy > 46 && sy < 78 {
				v += 150
			}
			if sx > 320 && sx < 560 && sy > 62 && sy < 94 {
				v += 195
			}
			if sy > 140 && sy < 150 {
				v += 80
			}
			if v > 255 {
				v = 255
			}
			c.Img.Set((wx-4)*m.Scale+px, (wy-8)*m.Scale+py, m.NRGBA{uint8(v / 5), uint8(v / 2), uint8(v), 0xff})
		}
	}
	c.Text(wx+330, wy+110, "overlap 41 ms — collision", m.Warn, m.SansBold, 10)
	c.Text(wx+560, wy+150, "mast 868.35 continuous", m.Alpha(m.Violet, 0xdd), m.Sans, 9.5)
	c.Text(wx, wy+206, "click a burst → symbol view:  peak bin 412 · 2nd 198 · ratio 6.2 dB", m.TextMid, m.Mono, 10)
	c.Text(wx, wy+228, "node-04 CAPTURED — GB7XYZ lost by 6.2 dB", m.Good, m.SansBold, 10.5)

	// ── run control ──────────────────────────────────────────────────────
	rx, ry := card(c, 912, 596, 512, 296, "RUN CONTROL", "")
	bx2 := rx
	for _, b := range []string{"Play", "Pause", "Step event", "Step symbol"} {
		w := c.Measure(b, m.Sans, 10.5) + 24
		c.RoundRect(bx2, ry-18, w, 28, 6, m.BgRaised, m.BorderLit)
		c.Text(bx2+12, ry+1, b, m.TextHi, m.Sans, 10.5)
		bx2 += w + 8
	}
	kv(c, rx, ry+42, 78, "Speed", "×8", m.TextHi)
	pill(c, rx+130, ry+30, "1× LOCKED — companion attached", m.Warn)
	kv(c, rx, ry+68, 78, "Seed", "4417", m.TextHi)
	c.Text(rx+140, ry+68, "randomise · lock", m.Accent, m.Sans, 10)
	kv(c, rx, ry+94, 78, "Nodes", "20  ·  1 emulated (8× slower)", m.TextMid)

	c.RoundRect(rx, ry+118, 480, 74, 8, m.Alpha(m.Warn, 0x14), m.Alpha(m.Warn, 0x60))
	c.Text(rx+14, ry+142, "Model is kinder than the air", m.Warn, m.SansBold, 10.5)
	c.Text(rx+14, ry+162, "No multipath, body loss or oscillator error.", m.TextMid, m.Sans, 10)
	c.Text(rx+14, ry+180, "Real links perform worse than shown.", m.TextMid, m.Sans, 10)

	// ── footer ───────────────────────────────────────────────────────────
	c.Line(0, 906, 1440, 906, m.Border, 1, 0)
	c.Text(16, 926, "fw repeater 1.17.0 727fc05   ·   seed 4417   ·   terrain z11   ·   model hamreach-coverage-1   ·   region Scotland", m.TextLo, m.Mono, 9.5)
	return c
}
