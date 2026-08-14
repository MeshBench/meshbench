package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/A13xB0/meshcoresim/internal/basemap"
	"github.com/A13xB0/meshcoresim/internal/ui"
)

// runWorkbench1 opens the previous, imgui workbench.
//
// Kept until the Gio one has bedded in, then this and internal/ui go. It is
// not maintained: fixes land in the workbench, not here.
func runWorkbench1(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("workbench", flag.ExitOnError)
	store := terrainFlags(fs)
	nodesPath := fs.String("nodes", "", "scenario JSON, or a CoreScope/Beacon export, to open with")
	source := fs.String("source", "", "load nodes from a provider: corescope or beacon")
	baseURL := fs.String("url", "", "provider base URL")
	token := fs.String("token", "", "provider token, if it needs one")
	board := fs.String("board", "RAK4631", "board profile for imported nodes")
	freq := fs.Float64("freq", 869.525, "frequency, MHz")
	sf := fs.Int("sf", 10, "spreading factor")
	bw := fs.Float64("bandwidth", 250, "bandwidth, kHz")
	layer := fs.String("layer", "carto-dark", "basemap: carto-dark, carto-light, osm, esri-imagery, esri-topo")
	tileCache := fs.String("tile-cache", defaultTileCache(), "where map tiles are kept")
	offlineTiles := fs.Bool("offline-tiles", false, "use only tiles already downloaded")
	width := fs.Int("width", 1600, "window width")
	height := fs.Int("height", 1000, "window height")
	wayland := fs.Bool("wayland", false, "stay on native Wayland (pop-out windows are impossible there; imgui disables them)")
	scale := fs.Float64("scale", 0, "UI scale, e.g. 1.5; 0 detects (XWayland lies about it, so set this if the UI is tiny)")
	if err := parse(fs, args, "open the workbench: build a scenario on a map and run it"); err != nil {
		return err
	}

	// X11 by default, even inside a Wayland session (XWayland picks it up).
	//
	// Not nostalgia: Dear ImGui's multi-viewport support — dragging a node's
	// console out to a second monitor as its own OS window — cannot work under
	// Wayland, because the protocol forbids a client from positioning windows
	// globally. Anyone who prefers native Wayland can have it with -wayland,
	// minus detachable windows.
	//
	// Unsetting WAYLAND_DISPLAY is not enough, and that was the bug that made
	// every normal launch quietly come up single-window with dead pop-out
	// buttons: wl_display_connect falls back to $XDG_RUNTIME_DIR/wayland-0
	// when the variable is absent, so GLFW still found the compositor.
	//
	// GLFW's platform tiebreak (platform.c) is explicit: XDG_SESSION_TYPE=x11
	// plus a DISPLAY hard-selects X11 before any Wayland probe runs. Both
	// variables are set because the tiebreak needs the pair — and
	// WAYLAND_DISPLAY still goes away so nothing else in the stack picks it
	// up. A bogus WAYLAND_DISPLAY instead of this crashes: with
	// XDG_SESSION_TYPE=wayland it hard-selects Wayland and the failed connect
	// is fatal rather than a fallback.
	if !*wayland {
		_ = os.Unsetenv("WAYLAND_DISPLAY")
		if os.Getenv("DISPLAY") != "" {
			_ = os.Setenv("XDG_SESSION_TYPE", "x11")
		}
	}

	t, err := store()
	if err != nil {
		return err
	}

	app := ui.New(t)
	app.UIScale = *scale

	// Tiles before nodes, so the map has somewhere to draw before the view is
	// fitted to whatever gets loaded.
	bm, err := basemap.NewStore(*tileCache)
	if err != nil {
		return fmt.Errorf("map tiles: %w", err)
	}
	app.SetBasemapStore(bm)
	app.SetFetchTiles(!*offlineTiles)
	if err := app.SetLayer(*layer); err != nil {
		return err
	}

	// A network on the command line rather than only through the load panel,
	// because the common case is opening the same deployment repeatedly and
	// retyping a URL every time is not a workflow.
	if *nodesPath != "" || *source != "" {
		nodes, err := loadNodes(ctx, *nodesPath, *source, *baseURL, *token, *board, *freq, *sf, *bw)
		if err != nil {
			return err
		}
		app.SetNodes(nodes)
	}

	return app.Run("MeshBench - main window", *width, *height)
}

// defaultTileCache keeps tiles with the user's other caches.
//
// The same place the basemap command uses. Two caches would mean the workbench
// re-downloading what a prefetch already fetched, which is the one thing a
// prefetch exists to avoid.
func defaultTileCache() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "tiles"
	}
	return filepath.Join(dir, "meshcoresim", "tiles")
}
