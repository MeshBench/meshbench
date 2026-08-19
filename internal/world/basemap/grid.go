// Where the tiles are: the slippy-map grid, and which of them a box covers.
//
// Separate from the store because this is arithmetic with no state and no
// network, and because it is the half that has to be right about the edges.
// The store failing to fetch a tile leaves a grey square; this failing to
// bound a box left a loop counting from the most negative int64 there is.
package basemap

import "math"

// MaxLat is where Web Mercator stops. Beyond it the projection runs to
// infinity, which is why every slippy map in the world is square.
const MaxLat = 85.0511287798066

// TilesFor lists the tiles a bounding box needs at a zoom.
//
// The box is clamped to the world first, and that is not tidiness. A viewport
// zoomed right out asks for a box far larger than the planet - hundreds of
// degrees of latitude - and the projection has no answer there: past +-90 the
// argument to the logarithm goes negative, tileXY returned NaN, and converting
// NaN to an int on amd64 gives the most negative int64 there is. The loop
// below then counted from -9223372036854775808 upwards, appending as it went,
// once per frame. That is the whole of the "zooming out hangs the machine"
// bug: not a slow frame, an unbounded one.
//
// So the result is bounded by the world by construction: never more than
// 4^zoom tiles, because that is how many exist.
func TilesFor(south, north, west, east float64, zoom int) [][2]int {
	if zoom < 0 || zoom > 30 {
		return nil
	}
	south, north = clampLat(south), clampLat(north)
	west, east = clampLon(west), clampLon(east)
	if north < south {
		south, north = north, south
	}
	if east < west {
		west, east = east, west
	}
	last := int(math.Exp2(float64(zoom))) - 1

	x0, y0 := tileXY(north, west, zoom)
	x1, y1 := tileXY(south, east, zoom)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	x0, x1 = clampTile(x0, last), clampTile(x1, last)
	y0, y1 = clampTile(y0, last), clampTile(y1, last)

	out := make([][2]int, 0, (x1-x0+1)*(y1-y0+1))
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			out = append(out, [2]int{x, y})
		}
	}
	return out
}

// clampLat holds a latitude inside the projection, NaN included - a NaN that
// reaches the arithmetic below is the failure this function exists to stop, so
// it is turned into a number here rather than propagated.
func clampLat(lat float64) float64 {
	switch {
	case math.IsNaN(lat):
		return 0
	case lat > MaxLat:
		return MaxLat
	case lat < -MaxLat:
		return -MaxLat
	}
	return lat
}

func clampLon(lon float64) float64 {
	switch {
	case math.IsNaN(lon):
		return 0
	case lon > 180:
		return 180
	case lon < -180:
		return -180
	}
	return lon
}

// clampTile holds an index on the tile grid. The east and south edges land
// exactly on the last boundary, which floors to one past the last tile.
func clampTile(v, last int) int {
	switch {
	case v < 0:
		return 0
	case v > last:
		return last
	}
	return v
}

// tileXY is the slippy-map projection. Its inputs must already be on the
// planet: it is called from TilesFor, which clamps them.
func tileXY(lat, lon float64, zoom int) (int, int) {
	n := math.Exp2(float64(zoom))
	x := int(math.Floor((lon + 180) / 360 * n))
	latRad := lat * math.Pi / 180
	y := int(math.Floor((1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n))
	return x, y
}
