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

	"github.com/MeshBench/meshbench/internal/app/flagdump"
	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

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
	// Unlisted, because it is addressed to tools/flagdoc rather than to an
	// operator: it describes the command line instead of doing anything with
	// it. Listing it in the usage would offer somebody a command whose output
	// is only useful to a generator.
	if name == "flagdump" {
		if err := describeSelf(ctx); err != nil {
			fmt.Fprintln(os.Stderr, invoked()+":", err)
			os.Exit(1)
		}
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
//
// It is also where the CLI reference comes from. Every command routes through
// here after declaring its flags and before doing anything with them, which is
// the one moment the flag set exists and nothing has run, so a process asked to
// describe itself takes the set here and stops the command.
func parse(fs *flag.FlagSet, args []string, describe string) error {
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s — %s\n\n", invoked(), fs.Name(), describe)
		fs.PrintDefaults()
	}
	if flagdump.Wanted() {
		flagdump.Record(fs.Name(), describe, fs)
		return flagdump.ErrRecorded
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

// notGiven is which of these flags were never written on the command line, in
// the shape requireAll takes.
//
// Asked of the flag set rather than of the values, because a coordinate cannot
// be tested for absence. Zero is a latitude and a longitude like any other -
// the equator and the prime meridian - and inferring "not supplied" from it
// refused Greenwich, refused the Gulf of Guinea, and refused every study area
// that straddles either line.
func notGiven(fs *flag.FlagSet, names ...string) map[string]bool {
	seen := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { seen[f.Name] = true })
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = !seen[n]
	}
	return out
}
