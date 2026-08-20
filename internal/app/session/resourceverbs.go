// Everything downloaded at runtime, as verbs.
//
// The panel is the obvious consumer, but the verbs come first and stand alone:
// the control socket is how a script or a headless machine reaches any of
// this, and the SoftDevice in particular unblocks emulated nRF52 boards for
// somebody who never opens the interface at all.
package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// resourceCacheDir is the root every provider keeps its bytes under.
func resourceCacheDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "meshcoresim")
}

// softDeviceProvider is the SoftDevice source, told how many nodes here are
// waiting on one so a missing row reads as needed rather than optional.
func (s *Sim) softDeviceProvider() *resource.SoftDevice {
	needed := 0
	for _, n := range s.nodes {
		if n.Firmware.Board == "" {
			continue
		}
		// An emulated nRF52 boots MBR, then the SoftDevice, then MeshCore.
		// Which boards those are is the catalogue's own MCU field rather than
		// a list of board names repeated here and left to drift.
		if b, err := scenario.BoardByName(n.Firmware.Board); err == nil &&
			strings.HasPrefix(b.MCU, "nRF52") {
			needed++
		}
	}
	return &resource.SoftDevice{CacheDir: resourceCacheDir(), Needed: needed}
}

func registerResources(st *state.Store, s *Sim) {
	// resource.list: what is on this machine. Never touches the network -
	// opening a panel must not start a download.
	st.Handle("resource.list", func(w *state.World, _ any) (any, error) {
		rows, err := s.softDeviceProvider().List(context.Background())
		if err != nil {
			return nil, err
		}
		out := make([]state.ResourceRow, 0, len(rows))
		for _, r := range rows {
			out = append(out, state.ResourceRow{
				Kind: string(r.Kind), Name: r.Name, Version: r.Version,
				Bytes: r.Bytes, Estimated: r.Estimated, State: string(r.State),
				Why: r.Why, Auto: r.Auto, Path: r.Path,
			})
		}
		w.Resources = out
		return map[string]any{"rows": len(out)}, nil
	})

	// resource.fetch: get one, as a job that can be stopped.
	st.Handle("resource.fetch", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		version, _ := stringField(p, "version")
		if name == "" {
			return nil, fmt.Errorf("resource.fetch needs a name")
		}
		if version == "" {
			version = "6.1.1"
		}
		id := "resource:" + name
		ctx, stop := context.WithCancel(context.Background())
		sd := s.softDeviceProvider()
		w.Jobs = append(w.Jobs, state.Job{
			ID: id, What: "fetching " + name + " " + version, Total: 1, Cancel: stop})
		go func() {
			defer stop()
			err := sd.Fetch(ctx, name, version, func(done, total int64) {
				if total <= 0 {
					return
				}
				_, _ = st.Do(context.Background(), "job.progress", state.Job{
					ID: id, What: "fetching " + name + " " + version,
					Done: int(done >> 10), Total: int(total >> 10)})
			})
			_, _ = st.Do(context.Background(), "job.done", id)
			if err != nil {
				if ctx.Err() != nil {
					_, _ = st.Do(context.Background(), "ui.said",
						"the "+name+" download was stopped")
					return
				}
				_, _ = st.Do(context.Background(), "ui.said", err.Error())
				return
			}
			_, _ = st.Do(context.Background(), "resource.fetched",
				map[string]any{"name": name, "version": version})
		}()
		return map[string]any{"fetching": name, "version": version}, nil
	})

	st.Handle("resource.fetched", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		version, _ := stringField(p, "version")
		// Nordic's own terms, cached beside the image and said aloud once:
		// a licensed binary arriving silently is the thing the licence
		// question was asked to avoid.
		w.Say(name + " " + version + " is cached, with its licence beside it")
		if _, err := st.Do(context.Background(), "resource.list", nil); err != nil {
			return nil, err
		}
		return map[string]any{"name": name}, nil
	})

	// resource.licence: the terms, for the interface to show.
	st.Handle("resource.licence", func(_ *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		version, _ := stringField(p, "version")
		if version == "" {
			version = "6.1.1"
		}
		text := s.softDeviceProvider().Licence(name, version)
		if text == "" {
			return nil, fmt.Errorf("no licence here: %s %s is not cached", name, version)
		}
		return map[string]any{"name": name, "version": version, "text": text}, nil
	})

	// resource.remove: delete one. The caller has already asked twice.
	st.Handle("resource.remove", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		version, _ := stringField(p, "version")
		if name == "" {
			return nil, fmt.Errorf("resource.remove needs a name")
		}
		if version == "" {
			version = "6.1.1"
		}
		err := s.softDeviceProvider().Remove(context.Background(),
			resource.Row{Name: name, Version: version})
		if err != nil {
			return nil, err
		}
		w.Say("removed " + name + " " + version)
		if _, err := st.Do(context.Background(), "resource.list", nil); err != nil {
			return nil, err
		}
		return map[string]any{"removed": name}, nil
	})
}
