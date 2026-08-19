package main

import m "github.com/MeshBench/meshbench/internal/mockup"

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

// coverage shows a full-bounds best-server raster — the whole map coloured by
// link margin, as HopReach's coverage.Raster does — with node markers on top and
// one selected.
func coverage() *m.Canvas {
	c := m.New(1400, 840)
	c.Fill(0, 0, 1400, 44, m.BgSurface)
	c.Line(0, 44, 1400, 44, m.Border, 1, 0)
	c.Text(20, 27, "COVERAGE", m.TextHi, m.SansBold, 12)
	c.Text(130, 27, "best-server raster · whole region · 20 m/px · SF10 · worst case", m.TextMid, m.Sans, 11)
	pill(c, 620, 13, "GPU · 6.4 s", m.Good)

	mx, my := card(c, 16, 60, 1060, 760, "MAP", "raster covers the full bounds, not a radius")
	W, H := 1044, 700
	ox, oy := mx-8, my-12
	c.RoundRect(ox, oy, W, H, 8, m.Sky, nil)

	// Sites contributing to the best-server field.
	type site struct{ x, y, erp int }
	sites := []site{
		{ox + 260, oy + 200, 230}, {ox + 640, oy + 300, 270},
		{ox + 420, oy + 520, 190}, {ox + 880, oy + 560, 200},
	}

	// Full-bounds raster: every cell gets the best margin from any site, with
	// terrain shadowing, so the field is continuous and irregular rather than a
	// set of circles.
	for px := 0; px < W-4; px += 2 {
		for py := 0; py < H-4; py += 2 {
			gx, gy := ox+2+px, oy+2+py
			best := -99.0
			// Terrain: ridges carve shadows, so the field is irregular rather than
			// a set of circles. Shared across sites — it is the same ground.
			ridge := m.Sin(float64(gx)/110)*m.Cos(float64(gy)/86) + 0.55*m.Sin(float64(gx+gy)/150) + 0.3*m.Cos(float64(gx-gy)/95)
			shadow := 0.0
			if ridge < 0.10 {
				shadow = (0.10 - ridge) * 15
			}
			for _, s := range sites {
				dx, dy := float64(gx-s.x), float64(gy-s.y)
				d := m.Sqrt(dx*dx+dy*dy) + 30
				margin := float64(s.erp)/10 - 22*m.Log10(d/40) - shadow
				if margin > best {
					best = margin
				}
			}
			var col m.NRGBA
			switch {
			case best >= 12:
				col = m.Alpha(m.Good, 0xcc)
			case best >= 6:
				col = m.Alpha(m.NRGBA{R: 0x86, G: 0xc8, B: 0x3a, A: 0xff}, 0xbb)
			case best >= 2:
				col = m.Alpha(m.Warn, 0xb0)
			case best >= 0:
				col = m.Alpha(m.NRGBA{R: 0xe8, G: 0x6a, B: 0x2a, A: 0xff}, 0xa0)
			default:
				continue // no service — basemap shows through
			}
			c.Fill(gx, gy, 2, 2, col)
		}
	}

	// nodes: repeaters and companions, distinguished by shape as well as colour
	type nodeM struct {
		x, y     int
		name     string
		repeater bool
		selected bool
	}
	nodes := []nodeM{
		{sites[0].x, sites[0].y, "GB7XYZ", true, false},
		{sites[1].x, sites[1].y, "GB7ABC", true, true},
		{sites[2].x, sites[2].y, "GB7DEF", true, false},
		{sites[3].x, sites[3].y, "GB7GHI", true, false},
		{ox + 380, oy + 350, "node-04", false, false},
		{ox + 760, oy + 190, "node-07", false, false},
		{ox + 540, oy + 640, "node-12", false, false},
	}
	for _, n := range nodes {
		if n.repeater {
			if n.selected {
				c.Dot(n.x, n.y, 16, m.Alpha(m.TextHi, 0x30))
			}
			c.Dot(n.x, n.y, 9, m.BgDeep)
			c.Dot(n.x, n.y, 7, m.TextHi)
			c.Dot(n.x, n.y, 3, m.BgDeep)
		} else {
			c.RoundRect(n.x-5, n.y-5, 10, 10, 2, m.BgDeep, m.TextHi)
		}
		c.Text(n.x+13, n.y+4, n.name, m.TextHi, m.SansBold, 9.5)
	}

	// selection popover for the clicked repeater
	sx, sy := sites[1].x+26, sites[1].y-96
	c.RoundRect(sx, sy, 250, 176, 8, m.Alpha(m.BgDeep, 0xf2), m.BorderLit)
	c.Text(sx+14, sy+26, "GB7ABC", m.TextHi, m.SansBold, 12)
	pill(c, sx+150, sy+12, "REPEATER", m.Accent)
	c.Text(sx+14, sy+50, "RAK 4631 · 6 dBi · 12 m AGL", m.TextMid, m.Sans, 9.5)
	c.Text(sx+14, sy+70, "869.525 · SF10 · +22 dBm", m.TextMid, m.Mono, 9.5)
	c.Line(sx+14, sy+82, sx+236, sy+82, m.Border, 1, 0)
	c.Text(sx+14, sy+102, "serves 38% of region", m.TextHi, m.Sans, 10)
	c.Text(sx+14, sy+122, "sole server for 11%", m.Warn, m.Sans, 10)
	c.Text(sx+14, sy+148, "Show only this node's raster", m.Accent, m.Sans, 9.5)
	c.Text(sx+14, sy+166, "Coverage · Path profile · Console", m.Accent, m.Sans, 9.5)

	// legend
	c.RoundRect(ox+16, oy+H-92, 330, 76, 8, m.Alpha(m.BgDeep, 0xdd), m.Border)
	c.Text(ox+30, oy+H-68, "link margin (worst case)", m.TextLo, m.SansBold, 9.5)
	steps := []struct {
		col   m.NRGBA
		label string
	}{
		{m.Good, "12+"}, {m.NRGBA{R: 0x86, G: 0xc8, B: 0x3a, A: 0xff}, "6"}, {m.Warn, "2"},
		{m.NRGBA{R: 0xe8, G: 0x6a, B: 0x2a, A: 0xff}, "0"},
	}
	for i, s := range steps {
		c.Fill(ox+30+i*72, oy+H-56, 68, 12, m.Alpha(s.col, 0xcc))
		c.Text(ox+30+i*72, oy+H-36, s.label, m.TextLo, m.Mono, 9)
	}
	c.Text(ox+300, oy+H-36, "dB", m.TextLo, m.Mono, 9)

	// ── layers ───────────────────────────────────────────────────────────
	lx, ly := card(c, 1092, 60, 292, 760, "LAYERS", "")
	c.Text(lx, ly-14, "RASTER MODE", m.TextLo, m.SansBold, 9.5)
	for i, mode := range []string{"Best server (whole region)", "Selected node only", "Gap — served by nobody", "Redundancy — 2+ servers"} {
		yy := ly + 10 + i*26
		sel := i == 0
		c.Dot(lx+7, yy-4, 6, m.Alpha(m.Accent, 0x40))
		if sel {
			c.Dot(lx+7, yy-4, 3, m.Accent)
		}
		col := m.TextMid
		if sel {
			col = m.TextHi
		}
		c.Text(lx+26, yy, mode, col, m.Sans, 10)
	}

	ny := ly + 130
	c.Line(lx, ny-16, lx+260, ny-16, m.Border, 1, 0)
	c.Text(lx, ny, "NODES", m.TextLo, m.SansBold, 9.5)
	c.Text(lx+200, ny, "4 rptr", m.TextLo, m.Mono, 9)
	list := []struct {
		name string
		kind string
		pct  string
		sel  bool
	}{
		{"GB7XYZ", "repeater", "31%", false},
		{"GB7ABC", "repeater", "38%", true},
		{"GB7DEF", "repeater", "22%", false},
		{"GB7GHI", "repeater", "26%", false},
		{"node-04", "companion", "—", false},
		{"node-07", "companion", "—", false},
		{"node-12", "companion", "—", false},
	}
	for i, n := range list {
		yy := ny + 22 + i*30
		if n.sel {
			c.RoundRect(lx-8, yy-14, 276, 28, 5, m.Alpha(m.Accent, 0x22), m.Alpha(m.Accent, 0x70))
		}
		if n.kind == "repeater" {
			c.Dot(lx+6, yy-3, 5, m.TextHi)
		} else {
			c.RoundRect(lx+2, yy-8, 9, 9, 2, m.BgInset, m.TextMid)
		}
		col := m.TextHi
		if n.kind == "companion" {
			col = m.TextMid
		}
		c.Text(lx+22, yy, n.name, col, m.Sans, 10.5)
		c.TextRight(lx+240, yy, n.pct, m.TextLo, m.Mono, 9.5)
	}
	c.Text(lx, ny+22+len(list)*30+10, "click any node on the map or here", m.TextLo, m.Sans, 9.5)

	ry := ny + 22 + len(list)*30 + 34
	c.Line(lx, ry, lx+260, ry, m.Border, 1, 0)
	c.Text(lx, ry+22, "RESOLUTION", m.TextLo, m.SansBold, 9.5)
	c.Text(lx, ry+44, "20 m/px", m.TextHi, m.Mono, 11)
	c.TextRight(lx+260, ry+44, "3.1 M cells", m.TextLo, m.Mono, 10)
	c.RoundRect(lx, ry+56, 260, 6, 3, m.BgInset, nil)
	c.RoundRect(lx, ry+56, 160, 6, 3, m.Accent, nil)
	c.Text(lx, ry+84, "local GPU · 6.4 s", m.Good, m.Sans, 10)
	c.Text(lx, ry+104, "remote worker · 2.2 s", m.Good, m.Sans, 10)
	c.Text(lx, ry+124, "CPU reference · 214 s", m.TextLo, m.Sans, 9.5)

	wy := ry + 148
	c.RoundRect(lx, wy, 260, 96, 8, m.Alpha(m.Warn, 0x12), m.Alpha(m.Warn, 0x60))
	c.Text(lx+12, wy+24, "Raster is a prediction", m.Warn, m.SansBold, 10)
	c.Text(lx+12, wy+46, "Terrain z11 ≈ 30 m/px, so", m.TextMid, m.Sans, 9.5)
	c.Text(lx+12, wy+64, "buildings, hedges and vans", m.TextMid, m.Sans, 9.5)
	c.Text(lx+12, wy+82, "are invisible to it.", m.TextMid, m.Sans, 9.5)
	return c
}
