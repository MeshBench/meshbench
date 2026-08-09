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

	if err := ui.New(store).Run("MeshcoreSim", *w, *h); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
