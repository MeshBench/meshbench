package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/firmware"
)

// runDev is the firmware development loop as one command.
//
//	meshbench dev -from ~/src/MeshCore
//
// It builds the checkout, hands the result to a running workbench, and assigns
// it. Nothing is added to the MeshCore tree and nothing in it is modified: the
// build happens in a temporary directory and the checkout is read only as far
// as this command is concerned.
func runDev(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dev", flag.ExitOnError)
	from := fs.String("from", ".", "a MeshCore checkout to build")
	role := fs.String("role", "simple_repeater",
		"which application: simple_repeater, companion_radio or simple_room_server")
	name := fs.String("name", "", "what to call the build; the git branch by default")
	watch := fs.Bool("watch", false, "rebuild and reassign whenever a source file changes")
	assign := fs.Bool("assign", true, "assign the build to every node of that role")
	if err := parse(fs, args, "build a MeshCore checkout and give it to the workbench"); err != nil {
		return err
	}

	src, err := filepath.Abs(*from)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(src, "examples", *role)); err != nil {
		return fmt.Errorf("%s does not look like a MeshCore checkout: no examples/%s\n\n"+
			"Point -from at the top of the tree, the directory holding src/ and examples/", src, *role)
	}

	build := func() error {
		// The same call the firmware.build verb makes, so a build started
		// here and a build started by a script are one thing rather than two
		// that agree today.
		built, err := firmware.Build(ctx, firmware.BuildOptions{
			Source: src, Role: *role, Label: *name, Log: os.Stderr,
		})
		if err != nil {
			return err
		}
		fmt.Printf("%s  %s  %.1f MB\n", built.Label, built.Path,
			float64(built.Bytes)/1e6)

		// Handing it over is best effort: a workbench that is not running is a
		// perfectly normal state, and the build is still in the cache for the
		// next time one starts.
		//
		// firmware.Build already left the binary at the path being handed to
		// firmware.import here, so on a native build this call is a second
		// import of the same file. It stays rather than being skipped: it is
		// the only public verb that tells a running workbench a build now
		// exists and refreshes its library, and copyFile's same-file guard
		// makes the redundant copy a stat and a chmod rather than a hazard.
		// What it must not do is go unchecked: the response is what actually
		// landed, not a promise that it did.
		resp, err := tell("firmware.import", map[string]any{
			"path": built.Path, "role": *role, "label": built.Label})
		if err != nil {
			fmt.Println("  workbench not running, so it is cached but not loaded")
			return nil //nolint:nilerr // not an error: the build is cached either way
		}
		landed, ok := importedBytes(resp)
		if !ok || landed != built.Bytes {
			return fmt.Errorf(
				"workbench reports %d bytes for %s, the build produced %d: "+
					"the import did not land correctly, so it was not assigned",
				landed, built.Label, built.Bytes)
		}
		fmt.Printf("  in the workbench's firmware library (%d bytes)\n", landed)
		if *assign {
			if _, err := tell("firmware.set", map[string]any{"role": *role, "version": built.Label}); err == nil {
				fmt.Printf("  assigned to every %s node\n", *role)
			}
		}
		return nil
	}

	if err := build(); err != nil {
		return err
	}
	if !*watch {
		return nil
	}

	fmt.Println("\nwatching for changes; press ctrl-c to stop")
	last := newestSource(src)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
		if n := newestSource(src); n.After(last) {
			last = n
			fmt.Printf("\n%s  change detected, rebuilding\n", time.Now().Format("15:04:05"))
			if err := build(); err != nil {
				fmt.Println("  build failed:", err)
			}
		}
	}
}

// newestSource is the most recent modification time under a checkout's own
// sources, which is enough to notice an edit without watching every file.
func newestSource(root string) time.Time {
	var newest time.Time
	for _, dir := range []string{"src", "examples"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil //nolint:nilerr // an unreadable entry skips, it does not stop the walk
			}
			switch filepath.Ext(p) {
			case ".cpp", ".h", ".hpp", ".c":
				if fi.ModTime().After(newest) {
					newest = fi.ModTime()
				}
			}
			return nil
		})
	}
	return newest
}

// tell sends one verb to a running workbench and returns its reply.
//
// Through the control client rather than by opening a socket here. This built
// the path by hand - XDG_RUNTIME_DIR or /run/user/<uid> - which is a Linux
// sentence, and os.Getuid() does not fail on Windows so much as return -1. One
// resolver, in the package that owns the address, is the only way the two
// cannot disagree about where a workbench is.
func tell(method string, params map[string]any) (json.RawMessage, error) {
	c, err := control.Dial()
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	return c.Call(method, params)
}

// importedBytes reads the size firmware.import reports for what it actually
// wrote, rather than trusting that a call which returned no error moved the
// bytes it was asked to.
//
// A same-file copy can silently truncate the build it is importing and still
// answer without an error, so the only thing that tells the truth is the size
// in the reply.
func importedBytes(resp json.RawMessage) (int64, bool) {
	var got struct {
		Bytes int64 `json:"bytes"`
	}
	if err := json.Unmarshal(resp, &got); err != nil {
		return 0, false
	}
	return got.Bytes, true
}
