// Where the CARTO API key comes from. CARTO's raster basemaps serve an
// "API KEY REQUIRED" watermark on every tile without one, and those
// watermarks cache like real answers.
package basemap

import (
	"os"
	"strings"
)

// defaultCartoKey is stamped by the release pipeline with -ldflags -X, from
// a repository secret, so a downloaded build serves CARTO tiles out of the
// box. It never appears in the source tree; a source build without the
// stamp simply has no default. The environment always wins over it.
var defaultCartoKey string

// CartoKey is the CARTO API key this process will use: the operator's
// environment first, then a .carto-key file in the working directory - the
// gitignored way a source checkout carries the key for development - then
// whatever the build was stamped with, else empty.
func CartoKey() string {
	if k := os.Getenv("MESHBENCH_CARTO_KEY"); k != "" {
		return k
	}
	if b, err := os.ReadFile(".carto-key"); err == nil {
		if k := strings.TrimSpace(string(b)); k != "" {
			return k
		}
	}
	return defaultCartoKey
}

// DefaultID is the basemap a session draws when nobody has chosen one.
//
// OpenStreetMap without a key, because the CARTO layers answer a keyless
// request with an "API KEY REQUIRED" tile and a first run that defaulted to one
// opened onto a wall of watermarks. With a key the CARTO dark map is the better
// ground for data, which is what it was designed to be.
//
// Here rather than beside the renderer because two callers need the same
// answer: the window, which draws it, and map.basemap, which reports it. They
// disagreed - the verb answered an empty id on any profile where nobody had
// chosen, while the window was plainly drawing carto-dark.
func DefaultID() string {
	if CartoKey() != "" {
		return "carto-dark"
	}
	return "osm"
}
