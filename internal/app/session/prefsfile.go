// The settings file, read and written whole.
//
// Written whole because os.WriteFile truncates before it writes: a machine that
// lost power, or a session killed, between those two moments left a file that
// was neither the old settings nor the new ones, and the next launch read it as
// a machine nobody had ever configured. The pattern is the one the rendezvous
// file and the matrix cache already use - a temporary file beside the target,
// then a rename, which is atomic on every filesystem this runs on.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// readPrefs reads the settings, or nil where there is no file yet.
//
// Nil rather than a zero Prefs, because "nobody has chosen anything" and "every
// choice is off" are different answers and only one of them leaves the
// defaults alone.
func readPrefs(path string) (*Prefs, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read the settings at %s: %w", path, err)
	}
	var p Prefs
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("the settings at %s are not readable (%d bytes): %w",
			path, len(b), err)
	}
	return &p, nil
}

// writePrefs writes the settings and renames over the target, so a reader sees
// the old file or the new one and never half of either.
func writePrefs(path string, p Prefs) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	// In the same directory as the target, because a rename across filesystems
	// is not a rename and would fall back to a copy - which is the tear this
	// exists to avoid.
	f, err := os.CreateTemp(dir, ".workbench2-*")
	if err != nil {
		return fmt.Errorf("cannot write in %s: %w", dir, err)
	}
	tmp := f.Name()
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot write the settings: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot write the settings: %w", err)
	}
	// Left at CreateTemp's 0600. The file this replaces was written 0644, which
	// it never needed to be: these are one person's own choices, in their own
	// configuration directory, and nothing else reads them.
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cannot replace %s: %w", path, err)
	}
	return nil
}
