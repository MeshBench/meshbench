package session

import "github.com/MeshBench/meshbench/internal/app/state"

// showConsole makes a node's console the one being looked at, and keeps it in
// the list every window draws from.
//
// The single Console/ConsoleNode pair stays for the socket's snapshot and the
// verbs that read it. The list is what the windows draw: with only the pair,
// a second window on a second node said the first "has the console" and showed
// nothing, though every line it would have shown was in its buffer.
func showConsole(w *state.World, node string, lines []string) {
	w.Console, w.ConsoleNode = lines, node
	putConsole(w, node, lines)
}

// putConsole lands one node's console in the list, replacing what that node's
// pane held and leaving every other pane alone.
func putConsole(w *state.World, node string, lines []string) {
	for i := range w.Consoles {
		if w.Consoles[i].Node == node {
			w.Consoles[i].Lines = lines
			return
		}
	}
	if len(w.Consoles) >= maxConsolePanes {
		// A bound, because every pane is re-read on every tick. The oldest
		// goes; its window asks again by being typed at or reopened.
		w.Consoles = w.Consoles[1:]
	}
	w.Consoles = append(w.Consoles, state.ConsolePane{Node: node, Lines: lines})
}

// maxConsolePanes is how many node consoles are re-read per tick.
const maxConsolePanes = 12

// refreshConsoles re-reads every console a window is showing, because replies
// land in a node's buffer when its firmware runs, long after the command that
// asked for them.
//
// A pane whose node had no firmware when its window opened is attached here
// once it has, so a window opened before the run started fills in when the
// node boots rather than waiting to be typed at.
func (s *Sim) refreshConsoles(w *state.World) {
	for i := range w.Consoles {
		node := w.Consoles[i].Node
		if _, comp := s.comps[node]; !comp {
			if _, seen := s.consoles[node]; !seen {
				if _, err := s.consoleFor(node); err != nil {
					continue
				}
			}
		}
		w.Consoles[i].Lines = s.consoleLines(node, w.Consoles[i].Lines)
	}
}
