// Package pick opens the file dialog the platform already has.
//
// MeshBench asks for paths in five places - importing a build, opening a
// network, saving one, choosing the tile cache directory, choosing a fixture -
// and until now every one of them was a text field expecting a path to be
// typed. That is fine from a script and hostile in a window, which is what
// Alex hit: "if it wants a path anywhere in the application give it a browse
// button".
//
// Each platform's own dialog rather than one drawn here, so it looks and
// behaves like every other file dialog on that machine:
//
//	macOS    osascript, which is the Finder's own panel
//	Windows  IFileOpenDialog through COM
//	Linux    xdg-desktop-portal, then zenity, then kdialog
//
// The library behind it needs no cgo, which keeps the Windows cross-build and
// the macOS bundle exactly as they were.
//
// Everything here blocks until the person answers, so callers run it on their
// own goroutine and post the answer back. Nothing in this package touches the
// interface.
package pick

import (
	"github.com/ncruces/zenity"
)

// Kind is what the caller wants back.
type Kind int

const (
	// File is an existing file - a firmware binary, a saved network.
	File Kind = iota
	// SaveFile is a name that need not exist yet.
	SaveFile
	// Directory is a folder - the tile cache, somewhere to write runs.
	Directory
)

// Filter narrows what the dialog offers, in the platform's own idiom.
type Filter struct {
	Name       string   // "MeshBench networks"
	Extensions []string // {"json"}
}

// Open asks for a path and returns it, or "" if the person cancelled.
//
// A cancel is not an error: it is the commonest outcome of opening a dialog
// and callers should do nothing, not report a failure. A real failure - no
// portal, no zenity, no kdialog on a Linux box - comes back as an error so
// the caller can say why the button did nothing.
func Open(title, start string, kind Kind, filters ...Filter) (string, error) {
	opts := []zenity.Option{zenity.Title(title)}
	if start != "" {
		opts = append(opts, zenity.Filename(start))
	}
	for _, f := range filters {
		if len(f.Extensions) == 0 {
			continue
		}
		patterns := make([]string, 0, len(f.Extensions))
		for _, e := range f.Extensions {
			patterns = append(patterns, "*."+e)
		}
		opts = append(opts, zenity.FileFilter{Name: f.Name, Patterns: patterns})
	}

	var (
		path string
		err  error
	)
	switch kind {
	case Directory:
		path, err = zenity.SelectFile(append(opts, zenity.Directory())...)
	case SaveFile:
		path, err = zenity.SelectFileSave(append(opts, zenity.ConfirmOverwrite())...)
	default:
		path, err = zenity.SelectFile(opts...)
	}
	if err == zenity.ErrCanceled {
		return "", nil
	}
	return path, err
}
