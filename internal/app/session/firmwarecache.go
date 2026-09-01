// What goes into the firmware cache, and what comes out of it.
//
// Beside the library that lists builds rather than inside it: listing is a
// question and these three are changes to the disk, with a refusal each that
// only matters here - a download that lands nowhere the library reads, an
// import of half an image, a delete handed a path outside the cache.
package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

func registerFirmwareCache(st *state.Store, s *Sim) {
	st.HandleSpec("firmware.download", state.Spec{
		What: "fetch a published build now rather than at the moment a node " +
			"first needs it, which is what somebody about to work offline wants",
		Params: []state.Param{
			{Name: "role", Type: state.ParamString, Required: true, Primary: true,
				What: "the role to fetch, such as `simple_repeater`; refused when absent"},
			{Name: "version", Type: state.ParamString, Required: true,
				What: "the published release tag; refused when absent"},
			{Name: "board", Type: state.ParamString,
				What: "the board image to fetch; absent means the native build " +
					"for this machine"},
		},
		Returns: []string{"downloading", "role", "version"},
		Answers: "It answers as soon as the fetch has been started, not when the " +
			"file lands. Progress arrives on a job called `fw-<version>-<role>`, " +
			"counted in kilobytes, and a failure is reported there rather than " +
			"here; the installed list and the library are re-read either way.",
		Example: &state.Example{
			Params: map[string]any{
				"role": "simple_repeater", "version": "repeater-v1.16.0",
			},
			What: "fetch a repeater build before working without a network",
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("firmware.import", state.Spec{
		What: "put somebody's own build into the library, which is how a change " +
			"is tested against a release before it is one",
		Params: []state.Param{
			{Name: "path", Type: state.ParamString, Required: true, Primary: true,
				What: "the file to import; refused when absent, and an ESP32 " +
					"board's application-only .bin is refused too, because a " +
					"board starts from the whole flash image"},
			{Name: "role", Type: state.ParamString, Required: true,
				What: "the role it is imported as; refused when absent"},
			{Name: "board", Type: state.ParamString,
				What: "the board it is for; absent means a build for this machine"},
			{Name: "label", Type: state.ParamString,
				What: "what the library will know it by and what a node pins; " +
					"absent, it is stamped with a timestamp so a second import " +
					"does not replace the first in place"},
			{Name: "version", Type: state.ParamString,
				What: "an older name for `label`, read only when `label` is " +
					"absent, because scripts written against it are already out " +
					"in the world"},
		},
		Returns: []string{"version", "role", "board", "path", "bytes"},
		Answers: "`version` is the label the build was stored under, which is " +
			"the timestamp when none was given and is what `firmware.set` then " +
			"has to be handed.",
		Example: &state.Example{
			Params: map[string]any{
				"path":  "/home/you/MeshCore/.pio/build/Heltec_v3/firmware.factory.bin",
				"role":  "simple_repeater",
				"board": "Heltec_v3",
				"label": "repeater-my-fix",
			},
			What: "test a local change against a published build",
		},
	}, func(w *state.World, p any) (any, error) {
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
			// A command-line path called it "version"
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

	st.HandleSpec("firmware.delete", state.Spec{
		What: "remove one build from the cache, to reclaim the disk or to prove " +
			"a download works by taking away what it produced",
		Params: []state.Param{
			{Name: "path", Type: state.ParamString, Required: true, Primary: true,
				What: "the build's path, as the library and `firmware.details` " +
					"give it; refused when absent, and refused when it points " +
					"anywhere but inside the firmware cache"},
		},
		Returns: []string{"deleted"},
		Answers: "The build's settings sidecar goes with it, so the next build " +
			"imported under the same name does not inherit somebody else's " +
			"answers. Nothing is said about the nodes pinned to it, which keep " +
			"the name and have nothing to run until they are pinned again.",
		Example: &state.Example{
			Params: map[string]any{
				"path": "/home/you/.cache/meshbench/firmware/native/repeater-v1.16.0",
			},
			What: "reclaim the disk a build was using",
		},
	}, func(w *state.World, p any) (any, error) {
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
}

func downloadBuildProgress(ctx context.Context, role, version, board string,
	onProgress func(done, total int64),
) error {
	cache := firmware.DefaultCacheDir()
	if board != "" {
		bc := &emulated.BoardCatalogue{CacheDir: cache, OnProgress: onProgress}
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
