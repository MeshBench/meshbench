// What a raster may be asked for, and what it may not. The dimensions arrive
// from a box somebody typed and a grid somebody chose, and neither is checked
// by anything upstream: profiles have been capped since there were profiles,
// and the raster the profiles fill was the one that was not.

package coverage

import "fmt"

// maxRasterCells is the ceiling on a raster, and it exists for the reason
// maxSamples exists on a profile: a dimension that arrives from a box somebody
// typed is not a dimension anybody chose. A Cell is forty bytes, so a 50000
// square request asks for a hundred gigabytes, and the allocation does not
// fail politely - it takes the process, and with it a session that has no
// autosave. The number is the workbench's own ceiling squared: 4096 on the
// long edge is the finest grid the map offers, so a square raster at that edge
// is the largest anything here has a reason to ask for.
const maxRasterCells = 4096 * 4096

// checkRasterSize refuses a raster before a byte is allocated for it, naming
// the limit so the answer is a grid to ask for rather than a mystery.
func checkRasterSize(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("coverage: raster is %dx%d", w, h)
	}
	// Divided rather than multiplied: the product of two ambitious dimensions
	// is the arithmetic being guarded against, and on a 32-bit build it wraps.
	if w > maxRasterCells/h {
		return fmt.Errorf("coverage: raster is %dx%d, over the limit of %d cells; ask for a coarser grid",
			w, h, maxRasterCells)
	}
	return nil
}
