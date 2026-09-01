// What survives a restart.
//
// Nothing did: the GPU switch, the tile cache bound and where the cache lives
// were all decided again every launch, so a machine configured on Tuesday was
// an unconfigured machine on Wednesday. The scenario itself deliberately stays
// in the fixture - this file is about the machine, not the study.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/diag"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// Prefs are the machine-level choices, as the file stores them.
type Prefs struct {
	// TileCacheDir is where terrain tiles live on disk. Empty means the
	// default under the user cache directory.
	TileCacheDir string `json:"tile_cache_dir,omitempty"`

	// UnverifiedWiring runs boards nobody has watched boot yet. Kept because
	// somebody developing firmware for one board sets it once, not on every
	// launch.
	UnverifiedWiring bool `json:"unverified_wiring,omitempty"`
	// TileCacheGB bounds the decoded tiles held in memory.
	TileCacheGB float64 `json:"tile_cache_gb,omitempty"`
	// TerrainDownloads is whether terrain may be fetched without asking. A
	// pointer for the same reason as GPU: "not yet asked" is a third state,
	// and it is the one a fresh install is in. A national network's ground is
	// several hundred megabytes, which is not a thing to spend on somebody's
	// tethered phone because they opened the application.
	TerrainDownloads *bool `json:"terrain_downloads,omitempty"`
	// GPU is nil until somebody has chosen; a pointer because "off" and
	// "never said" are different answers and only one of them lets the
	// hardware default decide.
	GPU *bool `json:"gpu,omitempty"`
	// KeepAbove is whether panels in their own windows stay above the main
	// one. A pointer for the same reason as GPU: the default is on, and a
	// machine that turned it off has to be distinguishable from one that was
	// never asked. Linux only - everywhere else always-on-top is a property
	// of a normal window and costs nothing.
	KeepAbove *bool `json:"keep_above,omitempty"`
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

// prefsPath is ~/.config/meshbench/workbench2.json, the file a test pointed
// somewhere else, or empty when the platform cannot say where config lives.
func (s *Sim) prefsPath() string {
	if s.prefsFile != "" {
		return s.prefsFile
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "meshbench", "workbench2.json")
}

// LoadPrefs reads the settings file and applies it to the session.
//
// Called by the command, once, before anything runs - and deliberately not by
// Register, so a test's session never depends on what the developer's own
// machine happens to have chosen.
//
// A file that cannot be read is returned rather than swallowed. Silence there
// is indistinguishable from a machine nobody has configured: the GPU choice,
// the RF mode, the cache bound, the environment directory and the coverage
// resolution all quietly back to their defaults, with nothing to say why.
func (s *Sim) LoadPrefs() error {
	s.persist = true
	path := s.prefsPath()
	if path == "" {
		return nil
	}
	p, err := readPrefs(path)
	if err != nil {
		diag.Printf("prefs", "%v", err)
		return err
	}
	if p == nil {
		// No file yet: nothing has been chosen, and nothing is applied. The
		// defaults below are the tile cache's own, not this file's.
		return nil
	}
	s.prefs = *p
	if p.TileCacheGB > 0 {
		s.tileCacheTiles = int(p.TileCacheGB * tilesPerGB)
	}
	s.applyMemoryCeiling()
	if p.GPU != nil {
		// A remembered choice is a choice: the hardware default does not get
		// another go at it.
		s.gpuAsked = true
		s.gpuWarm = *p.GPU
	}
	if p.RFMode == "waveform" {
		s.rfMode = "waveform"
	}
	s.unverifiedWiring = p.UnverifiedWiring
	s.envDir = p.EnvironmentDir
	s.covCells = p.CoverageCells
	s.realism = state.RFRealism{
		OscPPM: p.OscPPM, MultipathDB: p.MultipathDB, FadingHz: p.FadingHz,
		ImplLossDB: p.ImplLossDB, SaturationDBm: p.SaturationDBm,
	}
	return nil
}

// savePrefs writes the current choices, if persistence is on, and says so when
// it cannot.
//
// A setting somebody has been told is remembered and is not is worse than one
// that was never offered, and a full or read-only disk said nothing at all
// here: every error was discarded. The failure goes to the world's log, where
// the verb that made the promise is talking, and to the prefs diagnostic for
// the detail. The error comes back as well, for the verbs whose own sentence
// is a promise about the next launch.
func (s *Sim) savePrefs(w *state.World) error {
	if !s.persist {
		return nil
	}
	path := s.prefsPath()
	if path == "" {
		return nil
	}
	if err := writePrefs(path, s.prefs); err != nil {
		diag.Printf("prefs", "%v", err)
		if w != nil {
			w.Say("settings not saved: " + err.Error())
		}
		return err
	}
	return nil
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
	st.HandleSpec("map.basemap", state.Spec{
		What: "choose which map is drawn under the simulation and remember the " +
			"choice, or read the one in force",
		Params: []state.Param{
			{Name: "id", Type: state.ParamString, Primary: true,
				What: "the basemap: carto-dark, carto-light or esri-topo. An " +
					"absent or empty id reads the current choice and changes " +
					"nothing; the session does not draw the map, so an id it " +
					"does not recognise is stored rather than refused"},
		},
		Returns: []string{"id"},
		Answers: "Empty means nobody has chosen and the map's own default " +
			"stands. The choice is written to the settings file, so it survives " +
			"a restart; where that write fails the session still uses it and " +
			"says so, rather than promising a next launch it cannot keep.",
		Example: &state.Example{
			Params:   map[string]any{"id": "esri-topo"},
			What:     "put topography under the nodes for a siting review",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		if id, ok := stringField(p, "id"); ok && id != "" {
			s.prefs.Basemap = id
			// The promise is only made where it can be kept: savePrefs has
			// already said why not, so this says what is still true.
			if err := s.savePrefs(w); err == nil {
				w.Say("the basemap is " + id + ", here and on the next launch")
			} else {
				w.Say("the basemap is " + id + " for this session")
			}
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
	return filepath.Join(cache, "meshbench", "terrain")
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
	probe := filepath.Join(abs, ".meshbench-write-test")
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

// applyMemoryCeiling puts a soft limit on the Go heap: the tile budget plus
// fixed headroom for everything else the application is.
//
// Without one, the collector's doubling headroom sat on top of the tile
// cache's own budget: a 10 GB cache preference became 20 GB of process on a
// 31 GB desktop, and the operator watched their machine run out of memory
// mid-warm - twice. The ceiling makes the garbage collector actually collect
// what the tile cache evicts, instead of banking it against a heap target
// nobody chose.
//
// GOMEMLIMIT in the environment wins untouched: the runtime already honours
// it, and an operator who set it has said something this heuristic must not
// talk over.
func (s *Sim) applyMemoryCeiling() {
	if os.Getenv("GOMEMLIMIT") != "" {
		return
	}
	tiles := s.tileCacheTiles
	if tiles <= 0 {
		tiles = terrain.DefaultMaxLoadedTiles
	}
	// A decoded tile is 256x256 float32; the headroom carries the engine,
	// the interface, the decode scratch and the basemap.
	const tileBytes = 256 * 256 * 4
	const headroom = int64(3) << 30
	debug.SetMemoryLimit(int64(tiles)*tileBytes + headroom)
}
