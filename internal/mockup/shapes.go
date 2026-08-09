package mockup

import "math"

const pi = math.Pi

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func sqrt32(v float32) float32 { return float32(math.Sqrt(float64(v))) }
func cos32(a float64) float32  { return float32(math.Cos(a)) }
func sin32(a float64) float32  { return float32(math.Sin(a)) }
func Sin(x float64) float64    { return math.Sin(x) }
func Cos(x float64) float64    { return math.Cos(x) }
func Exp(x float64) float64    { return math.Exp(x) }
func Sqrt(x float64) float64   { return math.Sqrt(x) }
func Pow(x, y float64) float64 { return math.Pow(x, y) }
func Floor(x float64) float64  { return math.Floor(x) }

// roundRectPath approximates the corner arcs with short line segments. Twelve
// per corner is well below the point where the 2x downsample can show facets.
func roundRectPath(x, y, w, h, r float32) [][2]float32 {
	if r < 0 {
		r = 0
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	const seg = 12
	pts := make([][2]float32, 0, seg*4+4)
	corner := func(cx, cy float32, from float64) {
		for i := 0; i <= seg; i++ {
			a := from + float64(i)/seg*(pi/2)
			pts = append(pts, [2]float32{cx + r*cos32(a), cy + r*sin32(a)})
		}
	}
	corner(x+w-r, y+h-r, 0)    // bottom-right
	corner(x+r, y+h-r, pi/2)   // bottom-left
	corner(x+r, y+r, pi)       // top-left
	corner(x+w-r, y+r, 3*pi/2) // top-right
	return pts
}

func reverse(p [][2]float32) [][2]float32 {
	out := make([][2]float32, len(p))
	for i := range p {
		out[i] = p[len(p)-1-i]
	}
	return out
}
