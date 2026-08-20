package resource

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
)

// DirCache is a provider for the caches that are simply a directory of files.
//
// Terrain, basemaps, building footprints and map tiles all work the same way:
// the application fills them as the operator uses the map, nobody chose to
// download any of it, and until now nobody could see it either. The package
// comment names the case this exists for - a terrain cache that had reached
// 7.1 GB with no way to look at it, let alone delete it, without a terminal.
//
// One row per cache rather than per file. A row per terrain tile would be tens
// of thousands of rows about nothing: what an operator wants to know is how
// much of the disk this has taken and whether it can go.
type DirCache struct {
	K Kind
	// Label names the row, and Dir is the directory it measures.
	Label string
	Dir   string
	// Purpose is what the bytes are for, said in the row.
	Purpose string
	// Terms is the attribution the data arrived under, shown on demand. The
	// data is somebody else's; saying so is not optional.
	Terms string
}

func (d *DirCache) Kind() Kind { return d.K }

// List measures the directory. It walks rather than trusting a stored figure:
// the number is the point, and a stale one is worse than none.
func (d *DirCache) List(_ context.Context) ([]Row, error) {
	row := Row{
		Kind: d.K, Name: d.Label, Path: d.Dir, Why: d.Purpose,
		Licensed: d.Terms != "",
		// Filled by using the application rather than by asking, which is why
		// it can grow unnoticed - and why it is worth showing.
		Auto: true,
	}
	var total int64
	var files int
	err := filepath.WalkDir(d.Dir, func(p string, e fs.DirEntry, walkErr error) error {
		// A file that vanished mid-walk is a cache being used, not a fault:
		// skip it and keep counting rather than abandoning the measurement.
		if walkErr != nil || e == nil || e.IsDir() {
			return nil //nolint:nilerr // a disappearing cache entry is not an error
		}
		if fi, err := e.Info(); err == nil {
			total += fi.Size()
			files++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	row.Bytes = total
	if files == 0 {
		row.State = Available
		row.Path = ""
		row.Why = d.Purpose + "; nothing cached yet"
		return []Row{row}, nil
	}
	row.State = OnDisk
	return []Row{row}, nil
}

// Remove empties the cache, leaving the directory itself so the application can
// refill it without having to create anything.
func (d *DirCache) Remove(_ context.Context, row Row) error {
	if row.Path == "" {
		return nil
	}
	entries, err := os.ReadDir(row.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(row.Path, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Licence is the attribution for this cache. name and version are ignored: a
// directory of tiles carries one set of terms, not one per file.
func (d *DirCache) Licence(_, _ string) string { return d.Terms }
