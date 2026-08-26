package workbench

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// openSessionLog opens a fresh log file for this run and wires it into the
// store, so every status line - not just the last twenty the strip at the
// bottom shows - survives past the point somebody was watching for it.
//
// One file per launch rather than one appended-to file forever: a run that
// went quiet three sessions ago is not what "what just happened" means.
func openSessionLog(st *state.Store, sm *session.Sim) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "meshbench", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("session log directory: %w", err)
	}
	path := filepath.Join(dir, time.Now().Format("2006-01-02T15-04-05")+".log")
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("session log file: %w", err)
	}
	st.SetLogWriter(f)
	sm.SetLogPath(path)
	return path, nil
}
