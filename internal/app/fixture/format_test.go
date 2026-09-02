package fixture_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/fixture"
)

// write puts a one-node fixture on disk with whatever format number is asked
// for, which is the only thing these tests vary.
func write(t *testing.T, format string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "one.json")
	body := fmt.Sprintf(`{%s"name":"one","nodes":[{"Name":"A",`+
		`"Kind":"simple-repeater"}]}`, format)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A file from a later build is refused rather than read, because reading the
// three quarters of it this build recognises would answer a question about a
// network nobody described.
func TestLoadRefusesALaterFormat(t *testing.T) {
	ahead := fixture.Format + 1
	path := write(t, fmt.Sprintf(`"format":%d,`, ahead))

	_, err := fixture.Load(path)
	if err == nil {
		t.Fatalf("format %d loaded; a file from a later build must be refused",
			ahead)
	}
	// Both numbers, because a reader who is told only that the versions
	// disagree still has to work out which end of the pair to change.
	for _, want := range []string{fmt.Sprint(ahead), fmt.Sprint(fixture.Format)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not name %s", err, want)
		}
	}
	if !strings.Contains(err.Error(), "later MeshBench") {
		t.Errorf("refusal %q does not say which end wrote the file", err)
	}
}

// The other direction still works, and has to: every file written before the
// number existed reads as format 0, including the ones this release ships.
func TestLoadReadsAnEarlierFormat(t *testing.T) {
	for name, stamp := range map[string]string{
		"before the field existed": "",
		"this build's own":         fmt.Sprintf(`"format":%d,`, fixture.Format),
	} {
		t.Run(name, func(t *testing.T) {
			f, err := fixture.Load(write(t, stamp))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(f.Nodes) != 1 {
				t.Fatalf("got %d nodes, want 1", len(f.Nodes))
			}
		})
	}
}

// The shipped set declares its format, so that the files a first-time user
// opens are the worked example of a fixture that says what it is.
func TestShippedFixturesDeclareTheFormat(t *testing.T) {
	paths, err := filepath.Glob("../../../fixtures/fixture-*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no shipped fixtures found: %v", err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // a path this test globbed
		if err != nil {
			t.Fatal(err)
		}
		var head struct {
			Format int `json:"format"`
		}
		if err := json.Unmarshal(b, &head); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if head.Format != fixture.Format {
			t.Errorf("%s declares format %d, want %d",
				filepath.Base(p), head.Format, fixture.Format)
		}
	}
}
