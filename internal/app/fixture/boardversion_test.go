package fixture_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A board node's version is the bare one, and a native node's carries its role.
//
// The two namespaces look alike and are not. A native build is resolved by
// release tag - repeater-v1.17.1 - while a board image is named
// <role>-<version>.<format> on disk and BoardImage.Version is the bare
// "v1.17.1" ParseAssetName pulls out of the published asset. Pin a board node
// to the tag form and neither the computed path nor the fallback that scans
// the cache matches it: the node fails with "no image in the cache" naming a
// version nothing was ever going to have.
//
// This is checked here because nothing else can. tools/fixture-check.sh skips
// any fixture holding an emulated node - the lab runners have no emulator - so
// the one fixture that would have caught it is the one CI never opens.
func TestBoardNodesPinTheBareVersion(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "fixtures", "fixture-*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	rolePrefixes := []string{"repeater-", "companion-", "room-server-"}

	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Nodes []struct {
				Name     string
				Firmware struct{ Role, Version, Board string }
			}
		}
		if err := json.Unmarshal(body, &f); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		name := filepath.Base(path)
		for _, n := range f.Nodes {
			v, board := n.Firmware.Version, n.Firmware.Board
			if v == "" {
				continue
			}
			if board != "" {
				for _, p := range rolePrefixes {
					if strings.HasPrefix(v, p) {
						t.Errorf("%s: %s runs on %s and pins %q; a board image's "+
							"version is the bare one, so this resolves to nothing",
							name, n.Name, board, v)
					}
				}
				if !strings.HasPrefix(v, "v") {
					t.Errorf("%s: %s pins %q, which is neither form", name, n.Name, v)
				}
				continue
			}
			// A native build, which is the other way round: the bare form names
			// no release in either repository.
			bare := true
			for _, p := range rolePrefixes {
				if strings.HasPrefix(v, p) {
					bare = false
				}
			}
			if bare {
				t.Errorf("%s: %s runs a host build and pins %q; a native build is "+
					"resolved by release tag, and a bare version names no release",
					name, n.Name, v)
			}
		}
	}
}
