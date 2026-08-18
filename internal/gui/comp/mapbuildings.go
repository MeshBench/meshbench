// The buildings layer: what the environment actually holds, on the map.
//
// Footprints come from whatever environment is loaded, through a callback
// the workbench wires - the map stays data-blind. Drawn only when the view
// is close enough for a footprint to be more than a dot, cached per
// viewport, coloured by material, and filterable from the key exactly the
// way node kinds are: the key names a colour, so the key is where somebody
// reaches to stop drawing it.
package comp

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// buildingSpanMaxDeg is how wide (in longitude) the view may be before the
// layer stops drawing: past this a building is smaller than a pixel and the
// map would pay for thousands of invisible polygons.
const buildingSpanMaxDeg = 0.35

// buildingDrawCap bounds one frame's polygons; the key says when the
// viewport holds more than it draws.
const buildingDrawCap = 6000

// buildingMaterials is the taxonomy, in key order.
var buildingMaterials = []string{
	"BRICK", "CONCRETE", "METAL", "STONE", "GLASS", "TIMBER", "MIXED", "UNKNOWN",
}

// materialColour is deliberately its own small palette - materials are
// facts about the ground, not theme accents.
func materialColour(mat string) color.NRGBA {
	switch mat {
	case "BRICK":
		return color.NRGBA{R: 196, G: 106, B: 84, A: 150}
	case "CONCRETE":
		return color.NRGBA{R: 148, G: 152, B: 160, A: 150}
	case "METAL":
		return color.NRGBA{R: 120, G: 160, B: 190, A: 150}
	case "STONE":
		return color.NRGBA{R: 168, G: 150, B: 120, A: 150}
	case "GLASS":
		return color.NRGBA{R: 120, G: 200, B: 210, A: 140}
	case "TIMBER":
		return color.NRGBA{R: 170, G: 130, B: 80, A: 150}
	case "MIXED":
		return color.NRGBA{R: 160, G: 130, B: 170, A: 150}
	}
	return color.NRGBA{R: 130, G: 130, B: 130, A: 130}
}

// buildingsCache remembers one viewport's fetch, so a still camera costs
// nothing per frame.
type buildingsCache struct {
	key   string
	polys []state.BuildingPoly
	total int
}

func (m *MapView) drawBuildings(t *theme.Theme, gtx layout.Context, sz image.Point) {
	if m.BuildingsIn == nil {
		return
	}
	south, west := m.unproject(f32.Pt(0, float32(sz.Y)), sz)
	north, east := m.unproject(f32.Pt(float32(sz.X), 0), sz)
	if east-west > buildingSpanMaxDeg {
		return
	}
	key := fmt.Sprintf("%.4f/%.4f/%.4f/%.4f", south, west, north, east)
	if m.bldCache.key != key {
		polys := m.BuildingsIn(south, west, north, east)
		m.bldCache = buildingsCache{key: key, polys: polys, total: len(polys)}
		if len(polys) > buildingDrawCap {
			m.bldCache.polys = polys[:buildingDrawCap]
		}
	}
	for _, b := range m.bldCache.polys {
		if len(b.Ring) < 3 || m.Layers.hiddenMaterial(b.Material) {
			continue
		}
		var p clip.Path
		p.Begin(gtx.Ops)
		first := m.projectPoint(state.Point{Lat: b.Ring[0][0], Lon: b.Ring[0][1]}, sz)
		p.MoveTo(first)
		for _, v := range b.Ring[1:] {
			p.LineTo(m.projectPoint(state.Point{Lat: v[0], Lon: v[1]}, sz))
		}
		p.Close()
		paint.FillShape(gtx.Ops, materialColour(b.Material),
			clip.Outline{Path: p.End()}.Op())
	}
}

// hiddenMaterial reports whether the key has switched a material off.
func (l *Layers) hiddenMaterial(mat string) bool {
	for i, name := range buildingMaterials {
		if name == mat {
			return l.HideMaterial[i]
		}
	}
	return l.HideMaterial[len(buildingMaterials)-1]
}

// buildingKeyRows is the material key, drawn under the node key while the
// layer is on: one row per material, click to hide, with the truth about
// how much the viewport holds.
func (m *MapView) buildingKeyRows(t *theme.Theme) []layout.Widget {
	if !m.Layers.Buildings {
		return nil
	}
	rows := []layout.Widget{
		Text(t, t.Sz.Caption, t.P.Dim, "materials - click to hide"),
	}
	for i, mat := range buildingMaterials {
		i, mat := i, mat
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if m.Layers.matKeys[i].Clicked(gtx) {
				m.Layers.HideMaterial[i] = !m.Layers.HideMaterial[i]
			}
			return m.Layers.matKeys[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				ink := t.P.Ink
				if m.Layers.HideMaterial[i] {
					ink = t.P.Faint
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						sz := gtx.Dp(8)
						c := materialColour(mat)
						if m.Layers.HideMaterial[i] {
							c.A = 50
						}
						paint.FillShape(gtx.Ops, c,
							clip.Rect{Max: image.Pt(sz, sz)}.Op())
						return layout.Dimensions{Size: image.Pt(sz+gtx.Dp(5), sz)}
					}),
					layout.Rigid(Text(t, t.Sz.Caption, ink, titleCaseMaterial(mat))),
				)
			})
		})
	}
	if m.bldCache.total > buildingDrawCap {
		rows = append(rows, Text(t, t.Sz.Caption, t.P.Faint,
			fmt.Sprintf("%d in view, drawing %d - zoom in", m.bldCache.total, buildingDrawCap)))
	}
	return rows
}

func titleCaseMaterial(m string) string {
	if len(m) == 0 {
		return m
	}
	return m[:1] + func() string {
		out := make([]byte, 0, len(m)-1)
		for i := 1; i < len(m); i++ {
			c := m[i]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			out = append(out, c)
		}
		return string(out)
	}()
}

var _ = widget.Clickable{}
