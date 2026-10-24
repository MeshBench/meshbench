// Attaching a debugger to a node's firmware (#191).
//
// A native node is an ordinary process, so gdb, lldb and an IDE already work
// on it; what was missing was which process, whether its binary has symbols,
// and a run that does not mark the node down the moment a breakpoint stops it.
// An emulated board's firmware is inside the emulator, and the way in is the
// emulator's GDB stub, which only exists if the board was started with one.
package session

import (
	"context"
	"debug/elf"
	"fmt"
	"net"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
	"github.com/MeshBench/meshbench/internal/firmware/native"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

func registerNodeDebug(st *state.Store, s *Sim) {
	st.Handle("node.debug", func(w *state.World, p any) (any, error) {
		name := primaryString(p, "node")
		if s.eng == nil {
			return nil, ErrNoSimulation
		}
		n, ok := s.eng.NodeByName(name)
		if !ok {
			return nil, noSuchNode(name)
		}
		if n.Firmware == nil {
			return nil, fmt.Errorf("%s is not running firmware, so there is nothing to debug", name)
		}
		off, _ := boolField(p, "off")
		wait, _ := boolField(p, "wait")
		port := 0
		if v, ok := namedNum(p, "port"); ok {
			port = int(v)
		}
		switch b := n.Firmware.Backend.(type) {
		case *native.Native:
			if off {
				s.eng.SetDebug(name, nil)
				return map[string]any{"node": name, "debugging": false}, nil
			}
			if wait {
				return nil, fmt.Errorf("%s is native: wait holds an emulated board at reset, and a "+
					"native node has already run setup() by the time it can be attached to", name)
			}
			s.eng.SetDebug(name, &engine.Debug{})
			s.taint(name + " is being debugged, and a debugger stopping it moves its clock")
			out := nativeDebug(name, b)
			w.Say(fmt.Sprintf("%s can be debugged, and a breakpoint holds it while the run goes on: %s",
				name, out["attach"]))
			return out, nil
		case *emulated.EmulatedNode:
			if off {
				s.eng.SetDebug(name, nil)
				if err := s.restartNode(name, w.Seed); err != nil {
					return nil, err
				}
				return map[string]any{"node": name, "debugging": false}, nil
			}
			if port == 0 {
				free, err := freePort()
				if err != nil {
					return nil, err
				}
				port = free
			}
			s.eng.SetDebug(name, &engine.Debug{GDBPort: port, Wait: wait})
			s.taint(name + " is being debugged, and a debugger stopping it moves its clock")
			// The stub is part of how the emulator is started, so the board
			// starts again with it.
			if err := s.restartNode(name, w.Seed); err != nil {
				s.eng.SetDebug(name, nil)
				return nil, err
			}
			n, _ = s.eng.NodeByName(name)
			em, _ := n.Firmware.Backend.(*emulated.EmulatedNode)
			if em == nil {
				return nil, fmt.Errorf("%s did not come back as an emulated board", name)
			}
			out := emulatedDebug(name, em)
			w.Say(fmt.Sprintf("%s restarted with a GDB stub on port %d: %s", name, port, out["attach"]))
			return out, nil
		default:
			return nil, fmt.Errorf("%s runs on a backend nothing can attach a debugger to", name)
		}
	})
}

func (s *Sim) restartNode(name string, seed uint64) error {
	if err := s.stopNode(name); err != nil {
		return err
	}
	return s.startNode(context.Background(), name, seed)
}

// nativeDebug is how to attach to a native node: its process, its binary and
// whether that binary can be read at source level.
func nativeDebug(name string, b *native.Native) map[string]any {
	pid := b.PID()
	syms := symbols(b.Path)
	out := map[string]any{
		"node": name, "debugging": true, "backend": "native",
		"pid": pid, "binary": b.Path, "symbols": syms,
		"attach": fmt.Sprintf("gdb -p %d %s", pid, b.Path),
		"vscode": map[string]any{
			"name": "MeshBench " + name, "type": "cppdbg", "request": "attach",
			"program": b.Path, "processId": fmt.Sprint(pid), "MIMode": "gdb",
		},
		"note": "a breakpoint holds this node and the rest of the run goes on; when it is let go it " +
			"catches up to the run's clock. On Linux, attaching needs ptrace permission: " +
			"kernel.yama.ptrace_scope 0, or the debugger run as root",
	}
	if syms == "stripped" {
		out["note"] = out["note"].(string) + ". This binary has no symbols: build the firmware " +
			"with meshbench dev from a checkout to debug it at source level"
	}
	return out
}

// emulatedDebug is how to reach an emulated board's GDB stub.
func emulatedDebug(name string, em *emulated.EmulatedNode) map[string]any {
	port, gdb := em.GDB()
	target := fmt.Sprintf("127.0.0.1:%d", port)
	return map[string]any{
		"node": name, "debugging": true, "backend": "emulated",
		"gdb_port": port, "wait": em.GDBWait, "debugger": gdb, "image": em.Image,
		"attach": fmt.Sprintf("%s -ex 'target remote %s' firmware.elf", gdb, target),
		"vscode": map[string]any{
			"name": "MeshBench " + name, "type": "cppdbg", "request": "launch",
			"program": "${workspaceFolder}/.pio/build/<env>/firmware.elf",
			"MIMode":  "gdb", "miDebuggerPath": gdb, "miDebuggerServerAddress": target,
			"cwd": "${workspaceFolder}",
		},
		"note": "the image the board runs has no symbols: point the debugger at the firmware.elf " +
			"the same build produced. With wait, the CPU is held at reset until the debugger continues",
	}
}

// symbols says whether a binary can be debugged at source level: "debug" with
// DWARF, "symbols" with a symbol table only, "stripped" with neither, or
// "unknown" when it cannot be read as ELF (a Mach-O or PE build).
func symbols(path string) string {
	f, err := elf.Open(path)
	if err != nil {
		return "unknown"
	}
	defer func() { _ = f.Close() }()
	if f.Section(".debug_info") != nil {
		return "debug"
	}
	if f.Section(".symtab") != nil {
		return "symbols"
	}
	return "stripped"
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}
