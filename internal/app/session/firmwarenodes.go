// What each node runs, and what it remembers between runs.
//
// The two verbs an operator reaches for either side of a comparison, and the
// only two here that write to a node rather than to the library: one decides
// which build a node starts, the other takes away everything it learned last
// time so a changed default is actually reached.
package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerFirmwareNodes(st *state.Store, s *Sim) {
	st.HandleSpec("firmware.set", state.Spec{
		What: "pin a build to nodes, which is what decides what each one starts " +
			"and is the step a run that will not start is usually missing",
		Params: []state.Param{
			{Name: "version", Type: state.ParamString, Required: true, Primary: true,
				What: "the version or imported label to pin; refused when absent, " +
					"but not checked against the library, so a name nothing " +
					"answers to is only found out at the next start"},
			{Name: "node", Type: state.ParamString,
				What: "pin this one node by name; absent means every node the " +
					"role filter leaves"},
			{Name: "role", Type: state.ParamString,
				What: "only nodes running under this role, pinned or implied by " +
					"their kind; absent means all of them"},
		},
		Returns: []string{"version", "nodes", "considered"},
		Answers: "`nodes` is how many were pinned and `considered` how many " +
			"exist, which counts the ones that never run firmware. With a " +
			"`role` and no `node` it pins every node running that role, but " +
			"marks every node in the fleet list as running the version whatever " +
			"its role, so a call per role leaves the list reading as the last " +
			"one: pass `node` to pin exactly one.",
		Example: &state.Example{
			Params: map[string]any{
				"node": "West Lomond", "version": "repeater-v1.16.0",
			},
			What:     "pin one node to the build it will start",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		version, _ := stringField(p, "version")
		node, _ := namedField(p, "node")
		role, _ := namedField(p, "role")
		if version == "" {
			return nil, fmt.Errorf("firmware.set needs a version")
		}
		n := 0
		for i := range s.nodes {
			if node != "" && s.nodes[i].Name != node {
				continue
			}
			// The role a node runs under, not the role it has been pinned
			// to: a node with no build chosen yet has an empty one, and
			// those are exactly the nodes being asked about.
			if role != "" && nodeRole(s.nodes[i]) != role {
				continue
			}
			s.nodes[i].Firmware.Version = version
			// And the engine, which holds its own copy of every spec and is
			// the one that actually starts a process. Without this the
			// library, the row and the message all agree with each other and
			// the run asks for whatever the network was opened with.
			if s.eng != nil {
				s.eng.PinFirmware(s.nodes[i].Name, version)
			}
			n++
		}
		for i := range w.Nodes {
			if node == "" || w.Nodes[i].Name == node {
				w.Nodes[i].Firmware = version
			}
		}
		w.Say(fmt.Sprintf("%d nodes pinned to %s", n, version))
		return map[string]any{
			"version": version, "nodes": n, "considered": len(s.nodes),
		}, nil
	})

	st.HandleSpec("firmware.wipe", state.Spec{
		What: "clear every node's persistent files, which is what belongs " +
			"between the arms of a comparison: a node keeps its preferences " +
			"between runs exactly as hardware does, so one that has run before " +
			"loads its old settings, never reaches a changed default, and " +
			"returns the same numbers as the arm before it",
		Returns: []string{"wiped", "root", "cards"},
		Answers: "`wiped` counts the node directories removed under `root`, and " +
			"`cards` the storage images a node was given somewhere else, which " +
			"are wiped too or the claim would be a lie. A storage directory " +
			"that does not exist yet is not a failure: it answers `wiped` 0 and " +
			"nothing else.",
		Example: &state.Example{
			Params: map[string]any{},
			What:   "put every node back to factory before the next arm of a study",
		},
	}, func(w *state.World, _ any) (any, error) {
		root := nodeStorageRoot()
		if root == "" {
			return nil, fmt.Errorf("no node storage directory")
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				return map[string]any{"wiped": 0}, nil
			}
			return nil, err
		}
		n := 0
		for _, e := range entries {
			if err := os.RemoveAll(filepath.Join(root, e.Name())); err == nil {
				n++
			}
		}
		// And the cards kept somewhere else. A node handed a card of its own
		// choosing has its storage outside this directory, so wiping the
		// directory leaves it holding everything it was supposed to forget -
		// which is the one case where "wiped every node" would be a lie.
		cards := s.wipeCardsOutside(root)
		w.Say(fmt.Sprintf("wiped %d nodes' stored settings and %d cards kept elsewhere",
			n, cards))
		return map[string]any{"wiped": n, "root": root, "cards": cards}, nil
	})
}

// nodeStorageRoot is where nodes keep what they remember between runs.
func nodeStorageRoot() string {
	if v := os.Getenv("MESHBENCH_NODEFS"); v != "" {
		return v
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "meshbench", "nodefs")
}
