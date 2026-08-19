// The ground the propagation maths asks about.
package propagation

// Terrain supplies ground elevation. An interface because the maths does not
// care whether the heights came from a downloaded tile, a cache or a test.
type Terrain interface {
	// ElevationM returns metres above sea level, and whether the point is
	// covered by data at all. A raster over a coastline asks for points that no
	// tile covers, and inventing zero for them would draw sea-level coverage
	// across the Atlantic.
	ElevationM(lat, lon float64) (float64, bool)
}
