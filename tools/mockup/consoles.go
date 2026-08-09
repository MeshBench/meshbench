package main

import (
	"image/color"

	m "github.com/A13xB0/meshcoresim/internal/mockup"
)

// consoles shows multi-console with synchronised timestamps and mass commands
// over the virtual UART — never over the air.
func consoles() *m.Canvas {
	c := m.New(1100, 620)
	c.Fill(0, 0, 1100, 26, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(10, 18, "CONSOLES      [+ node]  [tile | tabs]  [sync scroll ON]", m.Text)

	type pane struct {
		x, y  int
		title string
		warn  string
		lines [][2]string
	}
	panes := []pane{
		{20, 40, "GB7XYZ   repeater 1.17.0", "", [][2]string{
			{"12.401", "advert sent (zerohop)"},
			{"12.440", "rx node-04 -92.1 snr +2.1"},
			{"12.610", "tx ack -> node-04"},
			{"> ", "_"},
		}},
		{560, 40, "node-04   repeater 1.16.0", "", [][2]string{
			{"12.418", "rx GB7XYZ -88.1 snr +4.2"},
			{"12.440", "retransmit (flood)"},
			{"12.611", "rx ack"},
			{"> ", "_"},
		}},
		{20, 250, "node-07   DEBUG BUILD", "instrumented - tracing can alter CSMA timing", [][2]string{
			{"12.418", "rx GB7XYZ -104.3 snr -7.5"},
			{"12.418", "DEBUG dedup: HIT, drop"},
			{"12.419", "DEBUG not relaying (seen 0.9s ago)"},
			{"> ", "_"},
		}},
		{560, 250, "node-09   repeater 1.17.0", "firmware logged nothing", [][2]string{
			{"", "(no firmware output)"},
			{"", ""},
			{"RF", "1 frame offered -131.2 dBm"},
			{"RF", "CRC FAIL - firmware never saw it"},
		}},
	}
	for _, p := range panes {
		c.Rect(p.x, p.y, 520, 190, m.Border, m.Panel)
		c.Fill(p.x+1, p.y+1, 518, 18, color.RGBA{0x24, 0x2b, 0x33, 0xff})
		col := m.Text
		if p.warn != "" {
			col = m.Warn
		}
		c.Text(p.x+8, p.y+14, p.title, col)
		for i, l := range p.lines {
			lc := m.Text
			if l[0] == "RF" {
				lc = m.Accent
			}
			if l[0] == "" && l[1] == "(no firmware output)" {
				lc = m.Muted
			}
			c.Text(p.x+10, p.y+40+i*20, l[0], m.Muted)
			c.Text(p.x+70, p.y+40+i*20, l[1], lc)
		}
		if p.warn != "" {
			c.Text(p.x+10, p.y+178, p.warn, m.Warn)
		}
	}
	c.Text(20, 468, "Synchronised timestamps across panes: reading four consoles at one instant", m.Muted)
	c.Text(20, 486, "is the value. Four terminal windows cannot do it.", m.Muted)
	c.Text(560, 468, "node-09 is the key idea: the firmware logged nothing, but the RF", m.Accent)
	c.Text(560, 486, "layer still saw the frame. No instrumentation could show that.", m.Accent)

	c.Rect(20, 508, 1060, 96, m.Border, color.RGBA{0x18, 0x1d, 0x22, 0xff})
	c.Text(34, 532, "BROADCAST   > set radio 869.525,250,10,5", m.Text)
	c.Text(700, 532, "to: 12 nodes selected", m.Muted)
	c.Text(34, 556, "transport: virtual UART - no RF cost, no airtime, no altered collisions", m.Good)
	c.Text(34, 578, "WARNING: changes radio config on 12 nodes and invalidates this run", m.Warn)
	c.Text(760, 578, "[Cancel]   [Send to 12]", m.Accent)
	return c
}

// interference shows external emitters and the filter question.
func interference() *m.Canvas {
	c := m.New(1000, 520)
	c.Fill(0, 0, 1000, 26, color.RGBA{0x24, 0x2b, 0x33, 0xff})
	c.Text(10, 18, "INTERFERENCE     [Import Ofcom]  [+ Manual]", m.Text)

	c.Rect(20, 40, 960, 170, m.Border, m.Panel)
	hdr := []string{"emitter", "type", "freq", "ERP", "height", "duty", "at node-07"}
	xs := []int{34, 220, 350, 470, 560, 660, 760}
	for i, h := range hdr {
		c.Text(xs[i], 66, h, m.Muted)
	}
	c.Line(30, 76, 970, 76, m.Border, 0)
	rows := [][7]string{
		{"Cairn Gorm", "PMR mast", "868.35 MHz", "25 W", "45 m", "30%", "+4.1 dB"},
		{"Aviemore relay", "Broadcast", "out of band", "2 kW", "80 m", "cont.", "+0.3 dB"},
		{"(manual) paging", "Paging", "869.50 MHz", "25 W", "20 m", "bursty", "+1.6 dB"},
	}
	for i, r := range rows {
		for j, cell := range r {
			col := m.Text
			if j == 6 {
				col = m.Warn
			}
			c.Text(xs[j], 100+i*24, cell, col)
		}
	}
	c.Text(34, 182, "8 emitters loaded   214 culled below -140 dBm contribution", m.Muted)

	c.Rect(20, 226, 470, 130, m.Warn, nil)
	c.Text(34, 250, "EFFECT AT node-07", m.Text)
	c.Text(34, 274, "thermal floor          -117.0 dBm", m.Muted)
	c.Text(34, 296, "with interference      -111.0 dBm", m.Warn)
	c.Text(34, 320, "sensitivity lost          6.0 dB", m.Bad)
	c.Text(34, 344, "= roughly half your range", m.Bad)

	c.Rect(510, 226, 470, 130, m.Accent, nil)
	c.Text(524, 250, "STRETCH: RX FILTER   [cavity 868 +/- 2 MHz]", m.Text)
	c.Text(524, 274, "insertion loss          -1.1 dB", m.Muted)
	c.Text(524, 296, "rejection @ 869.5      -38.0 dB", m.Good)
	c.Text(524, 320, "resulting floor        -116.2 dBm", m.Good)
	c.Text(524, 344, "VERDICT: fixable, buy the cavity", m.Good)

	c.Rect(20, 372, 960, 130, m.Border, color.RGBA{0x18, 0x1d, 0x22, 0xff})
	c.Text(34, 396, "THE ANSWER THAT SAVES MONEY", m.Text)
	c.Text(34, 420, "A filter only helps out-of-band interference. If the interferer is inside", m.Muted)
	c.Text(34, 440, "your own passband, no filter will help - and the tool must say so plainly", m.Muted)
	c.Text(34, 460, "rather than showing a marginal improvement that tempts a purchase.", m.Muted)
	c.Text(34, 486, "Culled emitters are counted in the UI. A silent cull is a silent lie.", m.Warn)
	return c
}
