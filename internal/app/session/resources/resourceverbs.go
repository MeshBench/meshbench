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
	st.HandleSpec("resource.list", state.Spec{
		What: "say what this machine already holds of everything downloaded at " +
			"runtime, what it has cost the disk, and what could still be fetched",
		Returns: []string{"rows", "resources"},
		Answers: "`rows` is a count, kept for the callers that were already " +
			"reading it; `resources` is the rows themselves, each carrying its " +
			"kind, name, version, state, size and path, why it is in the state " +
			"it is in, and whether it can be fetched, carries terms, or fills " +
			"itself as the map is used. A provider whose directory cannot be " +
			"read contributes one row saying so rather than emptying the list.",
		Example: &state.Example{
			Params: map[string]any{}, What: "see what this machine already holds",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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

	st.HandleSpec("resource.fetch", state.Spec{
		What: "download one runtime resource as a job that can be stopped, which " +
			"is how a headless machine gets the SoftDevice or the emulator " +
			"toolchain an emulated board is blocked on",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Required: true, Primary: true,
				What: "the row's name as resource.list gives it; absent is " +
					"refused, and so is a name the chosen provider does not hold"},
			{Name: "kind", Type: state.ParamString,
				What: "which provider owns it - softdevice, toolchain, terrain, " +
					"basemap or buildings - named rather than bare; absent means " +
					"softdevice, so a row of another kind is not found without it"},
			{Name: "version", Type: state.ParamString,
				What: "the release to fetch, named rather than bare; the row's " +
					"own version overrides it wherever the row carries one"},
		},
		Returns: []string{"fetching", "version"},
		Answers: "It answers as soon as the download is started, not when it has " +
			"arrived. Progress is the job `resource:<name>`, which `job.list` " +
			"shows and `job.cancel` stops, and how it ended is said aloud rather " +
			"than returned here. A resource that fills itself as the map is used " +
			"is refused: there is nothing to ask for out of context.",
		Example: &state.Example{
			Params: map[string]any{"name": "s140"},
			What:   "fetch the SoftDevice an emulated nRF52 boots",
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleInternalSpec("resource.fetched", state.Spec{
		What: "take a finished download back onto the store's goroutine: say " +
			"once that it is cached and where its terms are, then relist what " +
			"is on disk",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Required: true, Primary: true,
				What: "the resource that arrived"},
			{Name: "version", Type: state.ParamString,
				What: "the release that arrived, for the sentence it says"},
		},
		Returns: []string{"name"},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("resource.licence", state.Spec{
		What: "read the terms a cached resource arrived under, and leave them " +
			"open in the snapshot for the interface to show",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Required: true, Primary: true,
				What: "the row's name; absent or unknown to the chosen provider " +
					"is refused"},
			{Name: "kind", Type: state.ParamString,
				What: "which provider owns it, named rather than bare; absent " +
					"means softdevice"},
			{Name: "version", Type: state.ParamString,
				What: "the release whose terms to read, named rather than bare; " +
					"the row's own version overrides it where it has one"},
		},
		Returns: []string{"name", "version", "text"},
		Answers: "The whole licence text, and the same text is put into the " +
			"snapshot until `resource.licence.hide` clears it. Refused where the " +
			"resource is not cached: the terms are a file fetched beside it, not " +
			"a string this build carries.",
		Example: &state.Example{
			Params: map[string]any{"name": "s140"},
			What:   "read the terms the SoftDevice came under",
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("resource.licence.hide", state.Spec{
		What: "clear the terms resource.licence left open, so the interface " +
			"stops showing them",
		Returns: []string{"hidden"},
		Answers: "`hidden` is always true. Calling it with nothing open is not " +
			"an error, and it is left out of the session journal because it " +
			"changes a window rather than the world.",
		Example: &state.Example{
			Params: map[string]any{}, What: "put the terms away",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		w.Licence = state.LicenceText{}
		return map[string]any{"hidden": true}, nil
	})

	// The caller has already asked twice.
	st.HandleSpec("resource.remove", state.Spec{
		What: "delete one cached resource from the disk, through whichever " +
			"provider owns it, and relist what is left",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Required: true, Primary: true,
				What: "the row's name; absent is refused, and so is a name the " +
					"chosen provider does not hold"},
			{Name: "kind", Type: state.ParamString,
				What: "which provider owns it, named rather than bare; absent " +
					"means softdevice, so removing terrain or a toolchain needs it"},
			{Name: "version", Type: state.ParamString,
				What: "the release to remove, named rather than bare; used only " +
					"where the row carries no version of its own"},
		},
		Returns: []string{"removed"},
		Answers: "Nothing is confirmed here: this deletes, and the asking " +
			"belongs to whatever called it. Removing 7 GB of terrain and " +
			"removing a SoftDevice are the same call with a different kind.",
		Example: &state.Example{
			Params: map[string]any{"name": "terrain tiles", "kind": "terrain"},
			What:   "give the terrain cache's disk back",
		},
	}, func(w *state.World, p any) (any, error) {
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
