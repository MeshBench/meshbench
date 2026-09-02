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
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerFirmwareNodes(st *state.Store, s *Sim) {
	st.Handle("firmware.set", func(w *state.World, p any) (any, error) {
		version, _ := stringField(p, "version")
		node, _ := namedField(p, "node")
		role, _ := namedField(p, "role")
		if version == "" {
			return nil, fmt.Errorf("firmware.set needs a version")
		}
		// The board travels with the version here as it does on
		// node.set_firmware, because a board image is not a build on its own:
		// it is that image for that hardware. Dropped, it pinned a board node
		// to a host build and the start then failed asking for a native build
		// of a version that has none, which sends the reader to build MeshCore
		// from source over a field they had passed and we had ignored.
		//
		// Presence, not emptiness, and this is the one place it differs from
		// the single-node verb. There, an absent board means a host build and
		// clears whatever the node had. Here the same rule would make
		// `firmware.set {"version": ...}` across a mesh silently convert every
		// emulated node to native, which is the fault above with the sign
		// flipped and three hundred nodes behind it. So absent leaves the
		// board alone, and an explicit empty string is how a node is moved
		// back to a build for this machine.
		m, _ := p.(map[string]any)
		board, setBoard := m["board"].(string)
		n := s.updateNodes(w, func(n *scenario.Node, row *state.Node) bool {
			if node != "" && n.Name != node {
				return false
			}
			// The role a node runs under, not the role it has been pinned
			// to: a node with no build chosen yet has an empty one, and
			// those are exactly the nodes being asked about.
			if role != "" && nodeRole(*n) != role {
				return false
			}
			n.Firmware.Version = version
			if setBoard {
				n.Firmware.Board = board
			}
			// And the engine, which holds its own copy of every spec and is
			// the one that actually starts a process. Without this the
			// library, the row and the message all agree with each other and
			// the run asks for whatever the network was opened with.
			if s.eng != nil {
				s.eng.PinFirmware(n.Name, version)
				if setBoard {
					// The role is the filter here, not a value to write, so
					// the node keeps the one it has.
					s.eng.PinBoard(n.Name, board, "")
				}
			}
			// And the row, decided here rather than by a second walk with a
			// filter of its own: the one that used to be here honoured the
			// node name and ignored the role, so pinning a build to the
			// repeaters drew the whole mesh as running it.
			if row != nil {
				row.Firmware = version
			}
			return true
		})
		said := version
		if setBoard && board != "" {
			said += " on " + board
		}
		w.Say(fmt.Sprintf("%d nodes pinned to %s", n, said))
		out := map[string]any{
			"version": version, "nodes": n, "considered": len(s.nodes),
		}
		// Echoed only when it was asked for, so a caller can tell "left the
		// board alone" from "set it to a host build".
		if setBoard {
			out["board"] = board
		}
		return out, nil
	})

	st.Handle("firmware.wipe", func(w *state.World, _ any) (any, error) {
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
