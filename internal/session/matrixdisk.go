// The measured link matrix, kept across restarts.
//
// A path loss depends on the geometry fingerprint and nothing else, so a
// matrix measured yesterday for this exact network is this network's matrix.
// The engine already carries it across in-process rebuilds; this carries it
// across launches, which turns the eight-second warm on a national fixture
// into a file read.
package session

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// matrixDir is where matrices live, or empty when persistence is off - in
// tests, where writing to the developer's cache would make runs depend on
// each other.
func (s *Sim) matrixDir() string {
	if !s.persist {
		return ""
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "meshcoresim", "matrix")
}

// keepMatrices bounds the directory: a matrix per geometry, and a workbench
// that has seen many networks does not need all of them forever.
const keepMatrices = 24

// saveMatrix writes one matrix under its fingerprint, then prunes.
func saveMatrix(dir string, fp uint64, m map[[2]int]float64) {
	if dir == "" || len(m) == 0 {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, fmt.Sprintf("%016x.gob", fp))
	f, err := os.CreateTemp(dir, ".matrix-*")
	if err != nil {
		return
	}
	if err := gob.NewEncoder(f).Encode(m); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return
	}
	_ = f.Close()
	// Rename over, so a crash mid-write never leaves a torn matrix under a
	// real fingerprint.
	_ = os.Rename(f.Name(), path)
	pruneMatrices(dir)
}

// loadMatrix reads the matrix for a fingerprint, or nil.
func loadMatrix(dir string, fp uint64) map[[2]int]float64 {
	if dir == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%016x.gob", fp)))
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var m map[[2]int]float64
	if err := gob.NewDecoder(f).Decode(&m); err != nil {
		return nil
	}
	return m
}

// pruneMatrices drops the oldest beyond the bound.
func pruneMatrices(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type aged struct {
		name string
		mod  int64
	}
	var files []aged
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".gob" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, aged{name: e.Name(), mod: info.ModTime().UnixNano()})
	}
	if len(files) <= keepMatrices {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod > files[j].mod })
	for _, f := range files[keepMatrices:] {
		_ = os.Remove(filepath.Join(dir, f.name))
	}
}
