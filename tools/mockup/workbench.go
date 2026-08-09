package main

import (
	"image/color"

	m "github.com/A13xB0/meshcoresim/internal/mockup"
)

// workbench renders the full application shell: map, inspector, waterfall and
// run control, as described in the UX spec.
func workbench() *m.Canvas {
	c := m.New(1280, 760)

	// title bar
	c.Fill(0, 0, 1280, 26, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(10, 18, "MeshcoreSim", m.Text)
	c.Text(120, 18, "File  Scenario  Firmware  View  Run", m.Muted)
	c.Text(900, 18, "Scenario: Cairngorms winter", m.Muted)
	c.Text(1130, 18, "[Debug RF v]", m.Accent)

	// ---- map -------------------------------------------------------------
	mx, my := c.Panel(8, 34, 700, 430, "MAP / SCENE                    [Terrain] [Links] [Patterns] [Region]")
	_ = mx
	_ = my
	c.Fill(16, 70, 684, 386, color.RGBA{0x18, 0x1e, 0x22, 0xff})

	// terrain contours, suggestive rather than real
	for i := 0; i < 7; i++ {
		y := 120 + i*40
		prev := 0
		for x := 20; x < 700; x += 4 {
			h := y + int(28*sin(float64(x)/70+float64(i)))
			if prev != 0 {
				c.Line(x-4, prev, x, h, color.RGBA{0x2a, 0x33, 0x2d, 0xff}, 0)
			}
			prev = h
		}
	}

	// nodes and links
	type node struct {
		x, y int
		name string
		col  color.RGBA
	}
	gb := node{210, 180, "GB7XYZ", m.Good}
	n4 := node{430, 250, "node-04", m.Good}
	n7 := node{330, 360, "node-07", m.Warn}
	n9 := node{600, 400, "node-09", m.Bad}

	c.Line(gb.x, gb.y, n4.x, n4.y, m.Good, 0) // decoded, solid
	c.Line(gb.x, gb.y, n7.x, n7.y, m.Warn, 3) // marginal, dashed
	c.Line(gb.x, gb.y, n9.x, n9.y, m.Bad, 1)  // no path, dotted
	c.Text(300, 205, "-88 dB", m.Good)
	c.Text(258, 285, "+2.5 dB marginal", m.Warn)
	c.Text(400, 300, "no path -24 dB", m.Bad)

	// antenna pattern lobe around GB7XYZ
	for a := 0; a < 360; a += 4 {
		r := 34.0 + 20*cos(float64(a)*3.14159/180)
		x := gb.x + int(r*cos(float64(a)*3.14159/180))
		y := gb.y + int(r*sin(float64(a)*3.14159/180)*0.55)
		c.Img.Set(x, y, color.RGBA{0x4c, 0x9a, 0xff, 0x88})
	}
	c.Text(gb.x-52, gb.y-46, "antenna pattern", m.Accent)

	for _, n := range []node{gb, n4, n7, n9} {
		c.Rect(n.x-4, n.y-4, 9, 9, n.col, n.col)
		c.Text(n.x+10, n.y+4, n.name, m.Text)
	}
	// emitter
	c.Rect(470, 130, 9, 9, m.Warn, m.Warn)
	c.Text(400, 124, "Cairn Gorm mast  25 W PMR  +6 dB floor", m.Warn)
	// region
	c.Rect(30, 90, 650, 350, color.RGBA{0x2e, 0x3a, 0x46, 0xff}, nil)
	c.Text(36, 84, "region: Scotland  (+30 km RF margin)", m.Muted)

	// ---- inspector -------------------------------------------------------
	ix, iy := c.Panel(716, 34, 556, 430, "NODE INSPECTOR                                     GB7XYZ")
	rows := [][3]string{
		{"Board", "RAK 4631  nRF52840 + SX1262", ""},
		{"Firmware", "repeater 1.17.0 (727fc05)  native", ""},
		{"Radio", "869.525 MHz  BW 250k  SF10  CR5", ""},
		{"Antenna", "6 dBi collinear   az 0 deg  tilt 0", ""},
		{"Position", "57.1204, -3.6712   +/- 60 m", ""},
		{"Height AGL", "12.0 m   ground 842 m", ""},
	}
	for i, r := range rows {
		c.Text(ix, iy+i*18, r[0], m.Muted)
		c.Text(ix+90, iy+i*18, r[1], m.Text)
	}
	// battery
	by := iy + 6*18 + 8
	c.Text(ix, by+10, "Battery", m.Muted)
	c.Rect(ix+90, by, 160, 12, m.Border, nil)
	c.Fill(ix+91, by+1, 113, 10, m.Good)
	c.Text(ix+260, by+10, "71%   solar 12 W", m.Text)

	// console
	cy := by + 30
	c.Text(ix, cy, "CONSOLE", m.Muted)
	c.Rect(ix, cy+8, 532, 190, m.Border, color.RGBA{0x10, 0x14, 0x17, 0xff})
	logLines := []struct {
		s string
		c color.RGBA
	}{
		{"12.401  advert sent (zerohop)", m.Text},
		{"12.440  rx node-04  -92.1 dBm  snr +2.1", m.Text},
		{"12.610  tx ack -> node-04", m.Text},
		{"12.812  rx node-07  -104.3 dBm snr -7.5", m.Warn},
		{"12.813  dedup HIT - not relaying", m.Muted},
		{"> _", m.Accent},
	}
	for i, l := range logLines {
		c.Text(ix+8, cy+26+i*16, l.s, l.c)
	}

	// ---- waterfall -------------------------------------------------------
	wx, wy := c.Panel(8, 470, 700, 210, "WATERFALL   rx: node-07                       [REC armed 12 MB]")
	c.Fill(wx, wy, 676, 150, color.RGBA{0x0e, 0x12, 0x16, 0xff})
	for x := 0; x < 676; x++ {
		for y := 0; y < 150; y++ {
			v := noise(x, y)
			// two overlapping bursts, and a continuous interferer
			if x > 120 && x < 300 && y > 40 && y < 66 {
				v += 150
			}
			if x > 240 && x < 420 && y > 52 && y < 78 {
				v += 190
			}
			if y > 108 && y < 116 {
				v += 90
			}
			if v > 255 {
				v = 255
			}
			c.Img.Set(wx+x, wy+y, color.RGBA{uint8(v / 3), uint8(v / 2), uint8(v), 0xff})
		}
	}
	c.Text(wx+250, wy+96, "overlap 41 ms - collision", m.Warn)
	c.Text(wx+430, wy+118, "mast 868.35 continuous", m.Muted)
	c.Text(wx, wy+166, "click a burst -> symbol view: peak bin 412, 2nd 198, ratio 6.2 dB", m.Muted)
	c.Text(wx, wy+182, "=> node-04 CAPTURED. GB7XYZ lost by 6.2 dB.", m.Good)

	// ---- run control -----------------------------------------------------
	c.Rect(716, 470, 556, 210, m.Border, m.Panel)
	c.Text(724, 490, "RUN CONTROL", m.Text)
	c.Text(724, 516, "[>] [||] [step event] [step symbol]", m.Accent)
	c.Text(724, 540, "speed  x8", m.Text)
	c.Text(830, 540, "1x LOCKED - companion attached", m.Warn)
	c.Text(724, 564, "seed   4417   [randomise] [lock]", m.Text)
	c.Text(724, 588, "nodes  20      emulated 1 (8x slower)", m.Muted)
	c.Rect(724, 604, 540, 40, m.Warn, nil)
	c.Text(732, 620, "Model is kinder than the air: no multipath, body loss,", m.Warn)
	c.Text(732, 636, "or oscillator error. Real links perform worse.", m.Warn)

	// footer
	c.Text(8, 706, "provenance:  fw repeater 1.17.0 727fc05  |  seed 4417  |  terrain z11  |  model hamreach-coverage-1  |  region Scotland", m.Muted)
	c.Text(8, 726, "MeshcoreSim workbench - Debug RF workspace", m.Muted)
	return c
}
