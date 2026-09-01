// Where the rows come from: one provider per source, and the scan that turns
// them into the page.
//
// Split from the verbs because this is the half that changes. Adding the
// emulator toolchain made the list longer than the verbs it feeds, and
// "what can this machine hold, and what could it fetch" is the question a
// reader opens this package with.
package resources

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// resourceCacheDir is the root every provider keeps its bytes under.
func resourceCacheDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cache, "meshbench")
}

// softDeviceProvider is the SoftDevice source, told how many nodes here are
// waiting on one so a missing row reads as needed rather than optional.
func softDeviceProvider(s *session.Sim) *resource.SoftDevice {
	needed := 0
	for _, n := range s.Nodes() {
		if n.Firmware.Board == "" {
			continue
		}
		// An emulated nRF52 boots MBR, then the SoftDevice, then MeshCore.
		// Which boards those are is the catalogue's own MCU field rather than
		// a list of board names repeated here and left to drift.
		if b, err := hw.BoardByName(n.Firmware.Board); err == nil &&
			strings.HasPrefix(b.MCU, "nRF52") {
			needed++
		}
	}
	return &resource.SoftDevice{CacheDir: resourceCacheDir(), Needed: needed}
}

// toolchainProvider is the emulator toolchain, told which of its three tools
// this scenario is actually blocked on.
//
// The same counting as the SoftDevice, and for the same reason: a row that
// says "3 nodes here cannot boot without it" is a different thing from a row
// that sits there looking optional. Which emulator a board needs is the
// catalogue's MCU field rather than a list of board names kept here.
//
// The directory is the emulator's own ToolsDir rather than one spelled again,
// so a fetch cannot come to land somewhere the lookup does not search.
func toolchainProvider(s *session.Sim) *resource.Toolchain {
	needed := map[string]int{}
	for _, n := range s.Nodes() {
		if !n.Firmware.Emulated() {
			continue
		}
		b, err := hw.BoardByName(n.Firmware.Board)
		if err != nil {
			continue
		}
		for _, name := range resource.ToolsFor(b.MCU) {
			needed[name]++
		}
	}
	return &resource.Toolchain{Dir: emulated.ToolsDir(), Needed: needed}
}

// resourceProviders is every source of rows, in the order the panel shows them.
//
// The things a scenario can be blocked on first - the SoftDevice and the
// emulator toolchain - then the caches that fill themselves. Those are the
// reason this package exists: on the machine it was written for the terrain
// cache had reached 7.1 GB, and nothing in the application would tell you so.
//
// The toolchain is here because until it was, there was no way to obtain it at
// all. A release tarball carries it beside the binary, but the AppImage and
// the .deb carry only radioserver and a source checkout carried nothing, so
// the tools directory the emulator lookup searches was one that nothing ever
// put anything in.
func resourceProviders(s *session.Sim) []resource.Provider {
	root := resourceCacheDir()
	dir := func(k resource.Kind, label, sub, purpose, terms string) resource.Provider {
		return &resource.DirCache{
			K: k, Label: label, Dir: filepath.Join(root, sub),
			Purpose: purpose, Terms: terms,
		}
	}
	return []resource.Provider{
		softDeviceProvider(s),
		toolchainProvider(s),
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
func relistResources(s *session.Sim, w *state.World) (int, error) {
	var out []state.ResourceRow
	// One provider failing must not empty the page. A cache directory that
	// cannot be read is a row that says so, beside the ones that could.
	for _, p := range resourceProviders(s) {
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
func providerFor(s *session.Sim, kind, name string) (resource.Provider, resource.Row, bool) {
	for _, p := range resourceProviders(s) {
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
