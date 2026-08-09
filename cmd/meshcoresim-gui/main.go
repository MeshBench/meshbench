// Command meshcoresim-gui is the desktop application.
//
// A native window. Everything runs in this process: there is no server, no
// browser and nothing to deploy.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/A13xB0/meshcoresim/internal/basemap"
	"github.com/A13xB0/meshcoresim/internal/terrain"
	"github.com/A13xB0/meshcoresim/internal/ui"
)

func main() {
	defaultCache, _ := os.UserCacheDir()
	cacheDir := flag.String("terrain-cache", filepath.Join(defaultCache, "meshcoresim", "terrain"),
		"where downloaded elevation tiles live")
	offline := flag.Bool("offline", false, "never download terrain; use only what is cached")
	w := flag.Int("width", 1280, "window width")
	h := flag.Int("height", 800, "window height")
	flag.Parse()

	store, err := terrain.NewTileStore(*cacheDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	store.Offline = *offline

	app := ui.New(store)

	// Basemap imagery is optional and off by default. Every layer contacts a
	// third party whose terms are not settled (ADR-0021), so the operator picks
	// one deliberately or works from the hillshade, which needs nobody.
	bm, err := basemap.NewStore(filepath.Join(*cacheDir, "..", "basemap"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "basemap unavailable:", err)
	} else {
		bm.Offline = *offline
		app.Basemap = bm
		app.SetBasemapStore(bm)
	}

	if err := app.Run("MeshcoreSim", *w, *h); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
