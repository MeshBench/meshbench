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

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
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
		return map[string]any{
			"builds": libraryRows(w.Library), "count": len(w.Library),
		}, nil
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
		version, _ := namedField(p, "version")
		board, _ := namedField(p, "board")
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
				ID: id, What: done, Done: 1, Total: 1,
				Finished: true, Failed: err != nil})
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
		role, _ := namedField(p, "role")
		board, _ := namedField(p, "board")
		if path == "" || role == "" {
			return nil, badParams("firmware.import needs a path and a role")
		}
		// The label is what the library will know it by, and what a node pins.
		// Left out it is a timestamp rather than the constant it used to be:
		// every import called itself "imported", so a second one replaced the
		// first in place and nothing could say which of two local builds was
		// running.
		label, _ := namedField(p, "label")
		if label == "" {
			// The MCP tool and one command-line path both called it "version"
			// and the handler read neither, so every import through them was
			// stamped with a timestamp - and cmd_dev then pinned nodes to a
			// version that had never been created. Accepted here as well as
			// fixed there, because scripts written against the old name are
			// already out in the world.
			label, _ = namedField(p, "version")
		}
		if err := refuseHalfAnImage(path, board); err != nil {
			return nil, err
		}
		cat := &firmware.Catalogue{CacheDir: firmware.DefaultCacheDir()}
		img, err := cat.Import(path, board, role, firmware.ImportLabel(label))
		if err != nil {
			return nil, err
		}
		s.fillLibrary(w)
		w.Say("imported " + img.Version + " as " + role)
		return map[string]any{
			"version": img.Version, "role": role,
			"board": img.Board, "path": img.URL, "bytes": img.Size,
		}, nil
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
		deleteBuildSettings(clean)
		// The library the panels draw, not only the directory: a delete that
		// left the row behind read as a delete that did nothing, and the
		// only caller that refreshed was the one panel that remembered to.
		s.fillLibrary(w)
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
		node, _ := namedField(p, "node")
		role, _ := namedField(p, "role")
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
		// And the cards kept somewhere else. A node handed a card of its own
		// choosing has its storage outside this directory, so wiping the
		// directory leaves it holding everything it was supposed to forget -
		// which is the one case where "wiped every node" would be a lie.
		cards := s.wipeCardsOutside(root)
		w.Say(fmt.Sprintf("wiped %d nodes' stored settings and %d cards kept elsewhere",
			n, cards))
		return map[string]any{"wiped": n, "root": root, "cards": cards}, nil
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

// stringField reads a verb's PRIMARY field: the one, and only one, that a bare
// string parameter is allowed to mean.
//
// Use it once per verb. Every other field goes through namedField, because a
// bare string satisfies this function whatever it is asked for - ask it for two
// fields and both come back holding the same value. That is how node.window
// came to be unopenable: the window is asked for by name, the name arrived as a
// bare string, and the optional tab was read with this, so every double-click
// asked for a tab named after the node and was refused.
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

// namedField reads a field that has to be named to be meant.
//
// No bare-string fallback: a caller who wrote one has already spent it on the
// verb's primary field, and handing the same value to a second field invents a
// parameter they did not pass.
func namedField(p any, name string) (string, bool) {
	m, ok := p.(map[string]any)
	if !ok {
		return "", false
	}
	s, ok := m[name].(string)
	return s, ok
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

// refuseHalfAnImage turns away an application-only build before it is stored.
//
// A published release for an ESP32 board carries two files whose names differ
// by one word, and only one of them boots: firmware-heltec-v3-2.7.26.bin is the
// application, and firmware-heltec-v3-2.7.26.factory.bin is the whole flash -
// bootloader, partition table and application together. A board starts from the
// bootloader, so the first of those starts nothing.
//
// Checked here rather than at play, where it was checked before. The refusal
// arrived several minutes after the import, phrased as a flash-image error, to
// somebody who thought they were starting a board - and the library went on
// offering the build that could not run. Checked here it is an answer to the
// question being asked, at the moment it is asked.
//
// Only for a board with an ESP32-family MCU: the header this reads is Espressif's,
// and an nRF52 image is a different shape entirely.
// isESP32Board reports whether this board's MCU is one whose flash images the
// check below can read. A board nobody has heard of is not one to judge.
func isESP32Board(board string) bool {
	b, err := scenario.BoardByName(board)
	if err != nil {
		return false
	}
	return strings.HasPrefix(strings.ToUpper(b.MCU), "ESP32")
}

func refuseHalfAnImage(path, board string) error {
	if board == "" || strings.ToLower(filepath.Ext(path)) != ".bin" {
		return nil
	}
	if !isESP32Board(board) {
		return nil
	}
	// The path is what somebody chose in a file dialog, and reading it is the
	// whole of what this function is for.
	data, err := os.ReadFile(path) //nolint:gosec // the caller's own chosen import
	if err != nil {
		return err
	}
	if _, err := firmware.ClassifyESPImage(data); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	// The header is not the test. An application image begins with the same
	// magic byte - it is an ESP image too, just one that belongs at 0x10000
	// rather than at the start of the chip. What a whole flash image has and
	// an application has not is the partition table the ROM bootloader reads.
	if !firmware.HasPartitionTable(data) {
		return fmt.Errorf(
			"%s has no partition table at 0x8000, so it is the application on its "+
				"own rather than a whole flash image: it starts at 0x10000 and a board "+
				"starts from the bootloader. The release this came from publishes the "+
				"whole flash beside it, named -merged.bin or .factory.bin",
			filepath.Base(path))
	}
	return nil
}
