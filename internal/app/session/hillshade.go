// Hillshade, from the elevation the engine actually uses.
//
// 10.7 asks for terrain shading and an elevation source attribution, and the
// two have to be the same thing or the attribution is decoration. A
// topographic basemap tile would look like terrain while being somebody else's
// data; this is the DEM the path losses are cut against, so what is on the
// screen and what is in the answer are the same ground.
package session

import (
	"image"
	"image/color"
	"math"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/propagation"
)

// shadeGrid is the hillshade resolution. Coarser than the DEM on purpose: this
// is a picture of the shape of the ground, not a source of heights, and 256
// square over a national view is already finer than the screen.
const shadeGrid = 256

// hillshade renders the visible area as a shaded relief.
func (s *Sim) hillshade(south, north, west, east float64) (*state.Coverage, error) {
	g, _ := propagation.RasteriseHeights(s.terrain(), south, north, west, east,
		shadeGrid, shadeGrid)

	img := image.NewRGBA(image.Rect(0, 0, shadeGrid, shadeGrid))
	// The usual convention: light from the north-west, high in the sky, so
	// the shading reads as relief rather than as a map of aspect.
	const (
		azimuth  = 315.0
		altitude = 45.0
	)
	az := (360 - azimuth + 90) * math.Pi / 180
	alt := altitude * math.Pi / 180

	// Metres per cell, which decides how steep a slope looks. Getting this
	// wrong is how a hillshade of a whole country comes out looking like
	// crumpled paper.
	latSpanM := (north - south) * 111320
	lonSpanM := (east - west) * 111320 * math.Cos((north+south)/2*math.Pi/180)
	dy := latSpanM / shadeGrid
	dx := lonSpanM / shadeGrid
	if dx <= 0 || dy <= 0 {
		return nil, nil
	}

	known := 0
	for y := 1; y < shadeGrid-1; y++ {
		for x := 1; x < shadeGrid-1; x++ {
			z, ok := heightAt(g, x, y)
			if !ok {
				continue
			}
			known++
			zl, okl := heightAt(g, x-1, y)
			zr, okr := heightAt(g, x+1, y)
			zu, oku := heightAt(g, x, y-1)
			zd, okd := heightAt(g, x, y+1)
			if !okl || !okr || !oku || !okd {
				continue
			}
			_ = z
			// Horn's method, the standard one, on the four neighbours.
			gx := (zr - zl) / (2 * dx)
			gy := (zd - zu) / (2 * dy)
			slope := math.Atan(math.Hypot(gx, gy))
			aspect := math.Atan2(gy, -gx)
			v := math.Sin(alt)*math.Cos(slope) +
				math.Cos(alt)*math.Sin(slope)*math.Cos(az-aspect)
			if v < 0 {
				v = 0
			}
			// Relief in both directions, not shadow alone. Shadow-only was
			// black at up to 43% alpha, which over the carto-dark basemap is
			// invisible by construction - the layer computed, drew, and
			// changed nothing anybody could see. Slopes facing the light
			// brighten, slopes facing away darken, and flat ground is left
			// exactly as the basemap painted it.
			const flat = 0.70710678 // sin(45 deg): what level ground scores
			d := v - flat
			var px color.RGBA
			if d > 0 {
				a := d * 300
				if a > 170 {
					a = 170
				}
				// White, premultiplied.
				px = color.RGBA{R: uint8(a), G: uint8(a), B: uint8(a), A: uint8(a)}
			} else {
				a := -d * 300
				if a > 170 {
					a = 170
				}
				px = color.RGBA{A: uint8(a)}
			}
			// The grid's y=0 is the south edge; an image's is the top. Not
			// flipping this is a hillshade lit from the south-west that looks
			// like a valley wherever there is a hill.
			img.SetRGBA(x, shadeGrid-1-y, px)
		}
	}
	if known == 0 {
		return nil, nil
	}
	return &state.Coverage{
		Node: "hillshade", Image: img,
		South: south, North: north, West: west, East: east,
		Cells: shadeGrid * shadeGrid, NoDataCells: shadeGrid*shadeGrid - known,
	}, nil
}

// heightAt reads the grid, reporting whether the DEM covered that cell.
func heightAt(g propagation.HeightGrid, x, y int) (float64, bool) {
	i := y*shadeGrid + x
	if i < 0 || i >= len(g.Heights) {
		return 0, false
	}
	h := g.Heights[i]
	// A sentinel, not NaN: the grid says so, and a NaN check here would pass
	// for a value that is merely very negative.
	if h == propagation.NoDataHeight {
		return 0, false
	}
	return float64(h), true
}
