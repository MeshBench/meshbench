package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/world/basemap"
)

// runBasemap downloads map tiles for an area.
//
// Separate from the map's own on-demand fetching because prefetching a whole
// region is a different act with a different cost, and ADR-0019's rule applies:
// say what it will cost before spending it.
func runBasemap(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("basemap", flag.ExitOnError)
	defaultCache, _ := os.UserCacheDir()
	cache := fs.String("cache", filepath.Join(defaultCache, "meshcoresim", "basemap"), "tile cache")
	layerID := fs.String("layer", "", "which layer; omit to list them")
	south := fs.Float64("south", 0, "southern edge")
	north := fs.Float64("north", 0, "northern edge")
	west := fs.Float64("west", 0, "western edge")
	east := fs.Float64("east", 0, "eastern edge")
	zoom := fs.Int("zoom", 11, "tile zoom")
	estimate := fs.Bool("estimate", false, "report the download and stop")
	if err := parse(fs, args, "download map tiles for an area"); err != nil {
		return err
	}

	if *layerID == "" {
		fmt.Printf("%-14s %-18s %-9s %s\n", "ID", "NAME", "KIND", "ATTRIBUTION")
		for _, l := range basemap.Layers() {
			fmt.Printf("%-14s %-18s %-9s %s\n", l.ID, l.Name, l.Kind, l.Attribution)
		}
		fmt.Println("\nEvery layer here contacts a third party whose terms have not been")
		fmt.Println("checked against how this application uses them. Attribution is required")
		fmt.Println("wherever the imagery is shown; see ADR-0021 and docs/shortcomings.md.")
		return nil
	}

	l, ok := basemap.ByID(*layerID)
	if !ok {
		return fmt.Errorf("no layer %q; run without -layer to list them", *layerID)
	}
	if err := requireAll(map[string]bool{
		"south": *south == 0, "north": *north == 0, "west": *west == 0, "east": *east == 0,
	}); err != nil {
		return err
	}

	s, err := basemap.NewStore(*cache)
	if err != nil {
		return err
	}
	e := s.Estimate(l, *south, *north, *west, *east, *zoom)
	fmt.Printf("%s at zoom %d: %d tiles, %d cached, %d to fetch (about %d MB).\n",
		l.Name, *zoom, e.Tiles, e.Cached, e.ToFetch, e.BytesRough>>20)
	if *estimate || e.ToFetch == 0 {
		return nil
	}

	err = s.Prefetch(ctx, l, *south, *north, *west, *east, *zoom, func(done, total int) {
		if done == total || done%20 == 0 {
			fmt.Fprintf(os.Stderr, "\r%d/%d", done, total)
		}
	})
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s\nThat attribution must appear wherever these tiles are shown.\n", l.Attribution)
	return nil
}
