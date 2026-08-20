// Caching results against the image and emulator versions.
//
// Measuring costs real emulator time - ten boards times eight capabilities
// is a lot of QEMU - so caching is not an optimisation here, it is what
// makes the feature usable. And a cache that never goes stale lies:
// changing the QEMU binary has to mark affected results stale rather than
// keep serving them as current.
package boardcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Dir is where reports live, keyed by board and version.
func Dir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "meshcoresim", "boardcapability")
	return dir, os.MkdirAll(dir, 0o755)
}

func reportPath(dir, board, version string) string {
	return filepath.Join(dir, board+"-"+version+".json")
}

// Save writes a report, keyed by board and version - a later probe of the
// same pair overwrites it, which is the point: the cache holds the most
// recent measurement, not a history of them.
// Pointer receiver to match set(): a type with both copies itself on every
// call to half its methods, and the half that mutates is the one you notice
// too late.
func (r *BoardReport) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(reportPath(dir, r.Board, r.Version), b, 0o644)
}

// Load reads a cached report, or an all-Untested one if there is none -
// "never run" and "not on disk" are the same state, so Load never errors on
// a board it simply has not seen yet. Stale is set here, against the
// emulator fingerprint in use right now, not against whatever produced the
// file.
func Load(board, version string) BoardReport {
	dir, err := Dir()
	if err != nil {
		return untestedReport(board, version)
	}
	b, err := os.ReadFile(reportPath(dir, board, version))
	if err != nil {
		return untestedReport(board, version)
	}
	var r BoardReport
	if json.Unmarshal(b, &r) != nil || r.Results == nil {
		return untestedReport(board, version)
	}
	// A report from a format this build did not write is treated as
	// untested rather than guessed at - the same rule plan 1's experiment
	// manifests follow.
	for _, c := range Capabilities {
		if _, ok := r.Results[c]; !ok {
			return untestedReport(board, version)
		}
	}
	r.Stale = r.EmulatorFP != EmulatorFingerprint()
	return r
}
