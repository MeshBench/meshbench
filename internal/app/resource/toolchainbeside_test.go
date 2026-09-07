package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A release bundle carries the emulators beside the binary, and the toolchain
// rows looked only in the tools directory. So on every bundled install these
// said "needed" with no path, while setup.check - which asks the lookup a
// booting node asks - said ready and named where they were. Two verbs, one
// machine, opposite answers.
func TestAToolBesideTheBinaryIsNotNeeded(t *testing.T) {
	beside := t.TempDir()
	// A file standing in for an installed emulator, with a size to report.
	path := filepath.Join(beside, "qemu-system-xtensa")
	if err := os.WriteFile(path, make([]byte, 4096), 0o755); err != nil {
		t.Fatal(err)
	}

	tc := &Toolchain{
		Dir:    t.TempDir(), // an empty cache: nothing was ever fetched
		Needed: map[string]int{"qemu-system-xtensa": 3},
		Find: func(name string) (string, error) {
			if name == "qemu-system-xtensa" {
				return path, nil
			}
			return "", os.ErrNotExist
		},
	}
	rows, err := tc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, r := range rows {
		if r.Name != "qemu-system-xtensa" {
			continue
		}
		found = true
		if r.State != OnDisk {
			t.Errorf("a tool beside the binary is reported %q, want %q", r.State, OnDisk)
		}
		if r.Path != path {
			t.Errorf("the row says it is at %q, want %q", r.Path, path)
		}
		if r.Bytes != 4096 || r.Estimated {
			t.Errorf("size came back %d estimated=%v, want 4096 measured", r.Bytes, r.Estimated)
		}
	}
	if !found {
		t.Fatal("no row for the tool at all")
	}
}

// With no lookup and an empty cache the rows still say what is missing, so a
// source checkout is not told everything is present.
func TestWithNoLookupTheCacheStillDecides(t *testing.T) {
	tc := &Toolchain{Dir: t.TempDir(), Needed: map[string]int{"renode": 2}}
	rows, err := tc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Name == "renode" && r.State == OnDisk {
			t.Error("renode is reported on disk with an empty cache and no lookup")
		}
	}
}
