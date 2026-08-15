// The firmware library: what is on this machine, and what can be.
//
// The old workbench had a window for this - filters, downloads with progress,
// import, delete, use-for-role, wipe - and the Gio build had none of it. What
// is actually in the cache is the only thing that decides what a node can run,
// and a build that failed to download looks identical to one in daily use from
// outside that directory.
package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

func registerFirmwareLibrary(st *state.Store, s *Sim) {
	// firmware.installed: the cache, as it is.
	st.Handle("firmware.installed", func(w *state.World, _ any) (any, error) {
		cache := firmware.DefaultCacheDir()
		in := firmware.ListInstalled(cache)
		out := make([]map[string]any, 0, len(in))
		builds := make([]state.Build, 0, len(in))
		for _, b := range in {
			out = append(out, map[string]any{
				"version": b.Version, "role": b.Role, "board": b.Board,
				"native": b.Native, "bytes": b.Bytes, "path": b.Path,
			})
			builds = append(builds, state.Build{
				Version: b.Version, Role: b.Role, Board: b.Board,
				Native: b.Native, Bytes: b.Bytes, Path: b.Path,
			})
		}
		w.Builds = builds
		return map[string]any{"cache": cache, "installed": out}, nil
	})

	// firmware.library: every build there is, and what runs it.
	//
	// Published and on disk in one list rather than two, because a build
	// imported from a branch is in no catalogue and is exactly the kind of
	// thing worth testing, while a published one nobody has fetched still has
	// to be offerable. The old workbench answers this question with a table
	// and the Gio build answered it with a form asking for a version somebody
	// was expected to already know.
	st.Handle("firmware.library", func(w *state.World, _ any) (any, error) {
		// Disk first, immediately; the network once, afterwards. The library
		// read only the catalogue's cache, and everything in the cache is by
		// definition already downloaded - so the one thing a library is for,
		// showing what could be fetched, never appeared on it.
		if s.publishedNet == nil && !s.fetchingPublished {
			s.fetchingPublished = true
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				list := []publishedBuild{}
				// The native builds: MeshCore compiled for this machine, which
				// is how most nodes run.
				cat := &firmware.NativeCatalogue{CacheDir: firmware.DefaultCacheDir()}
				if images, err := cat.List(ctx); err == nil {
					for _, img := range images {
						if img.ForThisMachine() {
							list = append(list, publishedBuild{
								role: img.Role, version: img.Version,
							})
						}
					}
				}
				// And the published board images - the ones the flasher
				// serves, which emulated boards run. workbench1 listed these
				// and the Gio library never did, so the whole emulated half
				// of the library was missing: on Linux the native builds hid
				// it, and on a Mac, where no native build exists yet, the
				// panel was simply empty.
				list = append(list, publishedBoards(ctx)...)
				// An empty non-nil list on failure, so a dead network is one
				// failed fetch rather than a fetch per frame.
				_, _ = st.Do(context.Background(), "firmware.published", list)
			}()
		}
		s.fillLibrary(w)
		return map[string]any{"builds": len(w.Library)}, nil
	})

	// firmware.published: what the catalogue offers, landed from the fetch.
	st.Handle("firmware.published", func(w *state.World, p any) (any, error) {
		list, _ := p.([]publishedBuild)
		s.publishedNet = list
		s.fetchingPublished = false
		// Rebuild the rows here, or the fetch lands in a field nobody reads
		// again: the panel asks for the library once, the network answers a
		// few seconds later, and without this the answer sits unused until
		// somebody presses refresh. Which is what "the published builds never
		// appear" looked like.
		s.fillLibrary(w)
		return map[string]any{"published": len(list), "builds": len(w.Library)}, nil
	})

	// firmware.download: fetch a published build now rather than on first
	// use, which is what somebody about to work offline actually wants.
	st.Handle("firmware.download", func(w *state.World, p any) (any, error) {
		role, _ := stringField(p, "role")
		version, _ := stringField(p, "version")
		board, _ := stringField(p, "board")
		if role == "" || version == "" {
			return nil, fmt.Errorf("firmware.download needs a role and a version")
		}
		id := "fw-" + version + "-" + role
		w.Jobs = append(w.Jobs, state.Job{
			ID: id, What: "downloading " + role + " " + version, Total: 1})
		go func() {
			ctx := context.Background()
			what := "downloading " + role + " " + version
			// Bytes rather than one step: the job used to sit at 0 of 1 until
			// the file landed, which on a slow link is what a stall looks
			// like. Reported per whole percent by the catalogue.
			err := downloadBuildProgress(ctx, role, version, board,
				func(got, total int64) {
					_, _ = st.Do(ctx, "job.progress", state.Job{
						ID: id, What: what,
						Done: int(got / 1024), Total: int(total / 1024),
					})
				})
			done := "downloaded " + role + " " + version
			if err != nil {
				done = "download failed: " + err.Error()
			}
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: id, What: done, Done: 1, Total: 1, Finished: true})
			// Both lists: what is installed, and the library the panel draws.
			// Only the first was refreshed, so a finished download left the
			// row saying "not downloaded" until something else happened to
			// rebuild it - which is why downloading looked like it did
			// nothing at all.
			_, _ = st.Do(ctx, "firmware.installed", nil)
			_, _ = st.Do(ctx, "firmware.library", nil)
		}()
		return map[string]any{"downloading": true, "role": role, "version": version}, nil
	})

	// firmware.import: somebody's own build, which is how a change is tested
	// before it is a release.
	st.Handle("firmware.import", func(w *state.World, p any) (any, error) {
		path, _ := stringField(p, "path")
		role, _ := stringField(p, "role")
		board, _ := stringField(p, "board")
		if path == "" || role == "" {
			return nil, fmt.Errorf("firmware.import needs a path and a role")
		}
		cat := &firmware.Catalogue{CacheDir: firmware.DefaultCacheDir()}
		img, err := cat.Import(path, board, role)
		if err != nil {
			return nil, err
		}
		w.Say("imported " + img.Version + " as " + role)
		return map[string]any{"version": img.Version, "role": role}, nil
	})

	// firmware.delete: reclaim the disk, and prove a download works by
	// removing what it produced.
	st.Handle("firmware.delete", func(w *state.World, p any) (any, error) {
		path, _ := stringField(p, "path")
		if path == "" {
			return nil, fmt.Errorf("firmware.delete needs a path")
		}
		cache := firmware.DefaultCacheDir()
		clean := filepath.Clean(path)
		// Refuse anything outside the cache. A verb that deletes a path it was
		// handed is a verb that deletes whatever a mistake hands it.
		if rel, err := filepath.Rel(cache, clean); err != nil ||
			rel == ".." || len(rel) > 2 && rel[:3] == "../" {
			return nil, fmt.Errorf("%s is not inside the firmware cache", path)
		}
		if err := os.RemoveAll(clean); err != nil {
			return nil, err
		}
		w.Say("deleted " + filepath.Base(clean))
		return map[string]any{"deleted": clean}, nil
	})

	// firmware.needed: which roles have nodes with no usable build, and what
	// is installed for each.
	//
	// "no firmware for 34 of 58 nodes" is a diagnosis, not a way out. A run
	// that cannot start should be able to ask what these nodes ought to run,
	// and by role rather than by node, because pinning fifty-eight of them one
	// at a time is not a question anybody answers.
	st.Handle("firmware.needed", func(w *state.World, _ any) (any, error) {
		installed := firmware.ListInstalled(firmware.DefaultCacheDir())
		have := map[string]bool{}
		for _, b := range installed {
			have[b.Role+"@"+b.Version] = true
			have[b.Version] = true
		}
		var order []string
		counts := map[string]int{}
		for _, n := range s.nodes {
			if !n.Kind.RunsFirmware() {
				continue
			}
			role := nodeRole(n)
			if v := n.Firmware.Version; v != "" && (have[role+"@"+v] || have[v]) {
				continue
			}
			if counts[role] == 0 {
				order = append(order, role)
			}
			counts[role]++
		}
		roles := make([]any, 0, len(order))
		for _, role := range order {
			seen := map[string]bool{}
			var choices []string
			for _, b := range installed {
				if b.Role == role && !seen[b.Version] {
					seen[b.Version] = true
					choices = append(choices, b.Version)
				}
			}
			sort.Strings(choices)
			roles = append(roles, map[string]any{
				"role": role, "nodes": counts[role], "choices": choices,
			})
		}
		return map[string]any{"roles": roles}, nil
	})

	// firmware.set: pin a build to a role, or to one node.
	st.Handle("firmware.set", func(w *state.World, p any) (any, error) {
		version, _ := stringField(p, "version")
		node, _ := stringField(p, "node")
		role, _ := stringField(p, "role")
		if version == "" {
			return nil, fmt.Errorf("firmware.set needs a version")
		}
		n := 0
		for i := range s.nodes {
			if node != "" && s.nodes[i].Name != node {
				continue
			}
			// The role a node runs under, not the role it has been pinned
			// to: a node with no build chosen yet has an empty one, and
			// those are exactly the nodes being asked about.
			if role != "" && nodeRole(s.nodes[i]) != role {
				continue
			}
			s.nodes[i].Firmware.Version = version
			// And the engine, which holds its own copy of every spec and is
			// the one that actually starts a process. Without this the
			// library, the row and the message all agree with each other and
			// the run asks for whatever the network was opened with.
			if s.eng != nil {
				s.eng.PinFirmware(s.nodes[i].Name, version)
			}
			n++
		}
		for i := range w.Nodes {
			if node == "" || w.Nodes[i].Name == node {
				w.Nodes[i].Firmware = version
			}
		}
		w.Say(fmt.Sprintf("%d nodes pinned to %s", n, version))
		return map[string]any{
			"version": version, "nodes": n, "considered": len(s.nodes),
		}, nil
	})

	// firmware.wipe: a node keeps its preferences between runs exactly as
	// hardware does, so a node that has run before loads its old settings and
	// never reaches a changed default. Both arms of a study then return
	// identical numbers and the change looks inert.
	st.Handle("firmware.wipe", func(w *state.World, _ any) (any, error) {
		root := nodeStorageRoot()
		if root == "" {
			return nil, fmt.Errorf("no node storage directory")
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"wiped": 0}, nil
			}
			return nil, err
		}
		n := 0
		for _, e := range entries {
			if err := os.RemoveAll(filepath.Join(root, e.Name())); err == nil {
				n++
			}
		}
		w.Say(fmt.Sprintf("wiped %d nodes' stored settings", n))
		return map[string]any{"wiped": n, "root": root}, nil
	})
}

func downloadBuildProgress(ctx context.Context, role, version, board string,
	onProgress func(done, total int64),
) error {
	cache := firmware.DefaultCacheDir()
	if board != "" {
		bc := &firmware.BoardCatalogue{CacheDir: cache, OnProgress: onProgress}
		imgs, err := bc.List(ctx, version)
		if err != nil {
			return err
		}
		for _, img := range imgs {
			if img.Role == role && img.Board == board {
				_, err := bc.Ensure(ctx, img)
				return err
			}
		}
		return fmt.Errorf("no %s build of %s for %s", role, version, board)
	}
	nc := &firmware.NativeCatalogue{CacheDir: cache}
	_, err := nc.Ensure(ctx, role, version)
	return err
}

// nodeStorageRoot is where nodes keep what they remember between runs.
func nodeStorageRoot() string {
	if v := os.Getenv("MESHCORESIM_NODEFS"); v != "" {
		return v
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "meshcoresim", "nodefs")
}

func stringField(p any, name string) (string, bool) {
	switch v := p.(type) {
	case map[string]any:
		s, ok := v[name].(string)
		return s, ok
	case string:
		return v, true
	}
	return "", false
}

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
	var out []string
	for _, n := range s.nodes {
		if !n.Kind.RunsFirmware() {
			continue
		}
		if n.Firmware.Version == "" {
			out = append(out, n.Name+" (no version pinned)")
			continue
		}
		role := string(n.Firmware.Role)
		if role == "" {
			role = string(n.Kind.Application())
		}
		if have[role+"@"+n.Firmware.Version] || have[n.Firmware.Version] {
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

// soleString reads a verb's one parameter, which arrives either as a bare
// value or as the single-key object the old socket's callers write.
//
// Read here rather than unwrapped at the socket, because the socket cannot
// know which parameter a single key was meant to be.
func soleString(p any) string {
	switch v := p.(type) {
	case string:
		return v
	case map[string]any:
		if len(v) == 1 {
			for _, only := range v {
				if s, ok := only.(string); ok {
					return s
				}
			}
		}
	}
	return ""
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
		r.OnDisk, r.Bytes = true, in.Bytes
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
	cat := &firmware.BoardCatalogue{CacheDir: firmware.DefaultCacheDir()}
	imgs, err := cat.ListAll(ctx)
	if err != nil {
		return nil
	}
	known := map[string]bool{}
	for _, b := range scenario.Boards() {
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
