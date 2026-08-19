// What survives a restart.
//
// Nothing did: the GPU switch, the tile cache bound and where the cache lives
// were all decided again every launch, so a machine configured on Tuesday was
// an unconfigured machine on Wednesday. The scenario itself deliberately stays
// in the fixture - this file is about the machine, not the study.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// Prefs are the machine-level choices, as the file stores them.
type Prefs struct {
	// TileCacheDir is where terrain tiles live on disk. Empty means the
	// default under the user cache directory.
	TileCacheDir string `json:"tile_cache_dir,omitempty"`
	// TileCacheGB bounds the decoded tiles held in memory.
	TileCacheGB float64 `json:"tile_cache_gb,omitempty"`
	// GPU is nil until somebody has chosen; a pointer because "off" and
	// "never said" are different answers and only one of them lets the
	// hardware default decide.
	GPU *bool `json:"gpu,omitempty"`
	// Basemap is the chosen map layer's ID - carto-dark, carto-light,
	// esri-topo. Empty means the default.
	Basemap string `json:"basemap,omitempty"`
	// RFMode is which physics decides reception - "calculated" (default) or
	// "waveform". A machine-level choice like the GPU switch: the operator
	// picked a physics, and the pick survives a restart.
	RFMode string `json:"rf_mode,omitempty"`
	// Realism is the RF Simulation section's imperfection switches, kept
	// with the mode for the same reason.
	OscPPM        float64 `json:"osc_ppm,omitempty"`
	MultipathDB   float64 `json:"multipath_db,omitempty"`
	FadingHz      float64 `json:"fading_hz,omitempty"`
	ImplLossDB    float64 `json:"impl_loss_db,omitempty"`
	SaturationDBm float64 `json:"saturation_dbm,omitempty"`
	// EnvironmentDir is where the building tiles live; empty is bare earth.
	EnvironmentDir string `json:"environment_dir,omitempty"`
	// CoverageCells is the coverage raster's long edge; zero is the default.
	CoverageCells int `json:"coverage_cells,omitempty"`
}

// prefsPath is ~/.config/meshcoresim/workbench2.json, or empty when the
// platform cannot say where config lives.
func prefsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "meshcoresim", "workbench2.json")
}

// LoadPrefs reads the settings file and applies it to the session.
//
// Called by the command, once, before anything runs - and deliberately not by
// Register, so a test's session never depends on what the developer's own
// machine happens to have chosen.
func (s *Sim) LoadPrefs() {
	s.persist = true
	path := prefsPath()
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var p Prefs
	if err := json.Unmarshal(b, &p); err != nil {
		return
	}
	s.prefs = p
	if p.TileCacheGB > 0 {
		s.tileCacheTiles = int(p.TileCacheGB * tilesPerGB)
	}
	if p.GPU != nil {
		// A remembered choice is a choice: the hardware default does not get
		// another go at it.
		s.gpuAsked = true
		s.gpuWarm = *p.GPU
	}
	if p.RFMode == "waveform" {
		s.rfMode = "waveform"
	}
	s.envDir = p.EnvironmentDir
	s.covCells = p.CoverageCells
	s.realism = state.RFRealism{
		OscPPM: p.OscPPM, MultipathDB: p.MultipathDB, FadingHz: p.FadingHz,
		ImplLossDB: p.ImplLossDB, SaturationDBm: p.SaturationDBm,
	}
}

// savePrefs writes the current choices, if persistence is on.
//
// Small enough to write inline from a verb: the file is a couple of hundred
// bytes, and a change that does not survive a crash between "set" and "quit"
// is a change that was never really made.
func (s *Sim) savePrefs() {
	if !s.persist {
		return
	}
	path := prefsPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	b, err := json.MarshalIndent(s.prefs, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(b, '\n'), 0o644)
}

// Basemap is the remembered map layer's ID, for the command to hand the map
// at startup. Empty means nobody has chosen.
func (s *Sim) Basemap() string { return s.prefs.Basemap }

// registerBasemap remembers which map is under everything.
//
// The session never draws the map; it only keeps the choice, because the
// choice is the kind of thing that was decided again every launch until the
// settings file existed.
func registerBasemap(st *state.Store, s *Sim) {
	st.Handle("map.basemap", func(w *state.World, p any) (any, error) {
		if id, ok := stringField(p, "id"); ok && id != "" {
			s.prefs.Basemap = id
			s.savePrefs()
			w.Say("the basemap is " + id + ", here and on the next launch")
		}
		return map[string]any{"id": s.prefs.Basemap}, nil
	})
}

// tileCacheDir is where terrain tiles live: the chosen place, or the default.
func (s *Sim) tileCacheDir() string {
	if s.prefs.TileCacheDir != "" {
		return s.prefs.TileCacheDir
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "meshcoresim", "terrain")
}

// validateCacheDir refuses the moves that cannot work before any file moves.
func validateCacheDir(oldDir, newDir string) (string, error) {
	abs, err := filepath.Abs(newDir)
	if err != nil {
		return "", err
	}
	if oldDir != "" {
		if same, err := nestedOrSame(oldDir, abs); err == nil && same {
			return "", fmt.Errorf("the new cache cannot be the old one or inside it")
		}
		if same, err := nestedOrSame(abs, oldDir); err == nil && same {
			return "", fmt.Errorf("the new cache cannot contain the old one")
		}
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", abs, err)
	}
	// A write test, because MkdirAll succeeding says the directory exists,
	// not that tiles can land in it.
	probe := filepath.Join(abs, ".meshcoresim-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return "", fmt.Errorf("cannot write into %s: %w", abs, err)
	}
	_ = os.Remove(probe)
	return abs, nil
}

// nestedOrSame reports whether inner is outer or lives underneath it.
func nestedOrSame(outer, inner string) (bool, error) {
	rel, err := filepath.Rel(outer, inner)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

// moveTree moves every file under oldDir into newDir, preserving layout.
//
// Rename first - free on the same filesystem - and copy-then-delete when the
// kernel says the two are different devices. Progress by file, because tens of
// thousands of tiles across filesystems is gigabytes, and a move with nothing
// on screen is indistinguishable from a hang.
func moveTree(oldDir, newDir string, progress func(done, total int)) (int, error) {
	var files []string
	err := filepath.WalkDir(oldDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(oldDir, p)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	for i, rel := range files {
		src := filepath.Join(oldDir, rel)
		dst := filepath.Join(newDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return i, err
		}
		if err := os.Rename(src, dst); err != nil {
			if err := copyFile(src, dst); err != nil {
				return i, err
			}
			_ = os.Remove(src)
		}
		if progress != nil && (i == 0 || (i+1)%256 == 0 || i+1 == len(files)) {
			progress(i+1, len(files))
		}
	}
	return len(files), nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
