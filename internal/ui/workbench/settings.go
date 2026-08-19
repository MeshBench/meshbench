// Settings, and the dialog that changes them.
//
// Applied live rather than at next launch. A theme somebody cannot see the
// effect of until they restart is a theme they set by trial and error, and
// density is the setting most likely to be wrong for a particular screen.
package workbench

import (
	"sync"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// settings is what the interface looks like, shared between windows.
//
// Guarded because the settings dialog may be its own window on its own
// goroutine, and the main frame loop reads these every frame.
type settings struct {
	mu      sync.Mutex
	mode    theme.Mode
	density theme.Density
	scale   float64
	gen     uint64
}

func newSettings(mode theme.Mode) *settings {
	return &settings{mode: mode, density: theme.Default, scale: 1, gen: 1}
}

// scale is the interface's own size. Kept beside the theme because changing it
// invalidates the same things a theme change does.
func (s *settings) getScale() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scale <= 0 {
		return 1
	}
	return s.scale
}

func (s *settings) setScale(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v > 0 && s.scale != v {
		s.scale, s.gen = v, s.gen+1
	}
}

func (s *settings) get() (theme.Mode, theme.Density, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode, s.density, s.gen
}

func (s *settings) setMode(m theme.Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mode != m {
		s.mode, s.gen = m, s.gen+1
	}
}

func (s *settings) setDensity(d theme.Density) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.density != d {
		s.density, s.gen = d, s.gen+1
	}
}
