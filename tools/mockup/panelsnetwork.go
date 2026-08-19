package main

import m "github.com/MeshBench/meshbench/tools/internal/mockup"

func energy() *m.Canvas {
	c := m.New(1160, 640)
	c.Fill(0, 0, 1160, 44, m.BgSurface)
	c.Line(0, 44, 1160, 44, m.Border, 1, 0)
	c.Text(20, 27, "ENERGY", m.TextHi, m.SansBold, 12)
	c.Text(110, 27, "GB7XYZ · Heltec Mesh Solar · 12 W panel · 6 Ah", m.TextMid, m.Sans, 11)
	pill(c, 620, 13, "FLAT 17 DAYS", m.Bad)

	x, y := card(c, 16, 60, 1128, 400, "STATE OF CHARGE — 12 MONTHS", "traffic measured from actual firmware behaviour")
	left, right, top, base := x+50, x+1080, y, y+300
	for i, l := range []string{"100%", "75%", "50%", "25%", "0%"} {
		yy := top + i*(base-top)/4
		c.TextRight(left-12, yy+4, l, m.TextLo, m.Mono, 9.5)
		c.Line(left, yy, right, yy, m.Alpha(m.Border, 0x90), 1, 6)
	}
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	for i, mo := range months {
		c.Text(left+i*(right-left)/12+10, base+24, mo, m.TextLo, m.Sans, 9.5)
	}
	// winter danger band
	fx1 := left + int(0.925*float64(right-left))
	fx2 := left + int(0.972*float64(right-left))
	c.RoundRect(fx1, top, fx2-fx1, base-top, 4, m.Alpha(m.Bad, 0x1e), m.Alpha(m.Bad, 0x70))

	pts := [][2]int{}
	for i := 0; i <= right-left; i += 3 {
		t := float64(i) / float64(right-left)
		soc := 0.56 + 0.46*m.Sin((t-0.20)*2*3.14159)
		if soc > 1 {
			soc = 1
		}
		if soc < 0 {
			soc = 0
		}
		pts = append(pts, [2]int{left + i, base - int(soc*float64(base-top))})
	}
	c.Polyline(pts, m.Good, 2.4, base, m.Alpha(m.Good, 0x22))
	c.Text(fx1-140, top+26, "FLAT 03–19 Dec", m.Bad, m.SansBold, 10.5)
	c.Text(fx1-140, top+46, "min SoC 0%", m.Bad, m.Mono, 10)

	bx, by := card(c, 16, 476, 556, 148, "CAUSE", "")
	c.Text(bx, by, "41 h/month of terrain-shaded morning sun", m.TextHi, m.Sans, 10.5)
	c.Text(bx, by+22, "(ridge to the south-east blocks until 10:40)", m.TextMid, m.Sans, 10)
	c.Text(bx, by+48, "62 mAh/day traffic — measured, not assumed", m.TextHi, m.Sans, 10.5)

	fx, fy := card(c, 588, 476, 556, 148, "WHAT ACTUALLY FIXES IT", "")
	c.Dot(fx+6, fy-4, 4, m.Good)
	c.Text(fx+20, fy, "panel 12 W → 30 W", m.TextHi, m.Sans, 10.5)
	c.TextRight(fx+520, fy, "survives · min 22%", m.Good, m.Mono, 10.5)
	c.Dot(fx+6, fy+26, 4, m.Bad)
	c.Text(fx+20, fy+30, "battery 6 Ah → 12 Ah", m.TextHi, m.Sans, 10.5)
	c.TextRight(fx+520, fy+30, "still flat", m.Bad, m.Mono, 10.5)
	c.Text(fx, fy+64, "People routinely buy the bigger battery.", m.Warn, m.Sans, 10.5)
	c.Text(fx, fy+84, "In a UK December it does not help.", m.Warn, m.Sans, 10.5)
	return c
}

func consoles() *m.Canvas {
	c := m.New(1240, 720)
	c.Fill(0, 0, 1240, 44, m.BgSurface)
	c.Line(0, 44, 1240, 44, m.Border, 1, 0)
	c.Text(20, 27, "CONSOLES", m.TextHi, m.SansBold, 12)
	c.Text(130, 27, "4 nodes · synchronised timestamps", m.TextMid, m.Sans, 11)
	pill(c, 400, 13, "SYNC SCROLL", m.Accent)

	type pane struct {
		x, y  int
		title string
		badge string
		bcol  m.NRGBA
		lines [][3]string
	}
	panes := []pane{
		{16, 60, "GB7XYZ", "repeater 1.17.0", m.Accent, [][3]string{
			{"12.401", "advert sent (zerohop)", "mid"},
			{"12.440", "rx node-04 −92.1 snr +2.1", "hi"},
			{"12.610", "tx ack → node-04", "mid"},
		}},
		{628, 60, "node-04", "repeater 1.16.0", m.Accent, [][3]string{
			{"12.418", "rx GB7XYZ −88.1 snr +4.2", "hi"},
			{"12.440", "retransmit (flood)", "mid"},
			{"12.611", "rx ack", "mid"},
		}},
		{16, 306, "node-07", "DEBUG BUILD", m.Warn, [][3]string{
			{"12.418", "rx GB7XYZ −104.3 snr −7.5", "warn"},
			{"12.418", "DEBUG dedup: HIT, drop", "lo"},
			{"12.419", "DEBUG not relaying (seen 0.9s)", "lo"},
		}},
		{628, 306, "node-09", "repeater 1.17.0", m.Accent, [][3]string{
			{"", "(firmware logged nothing)", "lo"},
			{"RF", "1 frame offered −131.2 dBm", "accent"},
			{"RF", "CRC FAIL — firmware never saw it", "accent"},
		}},
	}
	for _, p := range panes {
		px, py := card(c, p.x, p.y, 596, 226, p.title, "")
		pill(c, px+90, p.y+12, p.badge, p.bcol)
		c.RoundRect(px-8, py-14, 580, 150, 8, m.BgInset, m.Border)
		for i, l := range p.lines {
			col := m.TextMid
			switch l[2] {
			case "hi":
				col = m.TextHi
			case "lo":
				col = m.TextLo
			case "warn":
				col = m.Warn
			case "accent":
				col = m.Accent
			}
			c.Text(px+6, py+10+i*24, l[0], m.TextLo, m.Mono, 9.5)
			c.Text(px+56, py+10+i*24, l[1], col, m.Mono, 9.5)
		}
		c.Text(px+6, py+10+3*24+6, "›", m.Accent, m.MonoBold, 11)
	}
	c.Text(640, 520, "node-09 is the key idea: the firmware logged nothing, but the RF layer", m.Accent, m.Sans, 10.5)
	c.Text(640, 542, "still saw the frame. No instrumentation could show that.", m.Accent, m.Sans, 10.5)
	c.Text(30, 520, "Synchronised timestamps across panes — reading four consoles at", m.TextMid, m.Sans, 10.5)
	c.Text(30, 542, "one instant is the value four terminal windows cannot give you.", m.TextMid, m.Sans, 10.5)

	bx, by := card(c, 16, 568, 1208, 136, "BROADCAST", "virtual UART — no RF cost")
	c.RoundRect(bx-8, by-14, 800, 32, 6, m.BgInset, m.BorderLit)
	c.Text(bx+6, by+6, "› set radio 869.525,250,10,5", m.TextHi, m.Mono, 11)
	c.TextRight(bx+1180, by+6, "12 nodes selected", m.TextMid, m.Sans, 10.5)
	c.Dot(bx+6, by+42, 4, m.Good)
	c.Text(bx+20, by+46, "Sent over virtual UART: no airtime, no backoff, no altered collision statistics", m.Good, m.Sans, 10.5)
	c.Dot(bx+6, by+70, 4, m.Warn)
	c.Text(bx+20, by+74, "Changes radio config on 12 nodes and invalidates this run", m.Warn, m.Sans, 10.5)
	c.RoundRect(bx+900, by+58, 120, 28, 6, m.BgRaised, m.BorderLit)
	c.Text(bx+928, by+77, "Cancel", m.TextHi, m.Sans, 10.5)
	c.RoundRect(bx+1030, by+58, 150, 28, 6, m.Alpha(m.Accent, 0x30), m.Accent)
	c.Text(bx+1050, by+77, "Send to 12", m.TextHi, m.SansBold, 10.5)
	return c
}
