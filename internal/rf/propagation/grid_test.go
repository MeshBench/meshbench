package propagation

import "testing"

func flatGrid(south, north, west, east float64, w, h int) HeightGrid {
	g := HeightGrid{South: south, North: north, West: west, East: east, W: w, H: h,
		Heights: make([]float32, w*h)}
	for i := range g.Heights {
		g.Heights[i] = 100
	}
	return g
}

func TestGridLossRowZeroIsNorth(t *testing.T) {
	g := flatGrid(56.0, 56.5, -3.6, -3.0, 32, 32)
	p := GridLossParams{
		StLat: 56.01, StLon: -3.3, StAltM: 120,
		RasterW: 16, RasterH: 16,
		South: 56.0, North: 56.5, West: -3.6, East: -3.0,
		RemoteHeightM: 1.5, FreqMHz: 869.618, Steps: 64,
	}
	losses := GridLossCPU(g, p)
	north := losses[0*16+8]
	south := losses[15*16+8]
	if !(north > south) {
		t.Fatalf("station at the south edge: north row loss %.1f, south row %.1f - "+
			"the grid is upside down", north, south)
	}
}
