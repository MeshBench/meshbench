// Everything downloaded at runtime, as verbs.
//
// The panel is the obvious consumer, but the verbs come first and stand alone:
// the control socket is how a script or a headless machine reaches any of
// this, and the SoftDevice in particular unblocks emulated nRF52 boards for
// somebody who never opens the interface at all.
package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerResources(st *state.Store, s *session.Sim) {
	// Never touches the network - opening a panel must not start a download.
	st.Handle("resource.list", func(w *state.World, _ any) (any, error) {
		n, err := relistResources(s, w)
		if err != nil {
			return nil, err
		}
		// The rows, not only how many there are.
		//
		// It answered {"rows": 5} and left the rows in the snapshot, where
		// only a panel could reach them, so from outside the window "is the
		// emulator toolchain here, and can I fetch it" was unanswerable
		// without reading this file. The count keeps its old key for whoever
		// is already reading it.
		return map[string]any{"rows": n, "resources": resourceRows(w.Resources)}, nil
	})

	st.Handle("resource.fetch", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "name")
		version, _ := session.NamedField(p, "version")
		if name == "" {
			return nil, fmt.Errorf("resource.fetch needs a name")
		}
		// The refusal lives here rather than in the panel because scripts and
		// the control socket call this directly, and a fetch that silently
		// does nothing is indistinguishable from one that failed.
		kind, _ := session.NamedField(p, "kind")
		if kind == "" {
			kind = string(resource.SoftDeviceKind)
		}
		prov, row, ok := providerFor(s, kind, name)
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
			done, release := session.Finishing(ctx)
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

	st.HandleInternal("resource.fetched", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "name")
		version, _ := session.NamedField(p, "version")
		// Said aloud once, because a licensed binary arriving silently is the
		// thing the licence question was asked to avoid. Where the terms are
		// rather than that there are some: the SoftDevice caches Nordic's own
		// file beside the image and the emulators carry theirs in the row, and
		// "beside it" was true of only one of those.
		w.Say(name + " " + version + " is cached; its terms are under Licence on its row")
		// Listed here rather than by asking the store to do it.
		//
		// A handler already runs on the store's goroutine, so calling Do from
		// inside one posts a command to the goroutine that is running it and
		// waits for a reply that cannot come. It waited on a background context
		// with no deadline, so the store stopped for good - and this handler is
		// what runs when a download finishes, which is to say the workbench
		// would have hung the first time anything was fetched.
		if _, err := relistResources(s, w); err != nil {
			return nil, err
		}
		return map[string]any{"name": name}, nil
	})

	st.Handle("resource.licence", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "name")
		version, _ := session.NamedField(p, "version")
		kind, _ := session.NamedField(p, "kind")
		if kind == "" {
			kind = string(resource.SoftDeviceKind)
		}
		prov, row, ok := providerFor(s, kind, name)
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

	// A verb rather than a flag the panel keeps, so both halves of the toggle
	// are scriptable and both are therefore capturable.
	st.Handle("resource.licence.hide", func(w *state.World, _ any) (any, error) {
		w.Licence = state.LicenceText{}
		return map[string]any{"hidden": true}, nil
	})

	// The caller has already asked twice.
	st.Handle("resource.remove", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "name")
		version, _ := session.NamedField(p, "version")
		if name == "" {
			return nil, fmt.Errorf("resource.remove needs a name")
		}
		// Whichever provider owns it. Removing 7 GB of terrain and removing a
		// SoftDevice are the same gesture to the operator and different code
		// underneath, which is what the provider list is for.
		kind, _ := session.NamedField(p, "kind")
		if kind == "" {
			kind = string(resource.SoftDeviceKind)
		}
		prov, row, ok := providerFor(s, kind, name)
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
		if _, err := relistResources(s, w); err != nil {
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
