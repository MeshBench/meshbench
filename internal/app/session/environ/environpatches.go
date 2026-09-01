// The ground a pull covers, and where what it brings back is kept.
//
// Both sources ask for the same patches and both land in the same cache, so
// this is the one place that decides what a network's footprint is worth
// asking for.
package environ

import (
	"crypto/sha256"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	worldenv "github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// environPatchKm is how far around each node footprints are pulled. A
// building changes a path where the path is low and close - at its ends -
// so the pull is node-centred patches, not the network's bounding box: a
// national network's box is mostly sea and empty country, and pulling it
// would be refused for size while every town that matters went unserved.
const environPatchKm = 2.5

// llBox is one patch of ground, in degrees.
type llBox struct{ South, North, West, East float64 }

// environPatches is the pull's footprint: a patch around every node, with
// overlapping patches merged so a town of nodes is asked for once.
func environPatches(nodes []scenario.Node) ([]llBox, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes loaded; a pull needs a map to cover")
	}
	boxes := make([]llBox, 0, len(nodes))
	for _, n := range nodes {
		dLat := environPatchKm / 111.32
		dLon := environPatchKm / (111.32 * math.Cos(n.Position.Lat*math.Pi/180))
		boxes = append(boxes, llBox{
			South: n.Position.Lat - dLat, North: n.Position.Lat + dLat,
			West: n.Position.Lon - dLon, East: n.Position.Lon + dLon,
		})
	}
	// Merge until stable. A merged box can newly overlap a third, so one
	// pass is not enough; the count only ever shrinks, so it ends.
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(boxes); i++ {
			for j := i + 1; j < len(boxes); j++ {
				if boxes[i].East < boxes[j].West || boxes[j].East < boxes[i].West ||
					boxes[i].North < boxes[j].South || boxes[j].North < boxes[i].South {
					continue
				}
				boxes[i] = llBox{
					South: math.Min(boxes[i].South, boxes[j].South),
					North: math.Max(boxes[i].North, boxes[j].North),
					West:  math.Min(boxes[i].West, boxes[j].West),
					East:  math.Max(boxes[i].East, boxes[j].East),
				}
				boxes = append(boxes[:j], boxes[j+1:]...)
				merged = true
				j--
			}
		}
	}
	return boxes, nil
}

func boxAreaKm2(b llBox) float64 {
	midLat := (b.South + b.North) / 2
	return (b.North - b.South) * 111.32 * (b.East - b.West) * 111.32 *
		math.Cos(midLat*math.Pi/180)
}

func patchesAreaKm2(patches []llBox) float64 {
	var a float64
	for _, b := range patches {
		a += boxAreaKm2(b)
	}
	return a
}

// environCacheDir is where one pull's tiles live: keyed by source and the
// patch set, so moving the network pulls fresh ground and reopening the same
// network reuses the cache without asking.
func environCacheDir(source string, patches []llBox) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(patches))
	for _, b := range patches {
		keys = append(keys, fmt.Sprintf("%.4f/%.4f/%.4f/%.4f", b.South, b.North, b.West, b.East))
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(strings.Join(keys, "|")))
	return filepath.Join(cache, "meshbench", "environment",
		fmt.Sprintf("%s-%x", source, sum[:4])), nil
}

// hasTiles reports whether a previous pull already landed here. The store
// lays tiles out as z<zoom>/x/y.jsonl.gz, so the zoom directory existing
// with anything in it is the fact that matters.
func hasTiles(dir string) bool {
	entries, err := os.ReadDir(filepath.Join(dir, fmt.Sprintf("z%d", worldenv.TileZoom)))
	return err == nil && len(entries) > 0
}
