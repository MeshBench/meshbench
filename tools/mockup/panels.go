package main

import (
	"fmt"
	"image/color"

	m "github.com/A13xB0/meshcoresim/internal/mockup"
)

// linkBudget shows both directions side by side — reachability is asymmetric and
// the UI must make that impossible to forget.
func linkBudget() *m.Canvas {
	c := m.New(900, 560)
	c.Fill(0, 0, 900, 26, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(10, 18, "LINK BUDGET    GB7XYZ  <->  node-09", m.Text)

	c.Rect(20, 40, 860, 400, m.Border, m.Panel)
	c.Text(300, 66, "OUT  (XYZ -> 09)", m.Accent)
	c.Text(560, 66, "IN  (09 -> XYZ)", m.Accent)
	c.Line(30, 76, 870, 76, m.Border, 0)

	rows := []struct {
		label, out, in string
		note           string
	}{
		{"TX power", "+22.0 dBm", "+14.0 dBm", ""},
		{"Feedline loss", "-1.2", "-0.3", ""},
		{"TX antenna gain (in direction)", "+5.8", "-2.1", "in a null"},
		{"Path loss (terrain, 2 edges)", "-153.7", "-153.7", ""},
		{"RX antenna gain (in direction)", "-2.1", "+5.8", ""},
		{"RX feedline loss", "-0.3", "-1.2", ""},
	}
	y := 100
	for _, r := range rows {
		c.Text(34, y, r.label, m.Muted)
		c.Text(320, y, r.out, m.Text)
		c.Text(580, y, r.in, m.Text)
		if r.note != "" {
			c.Text(700, y, "<- "+r.note, m.Warn)
		}
		y += 22
	}
	c.Line(30, y, 870, y, m.Border, 0)
	y += 24
	c.Text(34, y, "Received power", m.Text)
	c.Text(320, y, "-129.5 dBm", m.Text)
	c.Text(580, y, "-137.5 dBm", m.Text)
	y += 22
	c.Text(34, y, "Noise floor (BW 250k, NF 6)", m.Muted)
	c.Text(320, y, "-117.0", m.Muted)
	c.Text(580, y, "-117.0", m.Muted)
	y += 22
	c.Text(34, y, "Required SNR (SF10)", m.Muted)
	c.Text(320, y, "-15.0", m.Muted)
	c.Text(580, y, "-15.0", m.Muted)
	y += 28
	c.Text(34, y, "MARGIN", m.Text)
	c.Text(320, y, "+2.5 dB  marginal", m.Warn)
	c.Text(580, y, "-5.5 dB  no path", m.Bad)

	c.Rect(20, 460, 860, 70, m.Warn, nil)
	c.Text(34, 486, "ASYMMETRIC LINK", m.Warn)
	c.Text(34, 508, "node-09 can hear GB7XYZ. GB7XYZ cannot hear node-09. A one-way link is not a link.", m.Text)
	return c
}

// receptionLedger answers "what did each repeater actually receive" — including
// frames the firmware never saw.
func receptionLedger() *m.Canvas {
	c := m.New(1000, 470)
	c.Fill(0, 0, 1000, 26, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(10, 18, "RECEPTION LEDGER    packet #4471  advert from GB7XYZ", m.Text)

	c.Rect(20, 40, 960, 300, m.Border, m.Panel)
	hdr := []string{"node", "offered", "RSSI", "SNR", "demod", "CRC", "firmware", "action"}
	xs := []int{34, 150, 250, 340, 430, 520, 610, 760}
	for i, h := range hdr {
		c.Text(xs[i], 66, h, m.Muted)
	}
	c.Line(30, 76, 970, 76, m.Border, 0)

	rows := []struct {
		cells [8]string
		col   color.RGBA
	}{
		{[8]string{"node-04", "yes", "-88.1", "+4.2", "ok", "ok", "accepted", "RELAYED"}, m.Good},
		{[8]string{"node-07", "yes", "-104.3", "-7.5", "ok", "ok", "dedup HIT", "dropped"}, m.Warn},
		{[8]string{"node-09", "yes", "-131.2", "-19.1", "ok", "FAIL", "-", "never saw"}, m.Bad},
		{[8]string{"node-12", "yes", "-142.0", "-28.0", "no", "-", "-", "never saw"}, m.Bad},
		{[8]string{"node-15", "no", "-", "-", "-", "-", "-", "out of range"}, m.Muted},
	}
	y := 100
	for _, r := range rows {
		for i, cell := range r.cells {
			col := m.Text
			if i >= 4 {
				col = r.col
			}
			c.Text(xs[i], y, cell, col)
		}
		y += 24
	}
	c.Line(30, y, 970, y, m.Border, 0)
	c.Text(34, y+22, "reached 4 of 12   demodulated 3   accepted 2   relayed 1", m.Text)
	c.Text(34, y+44, "[why?] on any row -> path profile and link budget for that pair", m.Accent)

	c.Rect(20, 356, 960, 96, m.Accent, nil)
	c.Text(34, 380, "The five outcomes a packet-level simulator collapses into 'did not arrive':", m.Text)
	c.Text(34, 402, "out of range  |  too weak to demodulate  |  demodulated but CRC failed  |", m.Muted)
	c.Text(34, 422, "received and deliberately dropped (dedup)  |  received and relayed", m.Muted)
	c.Text(34, 444, "Rows come from the RF layer, so they exist even when the firmware never knew.", m.Good)
	return c
}

// energy shows the battery/solar answer that actually changes a purchase.
func energy() *m.Canvas {
	c := m.New(1000, 500)
	c.Fill(0, 0, 1000, 26, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(10, 18, "ENERGY    GB7XYZ   Heltec Mesh Solar   12 W panel, 6 Ah", m.Text)

	left, right, base, top := 80, 950, 300, 60
	c.Rect(20, 40, 960, 300, m.Border, m.Panel)
	c.Line(left, top, left, base, m.Border, 0)
	c.Line(left, base, right, base, m.Border, 0)
	for i, l := range []string{"100", "75", "50", "25", "0"} {
		y := top + i*(base-top)/4
		c.Text(left-36, y+4, l+"%", m.Muted)
		c.Line(left, y, right, y, color.RGBA{0x22, 0x28, 0x2e, 0xff}, 4)
	}
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	for i, mn := range months {
		x := left + i*(right-left)/12
		c.Text(x+8, base+20, mn, m.Muted)
	}

	// state of charge: healthy in summer, flat in December
	prev := -1
	for i := 0; i <= right-left; i++ {
		t := float64(i) / float64(right-left)
		soc := 0.55 + 0.45*sin((t-0.18)*2*3.14159)
		if soc > 1 {
			soc = 1
		}
		if soc < 0 {
			soc = 0
		}
		y := base - int(soc*float64(base-top))
		col := m.Good
		if soc < 0.2 {
			col = m.Bad
		} else if soc < 0.4 {
			col = m.Warn
		}
		if prev >= 0 {
			c.Line(left+i-1, prev, left+i, y, col, 0)
		}
		prev = y
	}
	// flat window
	fx1 := left + int(0.925*float64(right-left))
	fx2 := left + int(0.97*float64(right-left))
	c.Rect(fx1, top, fx2-fx1, base-top, m.Bad, nil)
	c.Text(fx1-150, top+20, "FLAT 03-19 Dec (17 days)", m.Bad)

	c.Rect(20, 356, 960, 130, m.Border, color.RGBA{0x18, 0x1d, 0x22, 0xff})
	c.Text(34, 382, "CAUSE", m.Text)
	c.Text(34, 404, "41 h/month of terrain-shaded morning sun (ridge to the SE) + 62 mAh/day traffic", m.Muted)
	c.Text(34, 432, "WHAT ACTUALLY FIXES IT", m.Text)
	c.Text(34, 454, "panel 12 W -> 30 W:  survives, min SoC 22%", m.Good)
	c.Text(500, 454, "battery 6 Ah -> 12 Ah:  still flat", m.Bad)
	c.Text(34, 474, "People routinely buy the bigger battery. In a UK December it does not help.", m.Warn)
	return c
}

func label(i int, s string) string { return fmt.Sprintf("%d %s", i, s) }
