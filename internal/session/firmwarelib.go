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

	"github.com/A13xB0/meshcoresim/internal/firmware"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
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
			err := downloadBuild(context.Background(), role, version, board)
			done := "downloaded " + role + " " + version
			if err != nil {
				done = "download failed: " + err.Error()
			}
			_, _ = st.Do(context.Background(), "job.progress", state.Job{
				ID: id, What: done, Done: 1, Total: 1, Finished: true})
			_, _ = st.Do(context.Background(), "firmware.installed", nil)
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
			if role != "" && string(s.nodes[i].Firmware.Role) != role {
				continue
			}
			s.nodes[i].Firmware.Version = version
			n++
		}
		for i := range w.Nodes {
			if node == "" || w.Nodes[i].Name == node {
				w.Nodes[i].Firmware = version
			}
		}
		w.Say(fmt.Sprintf("%d nodes pinned to %s", n, version))
		return map[string]any{"version": version, "nodes": n}, nil
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

// downloadBuild fetches one build into the cache.
func downloadBuild(ctx context.Context, role, version, board string) error {
	cache := firmware.DefaultCacheDir()
	if board != "" {
		bc := &firmware.BoardCatalogue{CacheDir: cache}
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

var _ = sort.Strings
