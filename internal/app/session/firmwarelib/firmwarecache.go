// What goes into the firmware cache, and what comes out of it.
//
// Beside the library that lists builds rather than inside it: listing is a
// question and these three are changes to the disk, with a refusal each that
// only matters here - a download that lands nowhere the library reads, an
// import of half an image, a delete handed a path outside the cache.
package firmwarelib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

func registerFirmwareCache(st *state.Store, s *session.Sim) {
	st.Handle("firmware.download", func(w *state.World, p any) (any, error) {
		role, _ := session.StringField(p, "role")
		version, _ := session.NamedField(p, "version")
		board, _ := session.NamedField(p, "board")
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

	st.Handle("firmware.import", func(w *state.World, p any) (any, error) {
		path, _ := session.StringField(p, "path")
		role, _ := session.NamedField(p, "role")
		board, _ := session.NamedField(p, "board")
		if path == "" || role == "" {
			return nil, session.BadParams("firmware.import needs a path and a role")
		}
		// The label is what the library will know it by, and what a node pins.
		// Left out it is a timestamp rather than the constant it used to be:
		// every import called itself "imported", so a second one replaced the
		// first in place and nothing could say which of two local builds was
		// running.
		label, _ := session.NamedField(p, "label")
		if label == "" {
			// A command-line path called it "version"
			// and the handler read neither, so every import through them was
			// stamped with a timestamp - and cmd_dev then pinned nodes to a
			// version that had never been created. Accepted here as well as
			// fixed there, because scripts written against the old name are
			// already out in the world.
			label, _ = session.NamedField(p, "version")
		}
		if err := session.RefuseHalfAnImage(path, board); err != nil {
			return nil, err
		}
		cat := &firmware.Catalogue{CacheDir: firmware.DefaultCacheDir()}
		img, err := cat.Import(path, board, role, firmware.ImportLabel(label))
		if err != nil {
			return nil, err
		}
		fillLibrary(s, w)
		w.Say("imported " + img.Version + " as " + role)
		return map[string]any{
			"version": img.Version, "role": role,
			"board": img.Board, "path": img.URL, "bytes": img.Size,
		}, nil
	})

	st.Handle("firmware.delete", func(w *state.World, p any) (any, error) {
		path, _ := session.StringField(p, "path")
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
		fillLibrary(s, w)
		w.Say("deleted " + filepath.Base(clean))
		return map[string]any{"deleted": clean}, nil
	})
}

func downloadBuildProgress(ctx context.Context, role, version, board string,
	onProgress func(done, total int64),
) error {
	cache := firmware.DefaultCacheDir()
	if board != "" {
		bc := &emulated.BoardCatalogue{CacheDir: cache, OnProgress: onProgress}
		// Every release rather than the one whose tag is this version,
		// because there is no such tag. The catalogue derives a build's
		// version from its asset name - v1.17.1 - while MeshCore tags its
		// releases by role, repeater-v1.17.1, so asking for
		// releases/tags/v1.17.1 answered 404 for every board image ever
		// offered and no emulated board could be fetched at all. ListAll
		// is what filled the library, so this finds the row the same way
		// it was listed.
		imgs, err := bc.ListAll(ctx)
		if err != nil {
			return err
		}
		for _, img := range imgs {
			if img.Role == role && img.Board == board && img.Version == version {
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
