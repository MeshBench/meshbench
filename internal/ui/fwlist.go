package ui

import (
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// The firmware library's list: what a row is, and how the list is ordered and
// filtered.
//
// Separate from the drawing because this half has no imgui in it and can be
// reasoned about on its own - which matters most for the ordering, where a
// comparator that is not a total order made the table reshuffle every frame.

// fwRow is one build, whether it is on this machine or merely published.
//
// There used to be two tables, one for each, and they shared four of their six
// columns: the same role, version, board and in-use-by, differing only in
// whether the last column offered a download or a delete. Two tables for one
// list made the window twice as tall as it needed to be and asked the reader to
// hold "is this row the same thing as that one" in their head.
type fwRow struct {
	Role    string
	Version string
	Board   string // empty means a host build
	Bytes   int64
	OnDisk  bool
	InUse   int

	// installed carries the path, so delete has something to act on; image
	// carries the download URL for a board build that is not here yet.
	installed firmware.Installed
	image     firmware.BoardImage
}

func (r fwRow) key() string { return r.Role + "\x00" + r.Version + "\x00" + r.Board }

// boardLabel is what the board column says. Native builds say so rather than
// leaving the cell blank, because a blank cell reads as unknown where "native"
// is the answer.
func (r fwRow) boardLabel() string {
	if r.Board == "" {
		return "native"
	}
	return r.Board
}

// fwRows merges what is published with what is on disk.
//
// A build can be in either, both, or only on disk: an imported branch build is
// in no catalogue and is exactly the kind of thing worth testing, so a list
// built from the catalogue alone would hide it.
func (a *App) fwRows() []fwRow {
	byKey := map[string]*fwRow{}
	add := func(role, version, board string) *fwRow {
		r := fwRow{Role: role, Version: version, Board: board}
		if got, ok := byKey[r.key()]; ok {
			return got
		}
		byKey[r.key()] = &r
		return &r
	}

	roles, _ := a.fw.forThisMachine()
	for _, role := range roles {
		for _, v := range a.fw.versionsFor(role) {
			add(role, v, "")
		}
	}
	// Published hardware images, filtered to boards that can actually be run.
	for _, img := range a.fw.boardImages() {
		r := add(img.Role, img.Version, img.Board)
		r.image = img
	}
	// What is on disk wins on size and gives delete something to act on.
	for _, in := range firmware.ListInstalled(firmware.DefaultCacheDir()) {
		r := add(in.Role, in.Version, in.Board)
		r.OnDisk, r.Bytes, r.installed = true, in.Bytes, in
	}

	// In use is per role and version, so a row about to be deleted can say
	// whether anything is relying on it.
	inUse := map[string]int{}
	for i := range a.Nodes {
		n := &a.Nodes[i]
		if !n.Kind.RunsFirmware() {
			continue
		}
		role := n.Firmware.Role
		if role == "" {
			role = n.Kind.Application()
		}
		version := n.Firmware.Version
		if version == "" {
			version = "main"
		}
		inUse[role+" "+version]++
	}

	out := make([]fwRow, 0, len(byKey))
	for _, r := range byKey {
		r.InUse = inUse[r.Role+" "+r.Version]
		out = append(out, *r)
	}
	return out
}

// fwSortDefault is the order before anyone clicks a header.
const fwSortDefault = -1

// fwSortRows orders the table by whichever column was clicked.
//
// imgui hands back the column and direction; the comparison per column lives
// here so the drawing loop stays a drawing loop. Ties fall back to role then
// version, so a sort on a column with many equal values still produces a stable
// reading order rather than shuffling on every frame.
// fwDrainDownloadMsg moves a finished download's message onto the status line.
//
// The download runs on its own goroutine and the status line belongs to the
// frame thread, so the message is parked under the mutex and collected here.
func (a *App) fwDrainDownloadMsg() {
	a.fwDlMu.Lock()
	msg := a.fwDoneMsg
	a.fwDoneMsg = ""
	a.fwDlMu.Unlock()
	if msg != "" {
		a.status = msg
	}
}

func fwSortRows(rows []fwRow, col int, ascending bool) {
	less := func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch col {
		case fwSortDefault:
			// Untouched, the useful order is what the operator is closest to:
			// what the scenario is running now, then what is already here and
			// needs no network, then the host builds that can actually run
			// today, and only then alphabetically. A catalogue sorted by name
			// opens on companion-v1.0.0a, which nobody wants.
			if a.InUse != b.InUse {
				return a.InUse > b.InUse
			}
			if a.OnDisk != b.OnDisk {
				return a.OnDisk
			}
			if (a.Board == "") != (b.Board == "") {
				return a.Board == ""
			}
		case 1:
			if a.Version != b.Version {
				return a.Version < b.Version
			}
		case 2:
			if a.boardLabel() != b.boardLabel() {
				return a.boardLabel() < b.boardLabel()
			}
		case 3:
			// On disk first, then by size: "what is taking up room" and "what
			// can run right now" are the two reasons to sort by this column.
			if a.OnDisk != b.OnDisk {
				return a.OnDisk
			}
			if a.Bytes != b.Bytes {
				return a.Bytes < b.Bytes
			}
		case 4:
			if a.InUse != b.InUse {
				return a.InUse < b.InUse
			}
		default:
			if a.Role != b.Role {
				return a.Role < b.Role
			}
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		if a.Version != b.Version {
			return a.Version > b.Version
		}
		// Board, then the asset name, so the comparator is a total order.
		// Without these two the rows that tie were left in whatever order they
		// came out of a map - and Go randomises that per iteration, so the
		// table reshuffled every frame and read as flashing. It only appeared
		// once more than one board published the same role and version.
		if a.Board != b.Board {
			return a.Board < b.Board
		}
		return a.key() < b.key()
	}
	if col == fwSortDefault {
		// The default is one order, not an ascending and a descending one.
		sort.SliceStable(rows, less)
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if ascending {
			return less(i, j)
		}
		return less(j, i)
	})
}

// fwMatches decides whether a row survives the search box and the toggles.
//
// The search is a plain substring over the three things a row is identified by,
// because that is how people look: they type "1.17", or "companion", or the
// board they have on the desk, and any of those should narrow the list.
func fwMatches(r fwRow, query string, onDiskOnly, boardsOnly, nativeOnly bool) bool {
	if onDiskOnly && !r.OnDisk {
		return false
	}
	if boardsOnly && r.Board == "" {
		return false
	}
	if nativeOnly && r.Board != "" {
		return false
	}
	if query == "" {
		return true
	}
	hay := strings.ToLower(r.Role + " " + r.Version + " " + r.boardLabel())
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(hay, term) {
			return false
		}
	}
	return true
}
