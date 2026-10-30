package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// deckWith installs a T-Deck build into a fresh cache, speaking dialect.
func deckWith(t *testing.T, version, dialect string) scenario.Node {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("HOME", cache)
	dir := filepath.Join(firmware.DefaultCacheDir(), "board", "LilyGo_TDeck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(dir, "companion_radio_usb@"+version+".bin")
	if err := os.WriteFile(image, make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	if dialect != "" {
		if err := firmware.SaveBuildSettings(image, firmware.BuildSettings{Dialect: dialect}); err != nil {
			t.Fatal(err)
		}
	}
	return scenario.Node{Name: "Deck", Kind: scenario.Companion,
		Firmware: scenario.FirmwareRef{Version: version, Board: "LilyGo_TDeck", Role: "companion_radio_usb"}}
}

// A node whose build does not speak MeshCore's console is told why it is sent
// nothing, rather than sent lines it refuses.
func TestAFirmwareThatIsNotMeshCoreIsNotProvisioned(t *testing.T) {
	n := deckWith(t, "wadamesh", "wadamesh")
	got := (&Sim{}).provisionLines(n)
	if len(got) != 1 || !got[0].Comment || !strings.Contains(got[0].Command, "wadamesh firmware") {
		t.Fatalf("provisioning for a wadamesh build: %+v", got)
	}

	plain := deckWith(t, "v1.17.1", "")
	if got := (&Sim{}).provisionLines(plain); len(got) < 2 {
		t.Errorf("a MeshCore build lost its provisioning: %+v", got)
	}
}

// An arm that pins a setting onto such a node is refused before the run; an
// arm that only chooses builds is not.
func TestAnArmPinningSettingsOntoForeignFirmwareIsRefused(t *testing.T) {
	n := deckWith(t, "wadamesh", "none")
	s := &Sim{}
	mode := 1
	why := s.RefuseUnprovisionable([]ExpArm{{Label: "two-byte", PathHashMode: &mode}}, []scenario.Node{n})
	if !strings.Contains(why, `arm "two-byte"`) || !strings.Contains(why, "Deck (none)") {
		t.Errorf("refusal: %q", why)
	}
	if why := s.RefuseUnprovisionable([]ExpArm{{Label: "builds", RepeaterVersion: "v1.17.1"}}, []scenario.Node{n}); why != "" {
		t.Errorf("an arm pinning no settings was refused: %q", why)
	}
}

// Settings belong to one image, so a repeater of the same version on the same
// board is not taken for the companion that declared a dialect.
func TestADialectBelongsToItsOwnRole(t *testing.T) {
	deck := deckWith(t, "wadamesh", "wadamesh")
	if d := dialectOf(deck, firmware.ListInstalled(firmware.DefaultCacheDir())); d != "wadamesh" {
		t.Fatalf("the companion itself: %q", d)
	}
	dir := filepath.Join(firmware.DefaultCacheDir(), "board", "LilyGo_TDeck")
	if err := os.WriteFile(filepath.Join(dir, "simple_repeater@wadamesh.bin"), make([]byte, 64), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := deck
	rep.Kind, rep.Firmware.Role = scenario.SimpleRepeater, scenario.RoleSimpleRepeater
	if d := dialectOf(rep, firmware.ListInstalled(firmware.DefaultCacheDir())); d != "" {
		t.Errorf("a repeater image with no settings of its own was given the companion's: %q", d)
	}
	generic := deck
	generic.Firmware.Role = "companion_radio"
	if d := dialectOf(generic, firmware.ListInstalled(firmware.DefaultCacheDir())); d != "wadamesh" {
		t.Errorf("a node naming the role without its transport: %q", d)
	}
}
