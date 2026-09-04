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

	"github.com/MeshBench/meshbench/internal/app/update"
	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
)

// fatal says why, in the one place a Windows user can see it.
//
// A release binary linked for the GUI subsystem has no standard handles, so
// the usual write to stderr reaches nobody and the process appears to vanish.
// adoptConsole gives it the terminal's handles when there is a terminal;
// reportFatal writes the message down when there is not, and the exit names
// the file so the next person is not left guessing.
func fatal(code int, msg string) {
	fmt.Fprintln(os.Stderr, msg)
	if path := reportFatal(msg); path != "" {
		fmt.Fprintln(os.Stderr, "written to", path)
	}
	os.Exit(code)
}

func main() {
	adoptConsole()
	recordCrashes()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	name, args := commandFor(os.Args)
	if name == "-h" || name == "--help" || name == "help" {
		usage()
		return
	}
	// Bare "version" as well as the flags, because "help" is accepted
	// that way one line above and a tool that takes one and not the other
	// is the kind of thing people report.
	if name == "version" || name == "-version" || name == "--version" {
		// The variant as well, because release filenames no longer carry a
		// version and this is where "what exactly have I got" is answered. It
		// is also the first question worth asking of a machine whose emulated
		// boards will not start.
		line := invoked() + " " + version.Detail()
		if v := update.ThisVariant(); v != update.UnknownVariant {
			line += ", " + string(v)
		}
		fmt.Println(line)
		return
	}

	for _, c := range commands() {
		if c.name == name {
			if err := c.run(ctx, args); err != nil {
				fatal(1, invoked()+": "+err.Error())
			}
			return
		}
	}
	fmt.Fprintf(os.Stderr, "%s: no command %q\n\n", invoked(), name)
	usage()
	os.Exit(2)
}

// commandFor is the command to run and the flags for it, from the
// argument vector.
//
// A bare invocation opens the workbench. Both launchers that have
// somewhere to put an argument already say so - the .desktop file runs
// "meshbench workbench %f" and the macOS wrapper execs the same word - but
// the MSI's Start menu shortcut carries no arguments and a meshbench.exe
// double-clicked out of the zip has nowhere to carry them. Those two used
// to reach the usage text and exit 2, and on Windows a release is linked
// -H windowsgui, so the text went to a stderr that was never opened: the
// application started and vanished with nothing to go on. The README
// beside the binary has always said "run meshbench.exe - it opens the
// workbench"; this is that sentence being true.
func commandFor(argv []string) (string, []string) {
	if len(argv) < 2 {
		return "workbench", nil
	}
	return argv[1], argv[2:]
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
