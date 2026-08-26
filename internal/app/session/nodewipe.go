// Putting one board back to factory.
//
// firmware.wipe has always been every node at once, which was the only
// granularity that made sense while an emulated node's flash was rewritten on
// every start anyway. Now that a board keeps what it was told between runs,
// "put this one back and leave the others" is a question somebody actually
// has: a node configured into a state it will not come out of, in a network of
// forty that are fine.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/mesh/firmware"
)

func registerNodeWipe(st *state.Store, s *Sim) {
	st.HandleSpec("node.wipe", state.Spec{
		What: "erase one node's stored state: its flash, its card and its files",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node to put back to factory"},
		},
		Returns: []string{"node", "wiped", "removed"},
	}, func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, badParams("node.wipe needs a node")
		}
		if _, err := s.nodeIsEmulated(w, name); err != nil {
			return nil, err
		}
		// Refused while it is running, rather than racing the emulator for its
		// own flash file. A wipe that half-succeeded against a live machine
		// would leave a chip whose partition table and contents disagree, and
		// the board would fail to boot for a reason nothing recorded.
		if s.eng != nil {
			if n, ok := s.eng.NodeByName(name); ok && n.Firmware != nil {
				return nil, fmt.Errorf(
					"%s is running: stop it before wiping, or its flash would be "+
						"rewritten underneath the emulator", name)
			}
		}
		dir := firmware.NodeWorkDir(name)
		removed := []string{}
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
			}
		}
		// And its card, where it keeps one somewhere other than here. A node
		// put back to factory with its storage intact is not back to factory:
		// the firmware finds its old settings on the card and nothing says
		// why.
		if i, ok := s.nodeIndex(name); ok && s.nodes[i].CardFile != "" {
			if err := os.Remove(s.nodes[i].CardFile); err == nil {
				removed = append(removed, filepath.Base(s.nodes[i].CardFile))
			}
		}
		w.Say("wiped " + name + "'s stored settings")
		return map[string]any{"node": name, "wiped": len(removed), "removed": removed}, nil
	})
}
