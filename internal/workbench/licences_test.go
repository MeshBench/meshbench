package workbench

import (
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/workbench/licences"
)

// The licence window's contract: the embedded inventory is real, the forks
// are all present and come before the third-party sections, and nothing
// linked into the binary ships under a licence that cannot.

func TestLicenceInventoryIsCompleteAndOrdered(t *testing.T) {
	f, err := licences.Load()
	if err != nil {
		t.Fatalf("embedded inventory: %v", err)
	}
	order := map[string]int{}
	for i, s := range f.Sections {
		order[s.Key] = i
	}
	for _, key := range []string{"project", "forks", "bundled", "golibs", "runtime", "data"} {
		if _, ok := order[key]; !ok {
			t.Fatalf("section %q missing", key)
		}
	}
	if order["forks"] > order["bundled"] || order["bundled"] > order["golibs"] {
		t.Fatalf("the forks are modified code and come first; got order %v", order)
	}

	// Every fork in docs/repositories.md's table, by name.
	want := []string{"MeshBench/qemu", "MeshBench/tlib",
		"MeshBench/renode-infrastructure", "MeshBench/renode",
		"MeshBench/meshcore-native"}
	forks := f.Sections[order["forks"]]
	for _, name := range want {
		found := false
		for _, e := range forks.Entries {
			if e.Name == name {
				found = true
				if e.Text == "" {
					t.Errorf("%s has no licence text", name)
				}
				if e.Detail == "" {
					t.Errorf("%s does not say what was changed", name)
				}
			}
		}
		if !found {
			t.Errorf("fork %s missing from the licence window", name)
		}
	}

	golibs := f.Sections[order["golibs"]]
	if len(golibs.Entries) < 10 {
		t.Fatalf("only %d Go modules listed - the inventory looks stale; run go generate ./internal/workbench/licences", len(golibs.Entries))
	}
	for _, e := range golibs.Entries {
		if e.Licence == "" || e.Text == "" {
			t.Errorf("%s: licence %q, text %d bytes", e.Name, e.Licence, len(e.Text))
		}
		if strings.HasPrefix(e.Licence, "GPL") || strings.HasPrefix(e.Licence, "AGPL") {
			t.Errorf("%s is %s: a linked module cannot ship under it", e.Name, e.Licence)
		}
	}
}

func TestLicencePanelFiltersAndLeadsWithTheForks(t *testing.T) {
	p := &licPanel{}
	p.build()
	if p.err != "" {
		t.Fatal(p.err)
	}
	rows := p.shown()
	if len(rows) < 2 || !rows[0].heading || rows[0].section.Key != "forks" {
		t.Fatalf("the first thing shown must be the forks section; got %+v", rows[0])
	}
	if rows[1].heading || !strings.HasPrefix(rows[1].entry.Name, "MeshBench/") {
		t.Fatalf("the first entry should be a fork, got %+v", rows[1])
	}

	p.search.Editor.SetText("qemu")
	rows = p.shown()
	for _, r := range rows {
		if r.heading {
			continue
		}
		hay := strings.ToLower(r.entry.Name + r.entry.Detail + r.entry.Licence)
		if !strings.Contains(hay, "qemu") {
			t.Fatalf("search let %q through", r.entry.Name)
		}
	}

	p.search.Editor.SetText("")
	p.filter = "data"
	rows = p.shown()
	if len(rows) == 0 || rows[0].section.Key != "data" {
		t.Fatal("the data chip should scope the list to attributions")
	}
	for _, r := range rows {
		if r.section.Key != "data" {
			t.Fatalf("filter let section %q through", r.section.Key)
		}
	}
}
