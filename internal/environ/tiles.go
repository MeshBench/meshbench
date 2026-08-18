// Tile-per-file building storage: loaded on demand, cached, offline-honest.
package environ

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// TileZoom is the slippy-map zoom the store shards at. Fourteen puts a tile
// near 2.4 x 1.5 km at UK latitudes - a few thousand buildings in a town,
// single-digit megabytes at worst.
const TileZoom = 14

// TileStore reads tiles written by tools/envgen from a directory, caching
// what it loads. Missing tiles are counted, never invented: a query over
// ground the dataset does not cover says so through Missing.
type TileStore struct {
	Dir string

	mu      sync.Mutex
	tiles   map[[2]int][]Building
	missing map[[2]int]bool
}

// OpenTiles points a store at a directory. The directory not existing is not
// an error here - every tile will simply be missing, and Missing will say so.
func OpenTiles(dir string) *TileStore {
	return &TileStore{Dir: dir, tiles: map[[2]int][]Building{}, missing: map[[2]int]bool{}}
}

// tileXY is the slippy tile containing a point.
func tileXY(lat, lon float64) (int, int) {
	n := float64(int(1) << TileZoom)
	x := int((lon + 180) / 360 * n)
	latRad := lat * math.Pi / 180
	y := int((1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n)
	return x, y
}

// TilePath is where one tile lives, exported so envgen writes where the
// store reads.
func TilePath(dir string, x, y int) string {
	return filepath.Join(dir, fmt.Sprintf("z%d", TileZoom), fmt.Sprint(x), fmt.Sprint(y)+".jsonl.gz")
}

func (s *TileStore) Buildings(minLat, minLon, maxLat, maxLon float64) []Building {
	x0, y1 := tileXY(minLat, minLon)
	x1, y0 := tileXY(maxLat, maxLon)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	var out []Building
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			out = append(out, s.tile(x, y)...)
		}
	}
	return out
}

// Missing is how many distinct tiles have been asked for and not found - the
// offline-mode honesty counter the UI surfaces.
func (s *TileStore) Missing() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.missing)
}

func (s *TileStore) tile(x, y int) []Building {
	k := [2]int{x, y}
	s.mu.Lock()
	if b, ok := s.tiles[k]; ok {
		s.mu.Unlock()
		return b
	}
	s.mu.Unlock()

	var loaded []Building
	f, err := os.Open(TilePath(s.Dir, x, y))
	if err == nil {
		if gz, err := gzip.NewReader(bufio.NewReader(f)); err == nil {
			dec := json.NewDecoder(gz)
			for {
				var b Building
				if err := dec.Decode(&b); err != nil {
					break
				}
				loaded = append(loaded, b)
			}
			_ = gz.Close()
		}
		_ = f.Close()
	}

	s.mu.Lock()
	if err != nil {
		s.missing[k] = true
	}
	s.tiles[k] = loaded
	s.mu.Unlock()
	return loaded
}

// WriteTile writes one tile the way the store reads it - envgen's output
// side, here so format knowledge lives in one file.
func WriteTile(dir string, x, y int, buildings []Building) error {
	path := TilePath(dir, x, y)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	enc := json.NewEncoder(gz)
	for _, b := range buildings {
		if err := enc.Encode(b); err != nil {
			_ = gz.Close()
			_ = f.Close()
			return err
		}
	}
	if err := gz.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// TileFor is which tile a building's first vertex lands in - envgen's
// sharding rule.
func TileFor(b Building) (int, int, bool) {
	if len(b.Footprint) == 0 {
		return 0, 0, false
	}
	x, y := tileXY(b.Footprint[0][0], b.Footprint[0][1])
	return x, y, true
}
