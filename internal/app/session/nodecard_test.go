package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// aCardSim is a session holding one T-Deck, which has a slot, and one Heltec,
// which has not.
func aCardSim(t *testing.T) (*state.Store, *Sim) {
	t.Helper()
	t.Setenv(firmware.EnvNodeFS, t.TempDir())
	// A firmware cache of this test's own. Without it the test reads the one
	// on the machine running it, and a build there marked as needing a card
	// fills this node's slot - which is the feature working and the test
	// failing, on one machine and not another.
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", home)
	t.Setenv("LOCALAPPDATA", home)
	t.Setenv("HOME", home)
	st := state.New(10)
	s := &Sim{nodes: []scenario.Node{
		{Name: "Deck", Kind: scenario.Companion, Board: "LilyGo_TDeck",
			Firmware: scenario.FirmwareRef{Version: "mesh-rs", Board: "LilyGo_TDeck",
				Role: scenario.RoleCompanionRadioUSB}},
		{Name: "V3", Kind: scenario.SimpleRepeater, Board: "Heltec_v3"},
	}}
	registerNodeCard(st, s)
	st.Handle("test.nodes", func(w *state.World, p any) (any, error) {
		w.Nodes = p.([]state.Node)
		return nil, nil
	})
	go st.Run(t.Context())
	if _, err := st.Do(t.Context(), "test.nodes", []state.Node{
		{Name: "Deck"}, {Name: "V3"},
	}); err != nil {
		t.Fatal(err)
	}
	return st, s
}

// A slot is not a fitted card, and a board with no slot has nothing to fit.
func TestACardCanBeTakenOutAndPutBack(t *testing.T) {
	st, s := aCardSim(t)
	ctx := t.Context()

	got, err := st.Do(ctx, "node.card", "Deck")
	if err != nil {
		t.Fatalf("node.card: %v", err)
	}
	m := got.(map[string]any)
	if m["board_has_slot"] != true || m["fitted"] != true {
		t.Errorf("a T-Deck starts with %v / %v, want a slot and a card",
			m["board_has_slot"], m["fitted"])
	}
	// Its own file, named after it and beside its flash.
	if want := filepath.Join(firmware.NodeWorkDir("Deck"), "card.img"); m["file"] != want {
		t.Errorf("its card is %v, want %s", m["file"], want)
	}

	got, err = st.Do(ctx, "node.card", map[string]any{"node": "Deck", "fitted": false})
	if err != nil {
		t.Fatal(err)
	}
	if got.(map[string]any)["fitted"] != false {
		t.Error("taking the card out left one in the slot")
	}
	if s.nodes[0].Card != scenario.CardEmpty {
		t.Errorf("the scenario says %q", s.nodes[0].Card)
	}

	// A board with no slot is refused rather than given an invisible card.
	if _, err := st.Do(ctx, "node.card",
		map[string]any{"node": "V3", "fitted": true}); err == nil {
		t.Error("a board with no slot accepted a card")
	} else if !strings.Contains(err.Error(), "no card slot") {
		t.Errorf("refused with %v, which does not say why", err)
	}
}

// A card somebody else prepared, and the way back to the node's own. It lives
// in the node storage the session owns, because that is the one place this verb
// is allowed to erase and reformat a file.
func TestANodeCanBeHandedACardOfItsOwnChoosing(t *testing.T) {
	st, s := aCardSim(t)
	ctx := t.Context()
	mine := filepath.Join(firmware.NodeFSRoot(), "shared.img")

	got, err := st.Do(ctx, "node.card", map[string]any{"node": "Deck", "file": mine})
	if err != nil {
		t.Fatalf("node.card: %v", err)
	}
	if got.(map[string]any)["file"] != mine {
		t.Errorf("it is using %v", got.(map[string]any)["file"])
	}

	// A relative path is refused: a card outlives the directory the workbench
	// happened to be started in.
	if _, err := st.Do(ctx, "node.card",
		map[string]any{"node": "Deck", "file": "cards/x.img"}); err == nil {
		t.Error("a relative card path was accepted")
	}

	if _, err := st.Do(ctx, "node.card",
		map[string]any{"node": "Deck", "file": ""}); err != nil {
		t.Fatal(err)
	}
	if s.nodes[0].CardFile != "" {
		t.Errorf("it did not go back to its own: %q", s.nodes[0].CardFile)
	}
}

// A card is deleted and reformatted where it stands, so an absolute path
// outside the session's own storage is a delete of anything the workbench can
// reach, asked for over a socket that scripts drive.
func TestACardOutsideTheSessionsStorageIsRefused(t *testing.T) {
	st, s := aCardSim(t)
	ctx := t.Context()
	theirs := filepath.Join(t.TempDir(), "not-ours.img")
	if err := os.WriteFile(theirs, []byte("somebody else's"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := st.Do(ctx, "node.card", map[string]any{"node": "Deck", "file": theirs})
	if err == nil {
		t.Fatal("a card outside the node storage directory was accepted")
	}
	if !strings.Contains(err.Error(), firmware.NodeFSRoot()) {
		t.Errorf("refused with %v, which does not say where cards may live", err)
	}
	if s.nodes[0].CardFile != "" {
		t.Errorf("it took the path anyway: %q", s.nodes[0].CardFile)
	}
	// And, having refused, it must not have gone near the file.
	if _, err := os.Stat(theirs); err != nil {
		t.Errorf("the refused file was touched: %v", err)
	}

	// The escape by relative segments, which an absolute path can still spell.
	up := filepath.Join(firmware.NodeFSRoot(), "..", "escaped.img")
	if _, err := st.Do(ctx, "node.card",
		map[string]any{"node": "Deck", "file": up}); err == nil {
		t.Error("a path climbing out of the node storage directory was accepted")
	}
}

// Erasing a card is what reformatting one is, and it is the node's storage
// wherever the node keeps it.
func TestErasingACardRemovesTheFileWhereverItIs(t *testing.T) {
	st, s := aCardSim(t)
	ctx := t.Context()
	mine := filepath.Join(firmware.NodeFSRoot(), "shared.img")
	if err := os.WriteFile(mine, []byte("settings"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Do(ctx, "node.card", map[string]any{"node": "Deck", "file": mine}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Do(ctx, "node.card", map[string]any{"node": "Deck", "wipe": true})
	if err != nil {
		t.Fatalf("node.card: %v", err)
	}
	if got.(map[string]any)["wiped"] != true {
		t.Error("it did not report erasing anything")
	}
	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Error("the card is still there")
	}
	_ = s
}

// firmware.wipe empties the node directories, which is every card kept in one.
// A card kept somewhere else would survive that, and "wiped every node" would
// be a lie.
func TestWipingEveryNodeTakesTheCardsKeptElsewhere(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MESHBENCH_NODEFS", root)
	outside := filepath.Join(t.TempDir(), "elsewhere.img")
	if err := os.WriteFile(outside, []byte("settings"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Sim{nodes: []scenario.Node{
		{Name: "Deck", Board: "LilyGo_TDeck", CardFile: outside},
	}}
	if n := s.wipeCardsOutside(root); n != 1 {
		t.Fatalf("wiped %d cards kept elsewhere, want 1", n)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Error("the card outside the node directories survived")
	}
	// A node using its own card is not double-counted: it goes with its
	// directory, and removing it here would report it twice.
	s.nodes[0].CardFile = ""
	if n := s.wipeCardsOutside(root); n != 0 {
		t.Errorf("counted %d cards that were already gone with their directory", n)
	}
}
