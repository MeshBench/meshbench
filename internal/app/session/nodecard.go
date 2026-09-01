// What is in a node's card slot.
//
// A slot is not a fitted card. The board profile can only say the slot exists;
// whether this particular handheld has storage in it, and which file that
// storage is, is the scenario's business - two of the same board in one
// network, one with a card and one without, is an ordinary thing to want.
//
// The exception is a firmware that keeps its settings on the card. That build
// gets one whatever the node would otherwise have had, because the alternative
// is a boot failure several minutes into a run, in a message that does not
// mention cards.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerNodeCard(st *state.Store, s *Sim) {
	st.HandleSpec("node.card", state.Spec{
		What: "Report or change what is in one node's card slot: whether a card is fitted, which file it is, and whether to erase it.",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "which node"},
			{Name: "fitted", Type: state.ParamBool,
				What: "put a card in the slot or take it out; unchanged when absent"},
			{Name: "file", Type: state.ParamString,
				What: "the file behind the card; empty string returns it to the " +
					"node's own, named after the node and kept beside its flash"},
			{Name: "wipe", Type: state.ParamBool,
				What: "erase the card, which is what reformatting one is"},
		},
		Returns: []string{"node", "slot", "fitted", "file", "own_file", "bytes",
			"required_by_firmware", "board_has_slot", "wiped"},
	}, func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, badParams("node.card needs a node")
		}
		i, found := s.nodeIndex(name)
		if !found {
			return nil, noSuchNode(name)
		}
		hasSlot := boardHasCardSlot(s.nodes[i].Firmware.Board)

		if on, ok := boolOf(p, "fitted"); ok {
			if !hasSlot {
				return nil, badParams(
					"%s is a %s, which has no card slot to put one in", name,
					boardOrMachine(s.nodes[i].Firmware.Board))
			}
			s.nodes[i].Card = scenario.CardFitted
			if !on {
				s.nodes[i].Card = scenario.CardEmpty
			}
		}
		if file, ok := namedField(p, "file"); ok {
			// Refused rather than accepted and half-used: a card is a file the
			// emulator writes to for the whole run, and a directory or an
			// unwritable path fails at start rather than here.
			if err := usableAsCard(file); err != nil {
				return nil, badParams("%s", err.Error())
			}
			s.nodes[i].CardFile = file
		}
		if s.eng != nil {
			s.eng.SetCard(name, s.nodes[i].Card == scenario.CardFitted, s.nodes[i].CardFile)
		}

		file := cardFileFor(s.nodes[i])
		wiped := false
		if yes, ok := boolOf(p, "wipe"); ok && yes {
			if err := s.refuseWhileNodeRuns(name, "wiping its card"); err != nil {
				return nil, err
			}
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			wiped = true
			w.Say("erased " + name + "'s card")
		}
		required := s.firmwareRequiresCard(s.nodes[i])
		s.publishCards(w)
		out := map[string]any{
			"node": name, "slot": string(s.nodes[i].Card),
			"fitted":               s.nodes[i].HasCard(hasSlot, required),
			"file":                 file,
			"own_file":             filepath.Join(firmware.NodeWorkDir(name), "card.img"),
			"required_by_firmware": required, "board_has_slot": hasSlot,
			"wiped": wiped,
		}
		if st, err := os.Stat(file); err == nil {
			out["bytes"] = st.Size()
		} else {
			out["bytes"] = int64(0)
		}
		return out, nil
	})
}

// cardFileFor is the file behind a node's card: its own unless it was handed
// another.
func cardFileFor(n scenario.Node) string {
	if n.CardFile != "" {
		return n.CardFile
	}
	return filepath.Join(firmware.NodeWorkDir(n.Name), "card.img")
}

// usableAsCard turns away a path that cannot be one, or that is not ours.
func usableAsCard(file string) error {
	if strings.TrimSpace(file) == "" {
		return nil // back to the node's own
	}
	if !filepath.IsAbs(file) {
		return fmt.Errorf(
			"%s is a relative path, and a card outlives the directory the "+
				"workbench happened to be started in - give it in full", file)
	}
	// Confined to the storage this session owns, because this path is not only
	// read from. `wipe` deletes it, and starting the node reformats whatever is
	// there. An absolute path taken on trust is therefore a delete of anything
	// the workbench can reach, asked for through a verb whose subject is a
	// memory card - and it arrives over a control socket that scripts drive.
	if err := underSessionStorage("a card", file); err != nil {
		return err
	}
	if st, err := os.Stat(file); err == nil && st.IsDir() {
		return fmt.Errorf("%s is a directory, not a card", file)
	}
	return nil
}

// underSessionStorage refuses a path outside the node filesystem root.
//
// The root is where every node's flash, settings and own card already live, so
// it is the one directory the workbench is entitled to erase files in. Named in
// the refusal rather than left implicit: somebody handed a card from their
// downloads folder needs to be told where to put it, not only that they were
// wrong.
func underSessionStorage(what, file string) error {
	root, err := filepath.Abs(firmware.NodeFSRoot())
	if err != nil {
		return fmt.Errorf("cannot locate the node storage directory: %w", err)
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return fmt.Errorf("%s is not a usable path: %w", file, err)
	}
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return fmt.Errorf(
			"%s is outside %s, and %s is erased and reformatted where it "+
				"stands - keep it under the node storage directory", file, root, what)
	}
	return nil
}

// boardHasCardSlot reports whether this board declares one.
func boardHasCardSlot(board string) bool {
	if board == "" {
		return false
	}
	b, err := hw.BoardByName(board)
	if err != nil || b.Hardware == nil {
		return false
	}
	for _, part := range b.Hardware.PartsOfKind(hw.Card) {
		if part.Pin != hw.PinNone {
			return true
		}
	}
	return false
}

func boardOrMachine(board string) string {
	if board == "" {
		return "build for this machine"
	}
	return board
}

// firmwareRequiresCard reports whether the build this node runs insists on
// storage.
func (s *Sim) firmwareRequiresCard(n scenario.Node) bool {
	if n.Firmware.Board == "" || n.Firmware.Version == "" {
		return false
	}
	for _, in := range firmware.ListInstalled(firmware.DefaultCacheDir()) {
		if in.Board == n.Firmware.Board && in.Version == n.Firmware.Version {
			if firmware.LoadBuildSettings(in.Path).CardRequired {
				return true
			}
		}
	}
	return false
}

// nodeIndex is where a node sits in the scenario, by name.
func (s *Sim) nodeIndex(name string) (int, bool) {
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			return i, true
		}
	}
	return 0, false
}

// refuseWhileNodeRuns turns away a change to a file the emulator has open.
func (s *Sim) refuseWhileNodeRuns(name, what string) error {
	if s.eng == nil {
		return nil
	}
	if n, ok := s.eng.NodeByName(name); ok && n.Firmware != nil {
		return fmt.Errorf("%s is running: stop it before %s, or the emulator "+
			"would go on writing to a file that is no longer there", name, what)
	}
	return nil
}

// wipeCardsOutside erases the cards that a directory-wide wipe would miss.
//
// Only the ones somebody pointed somewhere else, and only the files: a card
// given a path is still the node's storage, but it is not under the root that
// firmware.wipe empties. Failures are counted out rather than raised - one
// unwritable card is not a reason to report that nothing was wiped.
func (s *Sim) wipeCardsOutside(root string) int {
	n := 0
	for i := range s.nodes {
		file := s.nodes[i].CardFile
		if file == "" {
			continue // the node's own, already gone with its directory
		}
		if abs, err := filepath.Abs(file); err == nil &&
			strings.HasPrefix(abs, root+string(os.PathSeparator)) {
			continue
		}
		if err := os.Remove(file); err == nil {
			n++
		}
	}
	return n
}

// buildsNeedingCards is which builds in the cache insist on storage.
//
// Read once per pass rather than once per node: a network of forty handhelds
// would otherwise walk the whole cache forty times to answer one line of a
// panel.
func buildsNeedingCards() map[string]bool {
	out := map[string]bool{}
	for _, in := range firmware.ListInstalled(firmware.DefaultCacheDir()) {
		if firmware.LoadBuildSettings(in.Path).CardRequired {
			out[in.Board+"\x00"+in.Version] = true
		}
	}
	return out
}

// publishCards puts each node's card slot into the world the panels read.
//
// Called where it can change rather than every tick: the answer costs a walk
// of the firmware cache and a stat per node, and the only things that move it
// are a card being changed, a build being changed, and a node being pointed at
// a different build.
func (s *Sim) publishCards(w *state.World) {
	need := buildsNeedingCards()
	for i := range w.Nodes {
		j, ok := s.nodeIndex(w.Nodes[i].Name)
		if !ok {
			continue
		}
		n := s.nodes[j]
		slot := boardHasCardSlot(n.Board)
		required := need[n.Firmware.Board+"\x00"+n.Firmware.Version]
		w.Nodes[i].CardSlot = slot
		w.Nodes[i].CardRequired = required && slot
		w.Nodes[i].CardFitted = n.HasCard(slot, required)
		w.Nodes[i].CardFile = cardFileFor(n)
		w.Nodes[i].CardShared = n.CardFile != ""
	}
}
