package main

import m "github.com/MeshBench/meshbench/internal/mockup"

func linkBudget() *m.Canvas {
	c := m.New(1000, 660)
	c.Fill(0, 0, 1000, 44, m.BgSurface)
	c.Line(0, 44, 1000, 44, m.Border, 1, 0)
	c.Text(20, 27, "LINK BUDGET", m.TextHi, m.SansBold, 12)
	c.Text(150, 27, "GB7XYZ ⇄ node-09", m.TextMid, m.Sans, 11.5)

	x, y := card(c, 16, 60, 968, 470, "EVERY TERM, BOTH DIRECTIONS", "")
	c.Text(x+430, y-16, "OUT   XYZ → 09", m.Accent, m.SansBold, 10.5)
	c.Text(x+660, y-16, "IN   09 → XYZ", m.Accent, m.SansBold, 10.5)
	rows := []struct{ label, out, in, note string }{
		{"TX power", "+22.0 dBm", "+14.0 dBm", ""},
		{"Feedline loss", "−1.2", "−0.3", ""},
		{"TX antenna gain (in direction)", "+5.8", "−2.1", "in a null"},
		{"Path loss  terrain, 2 edges", "−153.7", "−153.7", ""},
		{"RX antenna gain (in direction)", "−2.1", "+5.8", ""},
		{"RX feedline loss", "−0.3", "−1.2", ""},
	}
	for i, r := range rows {
		yy := y + 16 + i*30
		if i%2 == 0 {
			c.RoundRect(x-8, yy-16, 952, 28, 4, m.Alpha(m.BgRaised, 0x80), nil)
		}
		c.Text(x, yy, r.label, m.TextMid, m.Sans, 10.5)
		c.TextRight(x+520, yy, r.out, m.TextHi, m.Mono, 10.5)
		c.TextRight(x+750, yy, r.in, m.TextHi, m.Mono, 10.5)
		if r.note != "" {
			c.Text(x+770, yy, "← "+r.note, m.Warn, m.Sans, 10)
		}
	}
	yy := y + 16 + len(rows)*30 + 10
	c.Line(x-8, yy-14, x+944, yy-14, m.Border, 1, 0)
	for i, r := range [][3]string{
		{"Received power", "−129.5 dBm", "−137.5 dBm"},
		{"Noise floor  BW 250k, NF 6", "−117.0", "−117.0"},
		{"Required SNR  SF10", "−15.0", "−15.0"},
	} {
		c.Text(x, yy+i*26, r[0], m.TextMid, m.Sans, 10.5)
		c.TextRight(x+520, yy+i*26, r[1], m.TextHi, m.Mono, 10.5)
		c.TextRight(x+750, yy+i*26, r[2], m.TextHi, m.Mono, 10.5)
	}
	my := yy + 3*26 + 14
	c.Text(x, my+4, "MARGIN", m.TextHi, m.SansBold, 12)
	pill(c, x+430, my-8, "+2.5 dB  marginal", m.Warn)
	pill(c, x+660, my-8, "−5.5 dB  no path", m.Bad)

	c.RoundRect(16, 548, 968, 96, 10, m.Alpha(m.Warn, 0x12), m.Alpha(m.Warn, 0x60))
	c.Text(36, 578, "ASYMMETRIC LINK", m.Warn, m.SansBold, 11.5)
	c.Text(36, 604, "node-09 can hear GB7XYZ. GB7XYZ cannot hear node-09.", m.TextHi, m.Sans, 11)
	c.Text(36, 626, "A result that does not say which direction works is wrong even when the arithmetic is right.", m.TextMid, m.Sans, 10.5)
	return c
}

func receptionLedger() *m.Canvas {
	c := m.New(1160, 620)
	c.Fill(0, 0, 1160, 44, m.BgSurface)
	c.Line(0, 44, 1160, 44, m.Border, 1, 0)
	c.Text(20, 27, "RECEPTION LEDGER", m.TextHi, m.SansBold, 12)
	c.Text(200, 27, "packet #4471 · advert from GB7XYZ", m.TextMid, m.Sans, 11)

	x, y := card(c, 16, 60, 1128, 350, "WHAT EVERY NODE ACTUALLY RECEIVED", "from the RF layer — independent of firmware")
	cols := []int{0, 130, 250, 350, 450, 550, 660, 810}
	hdr := []string{"node", "offered", "RSSI", "SNR", "demod", "CRC", "firmware", "action"}
	for i, h := range hdr {
		c.Text(x+cols[i], y-14, h, m.TextLo, m.SansBold, 9.5)
	}
	c.Line(x-8, y-4, x+1112, y-4, m.Border, 1, 0)
	rows := []struct {
		cells [8]string
		col   m.NRGBA
	}{
		{[8]string{"node-04", "yes", "−88.1", "+4.2", "ok", "ok", "accepted", "RELAYED"}, m.Good},
		{[8]string{"node-07", "yes", "−104.3", "−7.5", "ok", "ok", "dedup HIT", "dropped"}, m.Warn},
		{[8]string{"node-09", "yes", "−131.2", "−19.1", "ok", "FAIL", "—", "never saw"}, m.Bad},
		{[8]string{"node-12", "yes", "−142.0", "−28.0", "no", "—", "—", "never saw"}, m.Bad},
		{[8]string{"node-15", "no", "—", "—", "—", "—", "—", "out of range"}, m.TextLo},
	}
	for i, r := range rows {
		yy := y + 20 + i*34
		if i%2 == 0 {
			c.RoundRect(x-8, yy-18, 1112, 32, 4, m.Alpha(m.BgRaised, 0x80), nil)
		}
		c.Text(x, yy, r.cells[0], m.TextHi, m.MonoBold, 10.5)
		for j := 1; j < 7; j++ {
			c.Text(x+cols[j], yy, r.cells[j], m.TextMid, m.Mono, 10)
		}
		pill(c, x+cols[7], yy-13, r.cells[7], r.col)
	}
	c.Text(x, y+20+len(rows)*34+12, "reached 4 of 12   ·   demodulated 3   ·   accepted 2   ·   relayed 1", m.TextHi, m.Sans, 10.5)
	c.Text(x+700, y+20+len(rows)*34+12, "[ why? ] on any row → path profile + link budget", m.Accent, m.Sans, 10.5)

	c.RoundRect(16, 424, 1128, 178, 10, m.Alpha(m.Accent, 0x10), m.Alpha(m.Accent, 0x50))
	c.Text(36, 456, "FIVE OUTCOMES A PACKET SIMULATOR COLLAPSES INTO ONE", m.Accent, m.SansBold, 11)
	outs := []string{
		"out of range — nothing ever arrived",
		"arrived too weak to demodulate",
		"demodulated, but CRC failed",
		"received, and deliberately dropped (dedup)",
		"received, and relayed",
	}
	for i, o := range outs {
		c.Dot(46, 484+i*22, 3, m.Accent)
		c.Text(60, 488+i*22, o, m.TextMid, m.Sans, 10.5)
	}
	c.Text(640, 488, "Rows come from the RF layer, so they exist even", m.TextHi, m.Sans, 10.5)
	c.Text(640, 510, "when the firmware never knew the frame arrived.", m.TextHi, m.Sans, 10.5)
	c.Text(640, 540, "No firmware instrumentation can provide this —", m.Good, m.Sans, 10.5)
	c.Text(640, 562, "it is a stronger answer than the node itself has.", m.Good, m.Sans, 10.5)
	return c
}
