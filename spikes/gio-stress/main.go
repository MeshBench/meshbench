// The pre-P0 spike from the redesign plan, section 20: two surfaces, one
// binary. Press 1 for the map at 300 nodes / ~2000 stroked links, press 2 for
// a waterfall fed a new texture every frame. Both print measured FPS to
// stdout every second and to /tmp/stress-fps.log, so the numbers in the
// report are read off a log rather than eyeballed from a window.
package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

var (
	bg     = color.NRGBA{R: 0x0b, G: 0x0e, B: 0x11, A: 0xff}
	ink    = color.NRGBA{R: 0xe6, G: 0xe9, B: 0xee, A: 0xff}
	accent = color.NRGBA{R: 0x6e, G: 0xa8, B: 0xfe, A: 0xff}
	good   = color.NRGBA{R: 0x5c, G: 0xbf, B: 0xa8, A: 0xff}
	bad    = color.NRGBA{R: 0xe0, G: 0x8a, B: 0x76, A: 0xff}
)

const mode1Map = 1
const mode2Waterfall = 2
const mode3MapBatched = 3

func main() {
	fixture := "fixtures/fixture-scotland-ireland-strict.json"
	if len(os.Args) > 1 {
		fixture = os.Args[1]
	}
	sc := loadScene(fixture)
	fmt.Printf("stress scene: %s, %d nodes, %d links\n", fixture, len(sc.Nodes), len(sc.Links))

	logf, err := os.Create("/tmp/stress-fps.log")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = logf.Close() }()

	go func() {
		w := new(app.Window)
		w.Option(app.Title("MeshBench stress spike - 1: map  2: waterfall"),
			app.Size(unit.Dp(1400), unit.Dp(900)))
		if err := run(w, sc, logf); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window, sc *scene, logf *os.File) error {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	mode := mode1Map
	var wf *waterfall
	// Auto-cycle so the spike is reproducible without an input tool: each mode
	// gets a fixed window, in a fixed order, and the log line for each mode
	// change makes the transitions found by grep rather than by eye.
	cycleStart := time.Now()
	const cycleEvery = 8 * time.Second
	cycleOrder := []int{mode1Map, mode3MapBatched, mode2Waterfall}
	cycleAt := 0

	var ops op.Ops
	frames := 0
	windowStart := time.Now()
	var lastFPS float64

	for {
		e := w.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)

			for {
				ke, ok := gtx.Event(key.Filter{})
				if !ok {
					break
				}
				if k, ok := ke.(key.Event); ok && k.State == key.Press {
					switch k.Name {
					case "1":
						mode = mode1Map
					case "2":
						mode = mode2Waterfall
						if wf == nil {
							wf = newWaterfall(900, 500)
						}
					case "3":
						mode = mode3MapBatched
					}
				}
			}

			if time.Since(cycleStart) >= cycleEvery {
				cycleStart = time.Now()
				cycleAt = (cycleAt + 1) % len(cycleOrder)
				mode = cycleOrder[cycleAt]
				if mode == mode2Waterfall && wf == nil {
					wf = newWaterfall(900, 500)
				}
				fmt.Fprintf(logf, "%s SWITCH mode=%d\n", time.Now().Format("15:04:05.000"), mode)
				_ = logf.Sync()
				// A switch mid-window mixed frames from two modes into one
				// reading. Restart the window so every fps line belongs to
				// exactly one mode.
				frames = 0
				windowStart = time.Now()
			}

			frames++
			if since := time.Since(windowStart); since >= time.Second {
				lastFPS = float64(frames) / since.Seconds()
				fmt.Fprintf(logf, "%s mode=%d fps=%.1f nodes=%d links=%d\n",
					time.Now().Format("15:04:05.000"), mode, lastFPS, len(sc.Nodes), len(sc.Links))
				_ = logf.Sync()
				frames = 0
				windowStart = time.Now()
			}

			fill(gtx, bg)
			layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := fmt.Sprintf(
							"mode %d (1 map/frame  2 waterfall  3 map/batched)   fps %.1f   nodes %d   links %d   basemap: %d tiles",
							mode, lastFPS, len(sc.Nodes), len(sc.Links), len(demoBasemap))
						l := material.Label(th, unit.Sp(14), lbl)
						l.Color = ink
						if lastFPS > 0 && lastFPS < 55 {
							l.Color = bad
						} else if lastFPS >= 55 {
							l.Color = good
						}
						return l.Layout(gtx)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					switch mode {
					case mode2Waterfall:
						return wf.layout(gtx)
					case mode3MapBatched:
						return drawStressBatched(gtx, sc)
					default:
						return drawStress(gtx, sc)
					}
				}),
			)
			e.Frame(gtx.Ops)
			// Request the next frame immediately rather than waiting for
			// input, which is what makes this an FPS ceiling test rather
			// than an idle app.
			w.Invalidate()
		}
	}
}

// demoBasemap stands in for cached OSM tiles: a coarse grid of textured quads
// under everything else, so the stress test pays the real map's compositing
// cost rather than a flat fill. tiles.go draws one textured quad per real
// tile; this is the same shape of work at a size that does not need a tile
// cache on disk for a half-day spike.
var demoBasemapImg *image.RGBA
var demoBasemap [48]struct{} // 8x6, matched to the loop below

func drawBasemap(gtx layout.Context, sz image.Point) {
	if demoBasemapImg == nil {
		demoBasemapImg = image.NewRGBA(image.Rect(0, 0, 256, 256))
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				demoBasemapImg.Set(x, y, color.RGBA{0x10, 0x14, 0x18, 0xff})
			}
		}
		// A little texture so this is not just a second flat fill - roads and
		// coastline give real tiles edges to composite.
		for i := 0; i < 256; i += 16 {
			for x := 0; x < 256; x++ {
				demoBasemapImg.Set(x, i, color.RGBA{0x16, 0x1c, 0x22, 0xff})
			}
		}
	}
	const cols, rows = 8, 6
	tw, th := float32(sz.X)/cols, float32(sz.Y)/rows
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x0, y0 := float32(c)*tw, float32(r)*th
			off := op.Offset(image.Pt(int(x0), int(y0))).Push(gtx.Ops)
			im := paint.NewImageOp(demoBasemapImg)
			im.Add(gtx.Ops)
			clip.Rect{Max: image.Pt(int(tw)+1, int(th)+1)}.Push(gtx.Ops).Pop()
			paint.PaintOp{}.Add(gtx.Ops)
			off.Pop()
		}
	}
}

func drawStress(gtx layout.Context, sc *scene) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: bg}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	drawBasemap(gtx, sz)

	sc.project(float32(sz.X), float32(sz.Y), 30)

	// Every link is its own stroked path, exactly as the real map draws them
	// today - this is deliberately not batched into one path, because a
	// batched path would not tell us what the real map's per-link draw calls
	// cost.
	for _, l := range sc.Links {
		a, b := sc.Nodes[l.A], sc.Nodes[l.B]
		line(gtx.Ops, f32.Pt(a.X, a.Y), f32.Pt(b.X, b.Y),
			color.NRGBA{R: 0x6e, G: 0xa8, B: 0xfe, A: 0x30}, 1)
	}
	for _, n := range sc.Nodes {
		dot(gtx.Ops, f32.Pt(n.X, n.Y), 4, accent)
	}
	return layout.Dimensions{Size: sz}
}

// drawStressBatched is the same scene, but every link is one segment of a
// single path with one stroke fill, rather than 1223 separate draw calls.
// This answers the question the naive version cannot: whether a shortfall is
// Gio's rasteriser or this program's failure to batch, which is the
// difference between "the toolkit cannot do this" and "the code needs to be
// written properly" - the second is routine engineering, the first would send
// the whole redesign back to the drawing board.
func drawStressBatched(gtx layout.Context, sc *scene) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: bg}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	drawBasemap(gtx, sz)

	sc.project(float32(sz.X), float32(sz.Y), 30)

	var p clip.Path
	p.Begin(gtx.Ops)
	for _, l := range sc.Links {
		a, b := sc.Nodes[l.A], sc.Nodes[l.B]
		p.MoveTo(f32.Pt(a.X, a.Y))
		p.LineTo(f32.Pt(b.X, b.Y))
	}
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x6e, G: 0xa8, B: 0xfe, A: 0x30},
		clip.Stroke{Path: p.End(), Width: 1}.Op())

	// Nodes as one path too: a circle is four cubic segments, and 311 of them
	// batched is one draw call instead of 311.
	var np clip.Path
	np.Begin(gtx.Ops)
	for _, n := range sc.Nodes {
		circleSub(&np, f32.Pt(n.X, n.Y), 4)
	}
	paint.FillShape(gtx.Ops, accent, clip.Outline{Path: np.End()}.Op())

	return layout.Dimensions{Size: sz}
}

// circleSub appends one circle, as four cubic Bezier quadrants, to a path
// already begun - the standard construction, k = 0.5523 for a unit circle.
func circleSub(p *clip.Path, c f32.Point, r float32) {
	const k = 0.5522847498
	p.MoveTo(f32.Pt(c.X+r, c.Y))
	p.CubeTo(f32.Pt(c.X+r, c.Y+r*k), f32.Pt(c.X+r*k, c.Y+r), f32.Pt(c.X, c.Y+r))
	p.CubeTo(f32.Pt(c.X-r*k, c.Y+r), f32.Pt(c.X-r, c.Y+r*k), f32.Pt(c.X-r, c.Y))
	p.CubeTo(f32.Pt(c.X-r, c.Y-r*k), f32.Pt(c.X-r*k, c.Y-r), f32.Pt(c.X, c.Y-r))
	p.CubeTo(f32.Pt(c.X+r*k, c.Y-r), f32.Pt(c.X+r, c.Y-r*k), f32.Pt(c.X+r, c.Y))
	p.Close()
}

func line(ops *op.Ops, a, b f32.Point, c color.NRGBA, w float32) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(a)
	p.LineTo(b)
	paint.FillShape(ops, c, clip.Stroke{Path: p.End(), Width: w}.Op())
}

func dot(ops *op.Ops, at f32.Point, r float32, c color.NRGBA) {
	rr := clip.RRect{
		Rect: image.Rect(int(at.X-r), int(at.Y-r), int(at.X+r), int(at.Y+r)),
		NE:   int(r), NW: int(r), SE: int(r), SW: int(r),
	}
	paint.FillShape(ops, c, rr.Op(ops))
}

func fill(gtx layout.Context, c color.NRGBA) {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// waterfall uploads a brand new image every frame, which is the worst case
// for the "is a texture upload per frame viable" question - a real capture
// would scroll one row in and reuse the rest, so this spike is deliberately
// harder than the real workload.
type waterfall struct {
	w, h  int
	t     float64
	ring  []*image.RGBA
	next  int
}

// ringSize frames, pre-rendered once at start-up. 45 frames at the synthetic
// motion rate is about two seconds of unique content before it visibly
// repeats, which is enough to see it is moving without paying generation cost
// inside the loop under test.
const ringSize = 45

func newWaterfall(w, h int) *waterfall {
	wf := &waterfall{w: w, h: h}
	for i := 0; i < ringSize; i++ {
		wf.t = float64(i) / 30.0
		wf.ring = append(wf.ring, wf.render())
	}
	return wf
}

func (wf *waterfall) layout(gtx layout.Context) layout.Dimensions {
	sz := gtx.Constraints.Max
	img := wf.ring[wf.next%len(wf.ring)]
	wf.next++
	op_ := paint.NewImageOp(img)
	op_.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: sz}
}

func (wf *waterfall) render() *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, wf.w, wf.h))
	// A synthetic spectrogram: a few moving chirps plus noise, which exercises
	// the same per-pixel write pattern a real waterfall does without needing
	// captured IQ on hand for a half-day spike.
	rnd := rand.New(rand.NewSource(int64(wf.t * 1000)))
	for y := 0; y < wf.h; y++ {
		freqFrac := float64(y) / float64(wf.h)
		for x := 0; x < wf.w; x++ {
			timeFrac := float64(x) / float64(wf.w)
			v := 0.06 + 0.03*rnd.Float64()
			for c := 0; c < 3; c++ {
				cf := 0.2 + 0.25*float64(c) + 0.08*math.Sin(wf.t*0.7+float64(c))
				d := math.Abs(freqFrac - cf)
				chirp := math.Exp(-d*d*900) * (0.6 + 0.4*math.Sin(wf.t*3+timeFrac*10+float64(c)))
				v += chirp
			}
			if v > 1 {
				v = 1
			}
			r, g, b := heat(v)
			im.Set(x, y, color.RGBA{r, g, b, 0xff})
		}
	}
	return im
}

func heat(v float64) (uint8, uint8, uint8) {
	// A cheap blue-to-white heat ramp, good enough to look like a waterfall
	// and cheap enough not to be the bottleneck itself.
	r := clamp8(v*2.2*255 - 200)
	g := clamp8(v*2.0*255 - 80)
	b := clamp8(60 + v*400)
	return r, g, b
}

func clamp8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
