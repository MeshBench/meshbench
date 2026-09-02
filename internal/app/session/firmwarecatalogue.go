// What builds exist: the ones on disk, the ones published for this machine,
// and which of them a node is asking for but has not got.
//
// Version comparison lives here and is numeric rather than lexicographic -
// v1.9.0 is older than v1.17.0, and a string sort says the opposite.
package session

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// buildsMissing is every node that would fail to start, by name.
//
// Checked before the first process is launched rather than discovered node by
// node afterwards: a half-started mesh measures a network that does not exist,
// and the operator sees a status line that never changes.
func (s *Sim) buildsMissing() []string {
	cache := firmware.DefaultCacheDir()
	have := map[string]bool{}
	for _, b := range firmware.ListInstalled(cache) {
		have[b.Role+"@"+b.Version] = true
		have[b.Version] = true
	}
	// And whatever an override supplies, because this gate has to ask the
	// question the engine asks. firmware.Resolve tries FindNative before it
	// looks in the cache, so MESHBENCH_NATIVE, or a build sitting beside the
	// simulator, satisfies a node whatever version it is pinned to: the
	// version is never consulted on that path.
	//
	// Reading the cache alone made this stricter than the thing it guards, and
	// it refused runs that would have started. A firmware developer pointed at
	// their own build was told to pin one in the Firmware panel, which is the
	// one thing that would not have helped, and the nightly - which downloads
	// its builds into a directory of its own and names it - failed on a
	// version nothing had asked it to have.
	//
	// One lookup per role rather than per node: it is a stat, and a national
	// network asks it three hundred times.
	overrides := map[string]bool{}
	overridden := func(role string) bool {
		if v, ok := overrides[role]; ok {
			return v
		}
		_, err := firmware.FindNative("", role)
		overrides[role] = err == nil
		return overrides[role]
	}
	var out []string
	for _, n := range s.nodes {
		if !n.Kind.RunsFirmware() {
			continue
		}
		role := string(n.Firmware.Role)
		if role == "" {
			role = string(n.Kind.Application())
		}
		// A node with nothing pinned is reported whatever else is on the
		// machine. An override would in fact start it - Resolve reaches
		// FindNative before it looks at the version - but "no build chosen"
		// is a gap in the scenario rather than a gap in the cache, and one
		// worth seeing before a run rather than inferring from the results.
		// Answering it from an override also made this gate depend on the
		// environment: the same fixture refused on one machine and played on
		// another, which is how it reached the nightly.
		if n.Firmware.Version == "" {
			out = append(out, n.Name+" (no version pinned)")
			continue
		}
		if have[role+"@"+n.Firmware.Version] || have[n.Firmware.Version] {
			continue
		}
		// A version was chosen and the cache has not got it. An override
		// supplies it anyway, because Resolve never consults the version on
		// that path. A board image is not a native build, so an override of
		// one says nothing about the other.
		if n.Firmware.Board == "" && overridden(role) {
			continue
		}
		out = append(out, fmt.Sprintf("%s (%s %s)", n.Name, role, n.Firmware.Version))
	}
	// Naming forty nodes helps nobody; naming three and counting the rest
	// does.
	if len(out) > 4 {
		return append(out[:4], fmt.Sprintf("and %d more", len(out)-4))
	}
	return out
}

// firmwareStartBlocker names why real firmware cannot start yet, or nil if it
// can - shared by every caller that would otherwise launch half a mesh and
// only find out node by node afterwards. sim.start's own play-button guard
// and a one-shot script both call this rather than each formatting the same
// refusal its own way.
func (s *Sim) firmwareStartBlocker() error {
	missing := s.buildsMissing()
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"no firmware for %d of %d nodes, so this run would be half a mesh: %s. "+
			"Pin one in the Firmware panel, or download it there",
		len(missing), len(s.nodes), strings.Join(missing, ", "))
}

// nodeRole is what a node runs: its pinned role, or the one its kind implies.
func nodeRole(n scenario.Node) string {
	if r := string(n.Firmware.Role); r != "" {
		return r
	}
	return string(n.Kind.Application())
}

// fillLibrary puts every build there is into the world: what is on disk, what
// is published, and what the scenario is running.
func (s *Sim) fillLibrary(w *state.World) {
	rows := map[string]*state.FirmwareRow{}
	key := func(role, version, board string) string {
		return role + "\x00" + version + "\x00" + board
	}
	add := func(role, version, board string) *state.FirmwareRow {
		k := key(role, version, board)
		if r, ok := rows[k]; ok {
			return r
		}
		r := &state.FirmwareRow{Role: role, Version: version, Board: board}
		rows[k] = r
		return r
	}
	// What is on disk: the only thing that decides what a node can run
	// today, and what a delete has to act on.
	for _, in := range firmware.ListInstalled(firmware.DefaultCacheDir()) {
		r := add(in.Role, in.Version, in.Board)
		r.OnDisk, r.Bytes, r.Path, r.Native = true, in.Bytes, in.Path, in.Native
		if st, err := os.Stat(in.Path); err == nil {
			r.Modified = st.ModTime()
		}
		r.Settings = firmware.LoadBuildSettings(in.Path)
		// Only a board image is worth reading the front of: a native build is
		// an executable for this machine and none of what that reads applies.
		// Read on a rebuild rather than per frame, and only the first 36 KB of
		// each - a library of thirty sixteen-megabyte images read whole would
		// be half a gigabyte to answer one line of a window.
		if !in.Native {
			r.Facts = emulated.InspectImage(in.Path)
		}
	}
	// What is published for this machine, from the cache rather than the
	// network: a library that can only be read online is no use to
	// somebody about to work without it.
	published := map[string]bool{}
	for _, img := range s.publishedBuilds() {
		add(img.role, img.version, img.board)
		published[key(img.role, img.version, img.board)] = true
	}
	// What the scenario is running, so a row can say what deleting it
	// would break.
	for _, n := range s.nodes {
		if !n.Kind.RunsFirmware() || n.Firmware.Version == "" {
			continue
		}
		role := nodeRole(n)
		if r, ok := rows[key(role, n.Firmware.Version, n.Firmware.Board)]; ok {
			r.InUse++
			continue
		}
		r := add(role, n.Firmware.Version, n.Firmware.Board)
		r.InUse++
	}
	out := make([]state.FirmwareRow, 0, len(rows))
	for _, r := range rows {
		// Nothing on disk and nothing published for this machine, yet nodes
		// point at it: the scenario is asking for a build nobody here can
		// run, and the row should say so rather than wait to be discovered
		// at start time.
		r.Unavailable = !r.OnDisk && !published[key(r.Role, r.Version, r.Board)]
		out = append(out, *r)
	}
	// In use first, then what is here, then the rest: the order somebody
	// is closest to.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.InUse > 0) != (b.InUse > 0) {
			return a.InUse > 0
		}
		if a.OnDisk != b.OnDisk {
			return a.OnDisk
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return a.Board < b.Board
	})
	w.Library = out
}

// publishedBoards is the published board images, filtered down to something a
// person can read.
//
// The catalogue is about eight thousand images - every version of every board
// and role MeshCore has ever published. All of them on a panel is not a
// library, so this keeps the boards MeshBench can actually run and, for each
// board and role, the newest version. Older ones stay one click away in the
// firmware picker, which reads the catalogue directly.
func publishedBoards(ctx context.Context) []publishedBuild {
	cat := &emulated.BoardCatalogue{CacheDir: firmware.DefaultCacheDir()}
	imgs, err := cat.ListAll(ctx)
	if err != nil {
		return nil
	}
	known := map[string]bool{}
	for _, b := range hw.Boards() {
		known[strings.ToLower(b.Name)] = true
	}
	// Newest version per board and role. Versions are vX.Y.Z, so comparing
	// them as numbers rather than strings keeps v1.9.0 below v1.17.0.
	best := map[string]publishedBuild{}
	for _, img := range imgs {
		if !known[strings.ToLower(img.Board)] {
			continue
		}
		k := img.Board + "\x00" + img.Role
		if cur, ok := best[k]; ok && !newerVersion(img.Version, cur.version) {
			continue
		}
		best[k] = publishedBuild{role: img.Role, version: img.Version, board: img.Board}
	}
	out := make([]publishedBuild, 0, len(best))
	for _, b := range best {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].board != out[j].board {
			return out[i].board < out[j].board
		}
		return out[i].role < out[j].role
	})
	return out
}

// newerVersion compares vX.Y.Z by number, so v1.9.0 sorts below v1.17.0 -
// which a string comparison gets backwards, and which is how asking for a
// board with no version came back with v1.14.1 while v1.17.1 existed.
func newerVersion(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return len(pa) > len(pb)
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out []int
	for _, f := range strings.Split(v, ".") {
		n := 0
		for _, r := range f {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}

// publishedBuild is one build the catalogue offers, in the fields the library
// needs.
type publishedBuild struct{ role, version, board string }

// publishedBuilds is what can be downloaded, read from the catalogue's cache.
//
// From the cache and never the network: this is called to draw a panel, and a
// panel that waits on a fetch is a panel that hangs.
func (s *Sim) publishedBuilds() []publishedBuild {
	var out []publishedBuild
	cache := firmware.DefaultCacheDir()
	cat := &firmware.NativeCatalogue{CacheDir: cache}
	for _, img := range cat.CachedImages() {
		if !img.ForThisMachine() {
			continue
		}
		out = append(out, publishedBuild{role: img.Role, version: img.Version})
	}
	out = append(out, s.publishedNet...)
	return out
}
