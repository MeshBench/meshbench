// Command render produces figures from the *real* engine — not mockups.
//
// Everything drawn here comes from internal/terrain, internal/dsp, internal/rf
// and internal/antenna doing actual work. If a curve looks wrong, the code is
// wrong; nothing is hand-drawn.
package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/channel"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	m "github.com/MeshBench/meshbench/tools/internal/mockup"
)

func sq(x float64) float64 { return x * x }

func main() {
	out := "docs/output"
	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}
	for _, f := range []struct {
		name string
		draw func() *m.Canvas
	}{
		{"real-01-path-profile", realPathProfile},
		{"real-02-waterfall", realWaterfall},
		{"real-03-antenna", realAntenna},
		{"real-04-sdr-observer", realSDRObserver},
	} {
		p := filepath.Join(out, f.name+".png")
		fh, err := os.Create(p)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(fh, f.draw().Img); err != nil {
			panic(err)
		}
		if err := fh.Close(); err != nil {
			panic(err)
		}
		fmt.Println("wrote", p)
	}

	wav := filepath.Join(out, "real-04-sdr-observer.wav")
	if err := writeSDRAudio(wav); err != nil {
		panic(err)
	}
	fmt.Println("wrote", wav)
}

// realPathProfile runs the actual Deygout engine over a synthetic ridge and
// plots what it returns.
func realPathProfile() *m.Canvas {
	c := m.New(1200, 620)
	c.Fill(0, 0, 1200, 40, m.BgSurface)
	c.Text(20, 26, "PATH PROFILE — computed by internal/terrain", m.TextHi, m.SansBold, 12)

	// Build a profile: 12 km, two ridges.
	const n = 240
	prof := make([]terrain.Point, n+1)
	for i := 0; i <= n; i++ {
		d := float64(i) * 50 // 50 m steps -> 12 km
		t := float64(i) / n
		h := 120.0 +
			430*math.Exp(-sq((t-0.34)/0.07)) +
			260*math.Exp(-sq((t-0.66)/0.06)) +
			25*math.Sin(t*20)
		prof[i] = terrain.Point{DistM: d, HeightM: h}
	}
	const freq = 869.525
	loss := terrain.MultiEdgeLossDB(prof, 12, 2, freq)
	fspl := terrain.FSPLdB(12.0, freq)

	x0, x1 := 90, 1160
	base, top := 470, 70
	maxH := 0.0
	for _, p := range prof {
		maxH = math.Max(maxH, p.HeightM)
	}
	scaleY := float64(base-top) / (maxH * 1.15)

	for i := 0; i <= 4; i++ {
		y := base - int(float64(i)*maxH*1.15/4*scaleY)
		c.TextRight(x0-10, y+4, fmt.Sprintf("%.0f m", float64(i)*maxH*1.15/4), m.TextLo, m.Mono, 9.5)
		c.Line(x0, y, x1, y, m.Alpha(m.Border, 0x90), 1, 6)
	}

	pts := make([][2]int, len(prof))
	for i, p := range prof {
		pts[i] = [2]int{x0 + int(float64(i)/float64(n)*float64(x1-x0)), base - int(p.HeightM*scaleY)}
	}
	c.Polyline(pts, m.Terrain, 2, base, m.Alpha(m.Terrain, 0x66))

	// Antennas and the straight line between them.
	txY := base - int((prof[0].HeightM+12)*scaleY)
	rxY := base - int((prof[n].HeightM+2)*scaleY)
	c.Line(x0, base-int(prof[0].HeightM*scaleY), x0, txY, m.Accent, 2, 0)
	c.Line(x1, base-int(prof[n].HeightM*scaleY), x1, rxY, m.Accent, 2, 0)
	c.Line(x0, txY, x1, rxY, m.Accent, 1.6, 7)
	c.Dot(x0, txY, 4, m.Accent)
	c.Dot(x1, rxY, 4, m.Accent)
	c.Text(x0+8, txY-10, "TX 12 m AGL", m.Accent, m.Sans, 9.5)
	c.TextRight(x1-8, rxY-10, "RX 2 m AGL", m.Accent, m.Sans, 9.5)

	// Earth bulge, from the real function, exaggerated to be visible.
	bul := make([][2]int, 0, n+1)
	for i := 0; i <= n; i++ {
		d1 := prof[i].DistM
		d2 := prof[n].DistM - d1
		b := terrain.EarthBulgeM(d1, d2)
		yy := float64(txY) + float64(rxY-txY)*float64(i)/float64(n) + b*scaleY*20
		bul = append(bul, [2]int{x0 + int(float64(i)/float64(n)*float64(x1-x0)), int(yy)})
	}
	c.Polyline(bul, m.Violet, 1.4, 0, nil)
	c.Text(x0+430, txY-34, "k=4/3 earth bulge (x20 for visibility)", m.Violet, m.Sans, 9.5)

	// Results panel — real numbers.
	c.RoundRect(20, 500, 1160, 100, 10, m.BgSurface, m.Border)
	items := [][2]string{
		{"free-space loss (12.0 km @ 869.5 MHz)", fmt.Sprintf("%.1f dB", fspl)},
		{"multi-edge diffraction (Deygout)", fmt.Sprintf("%.1f dB", loss)},
		{"total path loss", fmt.Sprintf("%.1f dB", fspl+loss)},
	}
	for i, it := range items {
		c.Text(40, 530+i*22, it[0], m.TextMid, m.Sans, 10.5)
		c.TextRight(520, 530+i*22, it[1], m.TextHi, m.Mono, 10.5)
	}
	c.Text(580, 530, "noise floor (BW 250k, NF 6)", m.TextMid, m.Sans, 10.5)
	c.TextRight(900, 530, fmt.Sprintf("%.1f dBm", dsp.NoiseFloorDBm(250000, 6)), m.TextHi, m.Mono, 10.5)
	rx := 22.0 - 1.2 + 6.0 - (fspl + loss) - 2.0
	c.Text(580, 552, "received power (22 dBm TX, 6 dBi)", m.TextMid, m.Sans, 10.5)
	c.TextRight(900, 552, fmt.Sprintf("%.1f dBm", rx), m.TextHi, m.Mono, 10.5)
	margin := rx - dsp.NoiseFloorDBm(250000, 6) - 15
	col, verdict := m.Bad, "NO PATH"
	if margin > 10 {
		col, verdict = m.Good, "WORKS"
	} else if margin > 0 {
		col, verdict = m.Warn, "MARGINAL"
	}
	c.Text(580, 576, "margin over SF10 threshold", m.TextMid, m.Sans, 10.5)
	c.TextRight(900, 576, fmt.Sprintf("%+.1f dB", margin), col, m.MonoBold, 11)
	pill(c, 950, 566, verdict, col)
	return c
}

// realWaterfall modulates two real LoRa frames, sums them through the real
// channel with real noise, and plots the magnitude.
func realWaterfall() *m.Canvas {
	c := m.New(1200, 560)
	c.Fill(0, 0, 1200, 40, m.BgSurface)
	c.Text(20, 26, "COLLISION — real modulation, real channel, real noise", m.TextHi, m.SansBold, 12)

	const sf = 8
	n := dsp.SamplesPerSymbol(sf)
	mod := dsp.Modulator{SF: sf}
	window := n * 6

	strong := mod.Modulate([]int{10, 20, 30, 40})
	weak := mod.Modulate([]int{200, 210, 220})

	obs := channel.Observe([]channel.Transmission{
		{Node: "GB7XYZ", Samples: strong, GainDB: 0, StartSample: 0, DelaySamples: 0.4},
		{Node: "node-04", Samples: weak, GainDB: -6, StartSample: n * 2, DelaySamples: 1.7},
	}, channel.Receiver{NoisePowerLinear: 0.02, Seed: 4417}, window)

	x0, y0, w, h := 40, 70, 1120, 300
	c.RoundRect(x0-8, y0-8, w+16, h+16, 8, m.BgInset, m.Border)
	for px := 0; px < w; px++ {
		i := px * window / w
		mag := math.Hypot(real(obs[i]), imag(obs[i]))
		col := m.Alpha(m.Accent, uint8(math.Min(255, mag*110)))
		bar := int(math.Min(float64(h), mag*float64(h)/2.2))
		c.Fill(x0+px, y0+h-bar, 1, bar, col)
	}
	c.Text(x0, y0+h+26, "amplitude of the summed waveform at the receiver", m.TextLo, m.Sans, 10)

	// Demodulate each symbol slot and report what actually won.
	dm := dsp.Demodulator{SF: sf}
	c.RoundRect(20, 420, 1160, 126, 10, m.BgSurface, m.Border)
	c.Text(40, 450, "PER-SYMBOL DECISION", m.TextHi, m.SansBold, 11)
	hdr := []string{"slot", "recovered", "confidence", "interpretation"}
	xs := []int{40, 140, 280, 440}
	for i, s := range hdr {
		c.Text(xs[i], 474, s, m.TextLo, m.SansBold, 9.5)
	}
	sent := []int{10, 20, 30, 40}
	for slot := 0; slot < 4; slot++ {
		sym, conf := dm.DemodulateSymbol(obs[slot*n : (slot+1)*n])
		note, col := "clean", m.Good
		if slot >= 2 {
			note, col = "overlapped by node-04", m.Warn
		}
		if sym != sent[slot] {
			note, col = "LOST to the collision", m.Bad
		}
		y := 496 + slot*20
		c.Text(xs[0], y, fmt.Sprintf("%d", slot), m.TextMid, m.Mono, 10)
		c.Text(xs[1], y, fmt.Sprintf("%d (sent %d)", sym, sent[slot]), m.TextHi, m.Mono, 10)
		c.Text(xs[2], y, fmt.Sprintf("%.2f", conf), m.TextMid, m.Mono, 10)
		c.Text(xs[3], y, note, col, m.Sans, 10)
	}
	return c
}

// realAntenna plots the actual patterns from internal/antenna.
func realAntenna() *m.Canvas {
	c := m.New(1200, 520)
	c.Fill(0, 0, 1200, 40, m.BgSurface)
	c.Text(20, 26, "ANTENNA PATTERNS — computed by internal/antenna", m.TextHi, m.SansBold, 12)

	pats := []struct {
		p    antenna.Pattern
		name string
		col  m.NRGBA
	}{
		{antenna.Dipole{}, "half-wave dipole", m.Good},
		{antenna.Collinear{GainDBiPeak: 6}, "6 dBi collinear", m.Accent},
		{antenna.Collinear{GainDBiPeak: 9}, "9 dBi collinear", m.Violet},
	}
	x0, y0, w, h := 90, 90, 1060, 320
	c.RoundRect(x0-8, y0-8, w+16, h+16, 8, m.BgInset, m.Border)
	// elevation −40..+40, gain −20..+12 dBi
	for g := -20; g <= 10; g += 10 {
		y := y0 + int(float64(10-g)/32*float64(h))
		c.TextRight(x0-12, y+4, fmt.Sprintf("%d dBi", g), m.TextLo, m.Mono, 9)
		c.Line(x0, y, x0+w, y, m.Alpha(m.Border, 0x90), 1, 6)
	}
	for e := -40; e <= 40; e += 20 {
		x := x0 + int(float64(e+40)/80*float64(w))
		c.Text(x-12, y0+h+22, fmt.Sprintf("%d°", e), m.TextLo, m.Mono, 9)
	}
	for i, pa := range pats {
		pts := make([][2]int, 0, 161)
		for e := -40.0; e <= 40.0; e += 0.5 {
			g := pa.p.GainDBi(0, e)
			if g < -20 {
				g = -20
			}
			pts = append(pts, [2]int{
				x0 + int((e+40)/80*float64(w)),
				y0 + int((10-g)/32*float64(h)),
			})
		}
		c.Polyline(pts, pa.col, 2, 0, nil)
		c.Line(x0+20, y0+20+i*22, x0+50, y0+20+i*22, pa.col, 2, 0)
		c.Text(x0+58, y0+24+i*22, pa.name, m.TextHi, m.Sans, 10)
	}
	c.Text(x0, y0+h+44, "elevation angle — gain costs beamwidth: the 9 dBi antenna wins at the horizon and loses faster off it", m.TextMid, m.Sans, 10)
	return c
}

func pill(c *m.Canvas, x, y int, text string, col m.NRGBA) {
	w := c.Measure(text, m.SansBold, 10) + 20
	c.RoundRect(x, y, w, 22, 11, m.Alpha(col, 0x28), m.Alpha(col, 0x90))
	c.Text(x+10, y+16, text, col, m.SansBold, 10)
}
