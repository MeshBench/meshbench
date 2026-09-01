// Putting one board back to factory.
//
// firmware.wipe has always been every node at once, which was the only
// granularity that made sense while an emulated node's flash was rewritten on
// every start anyway. Now that a board keeps what it was told between runs,
// "put this one back and leave the others" is a question somebody actually
// has: a node configured into a state it will not come out of, in a network of
// forty that are fine.
package nodewipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

func registerNodeWipe(st *state.Store, s *session.Sim) {
	st.HandleSpec("node.wipe", state.Spec{
		What: "erase one node's stored state: its flash, its card and its files",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node to put back to factory"},
			{Name: "confirm", Type: state.ParamBool,
				What: "false lists what would go and removes nothing; " +
					"absent or true erases it"},
		},
		Returns: []string{"node", "wiped", "removed", "would_remove"},
		Answers: "`wiped` counts what went and `removed` names it, the node's " +
			"card included where it keeps one elsewhere; the emulator's own " +
			"sockets are left, being recreated at the next start. With `confirm` " +
			"false nothing is touched and `would_remove` names what a wipe would " +
			"take. A node with nothing on disk answers zero rather than refusing. " +
			"Refused where the node is not an emulated board, where it is still " +
			"running, and where only part of it could be removed - a partial " +
			"wipe is an error naming what stayed, because a board that boots " +
			"back into settings said to be gone is worse than one that was never " +
			"wiped.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "confirm": false},
			What:   "see what putting a board back to factory would take",
		},
	}, func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "node")
		if name == "" {
			return nil, session.BadParams("node.wipe needs a node")
		}
		if _, err := s.NodeIsEmulated(w, name); err != nil {
			return nil, err
		}
		// A destructive verb with no way to look first is one nobody can check
		// a script against. Absent still wipes, because the clients already
		// spell this as node.wipe() and a parameter that appears today cannot
		// be required of callers written yesterday - but a caller who passes
		// confirm:false is asking not to destroy anything, and honouring that
		// is the whole difference between a parameter and a decoration.
		if ok, given := session.BoolField(p, "confirm"); given && !ok {
			return dryWipe(name)
		}
		// Refused while it is running, rather than racing the emulator for its
		// own flash file. A wipe that half-succeeded against a live machine
		// would leave a chip whose partition table and contents disagree, and
		// the board would fail to boot for a reason nothing recorded.
		if s.Engine() != nil {
			if n, ok := s.Engine().NodeByName(name); ok && n.Firmware != nil {
				return nil, fmt.Errorf(
					"%s is running: stop it before wiping, or its flash would be "+
						"rewritten underneath the emulator", name)
			}
		}
		dir := firmware.NodeWorkDir(name)
		removed, kept := []string{}, []string{}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"node": name, "wiped": 0, "removed": removed}, nil
			}
			return nil, err
		}
		for _, e := range entries {
			// The sockets are the emulator's, recreated on the next start, and
			// removing a live one would be removing something that is not this
			// node's memory at all.
			if strings.HasSuffix(e.Name(), ".sock") {
				continue
			}
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err == nil {
				removed = append(removed, e.Name())
			} else {
				kept = append(kept, e.Name())
			}
		}
		// And its card, where it keeps one somewhere other than here. A node
		// put back to factory with its storage intact is not back to factory:
		// the firmware finds its old settings on the card and nothing says
		// why.
		if i, ok := s.NodeIndex(name); ok && s.Nodes()[i].CardFile != "" {
			card := s.Nodes()[i].CardFile
			if err := os.Remove(card); err == nil {
				removed = append(removed, filepath.Base(card))
			} else if !os.IsNotExist(err) {
				kept = append(kept, filepath.Base(card))
			}
		}
		// A partial wipe is reported as partial. The failures used to be
		// dropped on the floor, so a node whose flash image could not be
		// removed answered "wiped" and booted next time into the settings the
		// operator had just been told were gone.
		if len(kept) > 0 {
			return nil, fmt.Errorf(
				"%s is only partly wiped: %d removed, but %s could not be - "+
					"it is not back to factory", name, len(removed),
				strings.Join(kept, ", "))
		}
		w.Say("wiped " + name + "'s stored settings")
		return map[string]any{"node": name, "wiped": len(removed), "removed": removed}, nil
	})
}

// dryWipe answers what a wipe would take, without taking it.
func dryWipe(name string) (any, error) {
	dir := firmware.NodeWorkDir(name)
	would := []string{}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sock") {
			would = append(would, e.Name())
		}
	}
	return map[string]any{"node": name, "wiped": 0, "removed": []string{},
		"would_remove": would}, nil
}
