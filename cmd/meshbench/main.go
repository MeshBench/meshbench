// Command meshbench is the MeshCore network simulator.
//
// `workbench` opens the desktop application; every other command is headless.
// That split is deliberate and permanent: the headless path is what scripted
// runs and regression suites are built on, not a stopgap for
// the UI.
//
// Nothing but `workbench` needs a GPU, a display, or anything running anywhere
// else.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

func commands() []command {
	return []command{
		{"link", "link budget between two points, both directions", runLink},
		{"profile", "terrain profile and the worst obstruction on a path", runProfile},
		{"coverage", "coverage raster from one station, written as a PNG", runCoverage},
		{"spectrum", "what an SDR observer captures: waterfall PNG and audio", runSpectrum},
		{"terrain", "download elevation tiles for an area", runTerrain},
		{"boards", "the hardware profiles this build knows about", runBoards},
		{"firmware", "list, download or import MeshCore firmware", runFirmware},
		{"energy", "will a solar node survive the winter", runEnergy},
		{"airtime", "LoRa time on air, as the firmware computes it", runAirtime},
		{"traffic", "flood a message through a network and report what happened", runTraffic},
		{"basemap", "download map tiles for an area", runBasemap},
		{"dev", "build a MeshCore checkout and give it to the workbench", runDev},
		{"serve", "run a mesh and expose a companion to your app", runServe},
		{"test", "run a fixture on real firmware and check its assertions", runTest},
		{"headless", "run the verbs over the control socket, with no window", runHeadless},
		{"workbench", "open the desktop workbench: build a scenario on a map and run it", runWorkbench},
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	name := os.Args[1]
	if name == "-h" || name == "--help" || name == "help" {
		usage()
		return
	}
	// Bare "version" as well as the flags, because "help" is accepted
	// that way one line above and a tool that takes one and not the other
	// is the kind of thing people report.
	if name == "version" || name == "-version" || name == "--version" {
		fmt.Println(invoked(), version.Detail())
		return
	}

	for _, c := range commands() {
		if c.name == name {
			if err := c.run(ctx, os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, invoked()+":", err)
				os.Exit(1)
			}
			return
		}
	}
	fmt.Fprintf(os.Stderr, "%s: no command %q\n\n", invoked(), name)
	usage()
	os.Exit(2)
}

// invoked is the name this binary was run as.
//
// The release ships it as "meshbench" while the package is cmd/meshbench, so
// a hardcoded name is wrong for one audience or the other: a user who installs
// a release and types --help was told to run "meshbench", which is not on
// their machine. Reading argv gets both right, and a rename cannot make it
// stale again.
func invoked() string {
	if len(os.Args) == 0 || os.Args[0] == "" {
		return "meshbench"
	}
	return strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
}

func usage() {
	fmt.Fprintf(os.Stderr, "%s %s — MeshCore network simulator\n\n", invoked(), version.String())
	fmt.Fprintf(os.Stderr, "Usage: %s <command> [flags]\n", invoked())
	fmt.Fprintln(os.Stderr)
	cs := commands()
	sort.Slice(cs, func(i, j int) bool { return cs[i].name < cs[j].name })
	for _, c := range cs {
		fmt.Fprintf(os.Stderr, "  %-10s %s\n", c.name, c.summary)
	}
	fmt.Fprintln(os.Stderr, "\nEvery command takes -h for its own flags.")
	fmt.Fprintln(os.Stderr, "\nResults are a BEST CASE. The model has no multipath, bare-earth terrain and")
	fmt.Fprintln(os.Stderr, "an idealised demodulator; see docs/shortcomings.md. If it says a link will not")
	fmt.Fprintln(os.Stderr, "work, believe it. If it says a link works marginally, go and measure.")
}

// terrainFlags adds the elevation options every geographic command needs, and
// returns the store once they are parsed.
//
// Shared because the cache directory and the offline switch must mean the same
// thing everywhere: a run that silently downloads when the operator asked it not
// to is the kind of surprise that ends up on a metered connection.
func terrainFlags(fs *flag.FlagSet) func() (*terrain.TileStore, error) {
	defaultCache, _ := os.UserCacheDir()
	dir := fs.String("terrain-cache", filepath.Join(defaultCache, "meshbench", "terrain"),
		"where downloaded elevation tiles live")
	offline := fs.Bool("offline", false, "never download; answer from the cache and fail loudly otherwise")
	zoom := fs.Int("zoom", terrain.DefaultZoom, "tile zoom; 12 is about 30 m per pixel and matches the data")

	return func() (*terrain.TileStore, error) {
		s, err := terrain.NewTileStore(*dir)
		if err != nil {
			return nil, err
		}
		s.Offline = *offline
		s.Zoom = *zoom
		return s, nil
	}
}

// parse is the shared flag handling: a command's own usage, then its flags.
func parse(fs *flag.FlagSet, args []string, describe string) error {
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s — %s\n\n", invoked(), fs.Name(), describe)
		fs.PrintDefaults()
	}
	return fs.Parse(args)
}

// requireAll fails with one message naming everything that is missing, rather
// than one at a time. Being told about three missing flags in three runs is
// three times the work for no more information.
func requireAll(missing map[string]bool) error {
	var names []string
	for name, absent := range missing {
		if absent {
			names = append(names, "-"+name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return fmt.Errorf("missing required flag(s): %s", strings.Join(names, ", "))
}
