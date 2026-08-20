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

// resourceProviders is every source of rows, in the order the panel shows them.
//
// The SoftDevice first because it is the one thing here that a scenario can be
// blocked on, then the caches that fill themselves. Those are the reason this
// package exists: on the machine it was written for the terrain cache had
// reached 7.1 GB, and nothing in the application would tell you so.
func (s *Sim) resourceProviders() []resource.Provider {
	root := resourceCacheDir()
	dir := func(k resource.Kind, label, sub, purpose, terms string) resource.Provider {
		return &resource.DirCache{
			K: k, Label: label, Dir: filepath.Join(root, sub),
			Purpose: purpose, Terms: terms,
		}
	}
	return []resource.Provider{
		s.softDeviceProvider(),
		dir(resource.Terrain, "terrain tiles", "terrain",
			"height data under every link budget, fetched for the ground you look at",
			terrainTerms),
		dir(resource.Basemap, "basemap", "basemap",
			"the hillshaded map drawn under the simulation", basemapTerms),
		dir(resource.Buildings, "building footprints", "environment",
			"heights and materials that stand in the way of a signal", buildingTerms),
		dir(resource.Basemap, "map tiles", "tiles",
			"the map imagery itself, as the view has needed it", basemapTerms),
	}
}

// relistResources rescans every provider into the world, and is the only place
// that does. Called directly by whatever needs the list refreshed, because a store
// handler cannot ask the store for anything.
func (s *Sim) relistResources(w *state.World) (int, error) {
	var out []state.ResourceRow
	// One provider failing must not empty the page. A cache directory that
	// cannot be read is a row that says so, beside the ones that could.
	for _, p := range s.resourceProviders() {
		rows, err := p.List(context.Background())
		if err != nil {
			out = append(out, state.ResourceRow{
				Kind: string(p.Kind()), Name: string(p.Kind()),
				State: string(resource.Unavailable), Why: err.Error(),
			})
			continue
		}
		for _, r := range rows {
			out = append(out, state.ResourceRow{
				Kind: string(r.Kind), Name: r.Name, Version: r.Version,
				Bytes: r.Bytes, Estimated: r.Estimated, State: string(r.State),
				Why: r.Why, Auto: r.Auto, Path: r.Path,
				Fetchable: r.Fetchable, Licensed: r.Licensed,
			})
		}
	}
	w.Resources = out
	return len(out), nil
}

// providerFor finds the provider that owns a row, by the kind and name the
// panel sends back.
func (s *Sim) providerFor(kind, name string) (resource.Provider, resource.Row, bool) {
	for _, p := range s.resourceProviders() {
		if string(p.Kind()) != kind {
			continue
		}
		rows, err := p.List(context.Background())
		if err != nil {
			continue
		}
		for _, r := range rows {
			if r.Name == name {
				return p, r, true
			}
		}
	}
	return nil, resource.Row{}, false
}

func registerResources(st *state.Store, s *Sim) {
	// resource.list: what is on this machine. Never touches the network -
	// opening a panel must not start a download.
	st.Handle("resource.list", func(w *state.World, _ any) (any, error) {
		n, err := s.relistResources(w)
		if err != nil {
			return nil, err
		}
		return map[string]any{"rows": n}, nil
	})

	// resource.fetch: get one, as a job that can be stopped.
	st.Handle("resource.fetch", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		version, _ := stringField(p, "version")
		if name == "" {
			return nil, fmt.Errorf("resource.fetch needs a name")
		}
		// The refusal lives here rather than in the panel because scripts and
		// the control socket call this directly, and a fetch that silently
		// does nothing is indistinguishable from one that failed.
		kind, _ := stringField(p, "kind")
		if kind == "" {
			kind = string(resource.SoftDeviceKind)
		}
		prov, row, ok := s.providerFor(kind, name)
		if !ok {
			return nil, fmt.Errorf("nothing here called %s", name)
		}
		sd, ok := prov.(resource.Fetcher)
		if !ok {
			return nil, fmt.Errorf("%s fills itself as the map is used: "+
				"there is nothing to ask for out of context", name)
		}
		// The row is the authority on its own version: a default here was a
		// SoftDevice's, and it went out attached to a building set.
		if row.Version != "" {
			version = row.Version
		}
		id := "resource:" + name
		ctx, stop := context.WithCancel(context.Background())
		w.Jobs = append(w.Jobs, state.Job{
			ID: id, What: "fetching " + name + " " + version, Total: 1, Cancel: stop})
		go func() {
			defer stop()
			err := sd.Fetch(ctx, name, version, func(done, total int64) {
				if total <= 0 {
					return
				}
				// The job's own context: progress from a fetch somebody
				// stopped is noise they did not ask for.
				_, _ = st.Do(ctx, "job.progress", state.Job{
					ID: id, What: "fetching " + name + " " + version,
					Done: int(done >> 10), Total: int(total >> 10)})
			})
			// How it ended has to be said even when what ended it was the
			// cancel, so these outlive ctx - but not indefinitely.
			done, release := finishing(ctx)
			defer release()
			_, _ = st.Do(done, "job.done", id)
			if err != nil {
				if ctx.Err() != nil {
					_, _ = st.Do(done, "ui.said",
						"the "+name+" download was stopped")
					return
				}
				_, _ = st.Do(done, "ui.said", err.Error())
				return
			}
			_, _ = st.Do(done, "resource.fetched",
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
		// Listed here rather than by asking the store to do it.
		//
		// A handler already runs on the store's goroutine, so calling Do from
		// inside one posts a command to the goroutine that is running it and
		// waits for a reply that cannot come. It waited on a background context
		// with no deadline, so the store stopped for good - and this handler is
		// what runs when a download finishes, which is to say the workbench
		// would have hung the first time anything was fetched.
		if _, err := s.relistResources(w); err != nil {
			return nil, err
		}
		return map[string]any{"name": name}, nil
	})

	// resource.licence: the terms, for the interface to show.
	st.Handle("resource.licence", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		version, _ := stringField(p, "version")
		kind, _ := stringField(p, "kind")
		if kind == "" {
			kind = string(resource.SoftDeviceKind)
		}
		prov, row, ok := s.providerFor(kind, name)
		if !ok {
			return nil, fmt.Errorf("nothing here called %s", name)
		}
		if row.Version != "" {
			version = row.Version
		}
		lic, ok := prov.(resource.Licensor)
		if !ok {
			return nil, fmt.Errorf("no terms recorded for %s", name)
		}
		text := lic.Licence(name, version)
		if text == "" {
			return nil, fmt.Errorf("no licence here: %s %s is not cached", name, version)
		}
		// Nordic's file has CRLF endings. A carriage return reaches the text
		// shaper as a glyph nobody has, so the terms drew as a column of
		// tofu down the right-hand edge.
		text = strings.ReplaceAll(text, "\r\n", "\n")
		w.Licence = state.LicenceText{Kind: kind, Name: name, Version: version, Text: text}
		return map[string]any{"name": name, "version": version, "text": text}, nil
	})

	// resource.licence.hide: put the terms away again. A verb rather than a
	// flag the panel keeps, so both halves of the toggle are scriptable and
	// both are therefore capturable.
	st.Handle("resource.licence.hide", func(w *state.World, _ any) (any, error) {
		w.Licence = state.LicenceText{}
		return map[string]any{"hidden": true}, nil
	})

	// resource.remove: delete one. The caller has already asked twice.
	st.Handle("resource.remove", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		version, _ := stringField(p, "version")
		if name == "" {
			return nil, fmt.Errorf("resource.remove needs a name")
		}
		// Whichever provider owns it. Removing 7 GB of terrain and removing a
		// SoftDevice are the same gesture to the operator and different code
		// underneath, which is what the provider list is for.
		kind, _ := stringField(p, "kind")
		if kind == "" {
			kind = string(resource.SoftDeviceKind)
		}
		prov, row, ok := s.providerFor(kind, name)
		if !ok {
			return nil, fmt.Errorf("nothing here called %s", name)
		}
		if row.Version == "" {
			row.Version = version
		}
		if err := prov.Remove(context.Background(), row); err != nil {
			return nil, err
		}
		w.Say("removed " + name + " " + version)
		if _, err := s.relistResources(w); err != nil {
			return nil, err
		}
		return map[string]any{"removed": name}, nil
	})
}

// The terms the cached map data arrived under. Held here beside the providers
// that serve them so a new cache cannot be added without one.
const (
	terrainTerms = "Terrain heights are Copernicus DEM (ESA, © DLR e.V. 2010-2014 and " +
		"© Airbus Defence and Space GmbH 2014-2018, provided under COPERNICUS by the " +
		"European Union and ESA; all rights reserved) and NASA SRTM, which is public " +
		"domain. Both are redistributed for use, not resale."
	basemapTerms = "Map imagery and hillshading derive from OpenStreetMap data, " +
		"© OpenStreetMap contributors, available under the Open Database Licence " +
		"(ODbL). Any map you publish from this simulation carries that attribution."
	buildingTerms = "Building footprints, heights and materials come from " +
		"OpenStreetMap, © OpenStreetMap contributors, under the Open Database " +
		"Licence (ODbL). Heights absent from the data are inferred, and the " +
		"simulation says which."
)
