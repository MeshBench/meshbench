// The pins, enforced.
//
// packaging/emulator-pins.env is what the release pipeline fetches; the
// catalogue beside this file is what a first run downloads onto somebody's own
// machine. They are two statements of one fact, and while they were allowed to
// drift the pipeline rode /latest from both forks: a fork release re-cut by
// somebody else changed what every MeshBench build bundled, with no commit here
// to show for it, and three platforms shipped bundles with no emulator in them
// under a green build.
//
// A test rather than a CI step, so it fails on the machine of whoever moved a
// pin while they still remember which half they meant to move.
package resource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pinsFile is the one place a release tag is written down.
const pinsFile = "../../../packaging/emulator-pins.env"

// readPins reads the KEY=value lines, which is all that file is allowed to be:
// it is sourced by the workflow as shell, so anything cleverer would be a
// second language in a file two readers already parse differently.
func readPins(t *testing.T) map[string]string {
	t.Helper()
	b, err := os.ReadFile(pinsFile)
	if err != nil {
		t.Fatalf("reading the pins: %v", err)
	}
	pins := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Errorf("pins: %q is neither a comment nor KEY=value", line)
			continue
		}
		pins[k] = v
	}
	return pins
}

// tagOf is the release a download URL names.
func tagOf(t *testing.T, base string) string {
	t.Helper()
	_, after, ok := strings.Cut(base, "/releases/download/")
	if !ok {
		t.Fatalf("%s is not a release download URL", base)
	}
	return strings.Trim(after, "/")
}

func TestCatalogueIsPinnedToATag(t *testing.T) {
	pins := readPins(t)
	for _, c := range []struct{ key, base string }{
		{"QEMU_RELEASE", qemuBase},
		{"RENODE_RELEASE", renodeBase},
		{"RADIOSERVER_RELEASE", radioBase},
	} {
		// "latest" moves under whoever published last, which is the whole
		// failure this file exists to stop.
		if strings.Contains(c.base, "/latest") {
			t.Errorf("%s follows a moving release: %s", c.key, c.base)
		}
		if got, want := tagOf(t, c.base), pins[c.key]; got != want {
			t.Errorf("%s: the catalogue fetches %s and the pipeline fetches %s. "+
				"A first run and a release bundle would be different emulators",
				c.key, got, want)
		}
	}
}

// pinPrefix maps a tool to the pins that name it. Spelled out rather than
// derived, because the tool's own name is the binary and the pin's is the
// project, and qemu-system-xtensa against QEMU is exactly where a derivation
// would have to start guessing.
var pinPrefix = map[string]string{
	"radioserver":        "RADIOSERVER",
	"qemu-system-xtensa": "QEMU",
	"renode":             "RENODE",
}

// pinPlatform is the pins' spelling of a Go platform key.
func pinPlatform(goPlatform string) string {
	return strings.ToUpper(strings.ReplaceAll(goPlatform, "/", "_"))
}

func TestPinsAndCatalogueNameTheSameAssets(t *testing.T) {
	pins := readPins(t)
	for _, rel := range toolReleases {
		prefix, ok := pinPrefix[rel.Name]
		if !ok {
			t.Errorf("%s has no pins; add it to packaging/emulator-pins.env", rel.Name)
			continue
		}
		// Contains rather than equals: Version is what a person is shown, and
		// radioserver's tag says radioserver-v2 where the row says v2. What
		// must not happen is a row telling somebody it will fetch one release
		// while the URL beside it fetches another.
		if tag := pins[prefix+"_RELEASE"]; !strings.Contains(tag, rel.Version) {
			t.Errorf("%s tells the operator it is version %q and fetches release %q",
				rel.Name, rel.Version, tag)
		}
		for platform, asset := range rel.Assets {
			key := prefix + "_ASSET_" + pinPlatform(platform)
			pinned, named := pins[key]
			if !named {
				t.Errorf("%s has a %s build and the pins have no %s", rel.Name, platform, key)
				continue
			}
			got := filepath.Base(asset.URL)
			// An empty pin is a platform the bundle deliberately ships
			// without. The catalogue offering a download for it means one of
			// the two was updated when a fork published a build and the other
			// was not.
			if pinned == "" {
				t.Errorf("%s: the catalogue downloads %s for %s and the bundle ships nothing",
					rel.Name, got, platform)
				continue
			}
			if got != pinned {
				t.Errorf("%s on %s: the catalogue takes %s and the bundle takes %s",
					rel.Name, platform, got, pinned)
			}
		}
	}
}
