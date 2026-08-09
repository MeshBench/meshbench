package main

import m "github.com/A13xB0/meshcoresim/internal/mockup"

func interference() *m.Canvas {
	c := m.New(1160, 620)
	c.Fill(0, 0, 1160, 44, m.BgSurface)
	c.Line(0, 44, 1160, 44, m.Border, 1, 0)
	c.Text(20, 27, "INTERFERENCE", m.TextHi, m.SansBold, 12)
	c.Text(160, 27, "8 emitters loaded · 214 culled below −140 dBm", m.TextMid, m.Sans, 11)

	x, y := card(c, 16, 60, 1128, 240, "EXTERNAL EMITTERS", "Ofcom import · manual entry")
	cols := []int{0, 190, 330, 470, 560, 660, 790}
	for i, h := range []string{"emitter", "type", "frequency", "ERP", "height", "duty", "at node-07"} {
		c.Text(x+cols[i], y-14, h, m.TextLo, m.SansBold, 9.5)
	}
	c.Line(x-8, y-4, x+1112, y-4, m.Border, 1, 0)
	rows := [][7]string{
		{"Cairn Gorm", "PMR mast", "868.35 MHz", "25 W", "45 m", "30%", "+4.1 dB"},
		{"Aviemore relay", "Broadcast", "out of band", "2 kW", "80 m", "cont.", "+0.3 dB"},
		{"(manual) paging", "Paging", "869.50 MHz", "25 W", "20 m", "bursty", "+1.6 dB"},
	}
	for i, r := range rows {
		yy := y + 20 + i*32
		if i%2 == 0 {
			c.RoundRect(x-8, yy-18, 1112, 30, 4, m.Alpha(m.BgRaised, 0x80), nil)
		}
		c.Text(x, yy, r[0], m.TextHi, m.Sans, 10.5)
		for j := 1; j < 6; j++ {
			c.Text(x+cols[j], yy, r[j], m.TextMid, m.Mono, 10)
		}
		c.Text(x+cols[6], yy, r[6], m.Warn, m.MonoBold, 10.5)
	}

	ex, ey := card(c, 16, 316, 556, 168, "EFFECT AT node-07", "")
	c.Text(ex, ey, "thermal floor", m.TextMid, m.Sans, 10.5)
	c.TextRight(ex+300, ey, "−117.0 dBm", m.TextHi, m.Mono, 10.5)
	c.Text(ex, ey+26, "with interference", m.TextMid, m.Sans, 10.5)
	c.TextRight(ex+300, ey+26, "−111.0 dBm", m.Warn, m.MonoBold, 10.5)
	c.Line(ex, ey+42, ex+400, ey+42, m.Border, 1, 0)
	c.Text(ex, ey+66, "sensitivity lost", m.TextHi, m.SansBold, 11)
	c.Text(ex+300, ey+66, "6.0 dB", m.Bad, m.MonoBold, 14)
	c.Text(ex, ey+92, "≈ half your usable range", m.Bad, m.Sans, 10.5)

	fx, fy := card(c, 588, 316, 556, 168, "RX FILTER  (stretch)", "cavity 868 ± 2 MHz")
	c.Text(fx, fy, "insertion loss", m.TextMid, m.Sans, 10.5)
	c.TextRight(fx+300, fy, "−1.1 dB", m.TextHi, m.Mono, 10.5)
	c.Text(fx, fy+26, "rejection @ 869.5", m.TextMid, m.Sans, 10.5)
	c.TextRight(fx+300, fy+26, "−38.0 dB", m.Good, m.Mono, 10.5)
	c.Line(fx, fy+42, fx+400, fy+42, m.Border, 1, 0)
	c.Text(fx, fy+66, "resulting floor", m.TextHi, m.SansBold, 11)
	c.Text(fx+300, fy+66, "−116.2 dBm", m.Good, m.MonoBold, 14)
	pill(c, fx, fy+86, "FIXABLE — BUY THE CAVITY", m.Good)

	c.RoundRect(16, 500, 1128, 104, 10, m.Alpha(m.Accent, 0x10), m.Alpha(m.Accent, 0x50))
	c.Text(36, 530, "THE ANSWER THAT SAVES MONEY", m.Accent, m.SansBold, 11)
	c.Text(36, 556, "A filter only helps out-of-band interference. If the interferer sits inside your own passband,", m.TextMid, m.Sans, 10.5)
	c.Text(36, 578, "no filter will help — and the tool says so plainly rather than showing a tempting marginal gain.", m.TextMid, m.Sans, 10.5)
	return c
}

// coverage shows the raster map: per-repeater layers, or a combined view.
func coverage() *m.Canvas {
	c := m.New(1320, 800)
	c.Fill(0, 0, 1320, 44, m.BgSurface)
	c.Line(0, 44, 1320, 44, m.Border, 1, 0)
	c.Text(20, 27, "COVERAGE", m.TextHi, m.SansBold, 12)
	c.Text(130, 27, "GPU raster · 20 m/px · SF10 · both directions", m.TextMid, m.Sans, 11)
	pill(c, 460, 13, "COMBINED", m.Accent)

	mx, my := card(c, 16, 60, 1000, 720, "MAP", "raster overlay")
	c.RoundRect(mx-8, my-12, 984, 664, 8, m.Sky, nil)

	// combined coverage field from three sources
	type src struct {
		x, y, r int
		col     m.NRGBA
	}
	srcs := []src{
		{mx + 250, my + 200, 210, m.Good},
		{mx + 620, my + 300, 250, m.Accent},
		{mx + 400, my + 480, 180, m.Violet},
	}
	for px := 0; px < 984; px += 2 {
		for py := 0; py < 664; py += 2 {
			gx, gy := mx-8+px, my-12+py
			best, bcol := 0.0, m.NRGBA{}
			for _, s := range srcs {
				dx, dy := float64(gx-s.x), float64(gy-s.y)
				d := m.Sqrt(dx*dx + dy*dy)
				// terrain-shadowed falloff, not a circle
				shade := 1 + 0.45*m.Sin(float64(gx)/60)*m.Cos(float64(gy)/70)
				v := 1 - d/(float64(s.r)*shade)
				if v > best {
					best, bcol = v, s.col
				}
			}
			if best > 0 {
				a := uint8(30 + 150*best)
				if best > 0.62 {
					a = uint8(150 + 80*(best-0.62)/0.38)
				}
				c.Fill(gx, gy, 2, 2, m.Alpha(bcol, a))
			}
		}
	}
	for _, s := range srcs {
		c.Dot(s.x, s.y, 8, m.Alpha(s.col, 0x60))
		c.Dot(s.x, s.y, 4, m.TextHi)
	}
	c.Text(srcs[0].x+14, srcs[0].y+4, "GB7XYZ", m.TextHi, m.SansBold, 10)
	c.Text(srcs[1].x+14, srcs[1].y+4, "GB7ABC", m.TextHi, m.SansBold, 10)
	c.Text(srcs[2].x+14, srcs[2].y+4, "node-04", m.TextHi, m.SansBold, 10)

	// scale legend
	c.RoundRect(mx+700, my+560, 260, 76, 8, m.Alpha(m.BgDeep, 0xdd), m.Border)
	c.Text(mx+714, my+584, "received signal", m.TextLo, m.SansBold, 9.5)
	for i := 0; i < 20; i++ {
		c.Fill(mx+714+i*11, my+596, 11, 12, m.Alpha(m.Accent, uint8(30+i*11)))
	}
	c.Text(mx+714, my+624, "−137", m.TextLo, m.Mono, 9)
	c.TextRight(mx+934, my+624, "−90 dBm", m.TextLo, m.Mono, 9)

	// ── layer panel ──────────────────────────────────────────────────────
	lx, ly := card(c, 1032, 60, 272, 720, "LAYERS", "")
	c.Text(lx, ly-14, "COVERAGE RASTERS", m.TextLo, m.SansBold, 9.5)
	layers := []struct {
		name  string
		on    bool
		col   m.NRGBA
		state string
	}{
		{"Combined (best server)", true, m.Accent, "cached"},
		{"GB7XYZ", true, m.Good, "cached"},
		{"GB7ABC", true, m.Accent, "cached"},
		{"node-04", true, m.Violet, "computing 62%"},
		{"node-07", false, m.TextLo, "—"},
		{"node-09", false, m.TextLo, "—"},
	}
	for i, l := range layers {
		yy := ly + 12 + i*34
		box := m.BgInset
		if l.on {
			box = m.Alpha(l.col, 0x40)
		}
		c.RoundRect(lx, yy-11, 16, 16, 4, box, m.BorderLit)
		if l.on {
			c.Line(lx+4, yy-3, lx+7, yy+1, l.col, 2, 0)
			c.Line(lx+7, yy+1, lx+12, yy-7, l.col, 2, 0)
		}
		col := m.TextHi
		if !l.on {
			col = m.TextLo
		}
		c.Text(lx+26, yy+3, l.name, col, m.Sans, 10.5)
		c.Text(lx+26, yy+18, l.state, m.TextLo, m.Mono, 8.5)
	}

	oy := ly + 12 + len(layers)*34 + 16
	c.Line(lx, oy, lx+240, oy, m.Border, 1, 0)
	c.Text(lx, oy+22, "DIRECTION", m.TextLo, m.SansBold, 9.5)
	for i, d := range []string{"outbound (you → them)", "inbound (them → you)", "both — worst case"} {
		yy := oy + 42 + i*24
		sel := i == 2
		c.Dot(lx+7, yy-4, 6, m.Alpha(m.Accent, 0x40))
		if sel {
			c.Dot(lx+7, yy-4, 3, m.Accent)
		}
		col := m.TextMid
		if sel {
			col = m.TextHi
		}
		c.Text(lx+26, yy, d, col, m.Sans, 10)
	}

	ry := oy + 130
	c.Line(lx, ry, lx+240, ry, m.Border, 1, 0)
	c.Text(lx, ry+22, "RESOLUTION", m.TextLo, m.SansBold, 9.5)
	c.Text(lx, ry+44, "20 m/px", m.TextHi, m.Mono, 11)
	c.TextRight(lx+240, ry+44, "1.4 M cells", m.TextLo, m.Mono, 10)
	c.RoundRect(lx, ry+56, 240, 6, 3, m.BgInset, nil)
	c.RoundRect(lx, ry+56, 150, 6, 3, m.Accent, nil)
	c.Text(lx, ry+84, "GPU · 2.1 s per repeater", m.Good, m.Sans, 10)
	c.Text(lx, ry+104, "CPU reference · 46 s", m.TextLo, m.Sans, 9.5)

	wy2 := ry + 134
	c.RoundRect(lx, wy2, 240, 92, 8, m.Alpha(m.Warn, 0x12), m.Alpha(m.Warn, 0x60))
	c.Text(lx+12, wy2+24, "Raster is a prediction", m.Warn, m.SansBold, 10)
	c.Text(lx+12, wy2+46, "Terrain z11 ≈ 30 m/px, so", m.TextMid, m.Sans, 9.5)
	c.Text(lx+12, wy2+64, "buildings and hedges are", m.TextMid, m.Sans, 9.5)
	c.Text(lx+12, wy2+82, "invisible to it.", m.TextMid, m.Sans, 9.5)
	return c
}
