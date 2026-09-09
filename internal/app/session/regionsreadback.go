package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// registerRegionsReadback adds node.regions: read a running node's actual
// region map back from its firmware into the model.
//
// nodes.regions and infer.apply keep the model current for regions they set,
// but a region typed straight at a node's own console - console.type,
// fleet.send - lands in the firmware and never in the model, so nodes.list
// reported null for a node that held one. MeshCore's repeater console can be
// asked (region list allowed, region default), and this asks it.
func registerRegionsReadback(st *state.Store, s *Sim) {
	st.Handle("node.regions", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		if name == "" {
			return nil, fmt.Errorf("node.regions needs a node")
		}
		buf, err := s.consoleFor(name)
		if err != nil {
			return nil, err
		}
		en, ok := s.eng.NodeByName(name)
		if !ok || en.Firmware == nil {
			return nil, fmt.Errorf("%s is not running firmware, so it holds no live region map; "+
				"its regions are whatever the scenario says until it starts", name)
		}
		mark := buf.Mark()
		for _, cmd := range []string{"region list allowed", "region default"} {
			buf.Echo(cmd)
			if err := en.Firmware.Bridge.Type([]byte(cmd + "\r\n")); err != nil {
				return nil, err
			}
		}
		// The replies arrive when the engine steps. Playing, the ticker does
		// it; paused, step here as console.type does, or the read returns the
		// console as it was before the questions were asked.
		if !w.Playing {
			for i := 0; i < 60; i++ {
				_ = s.eng.Step(context.Background())
			}
			w.NowMs = s.eng.NowMs()
		}
		regions, scope, readErr := parseRegionReadback(buf.LinesSince(mark))
		if readErr != nil {
			return nil, fmt.Errorf("reading %s's regions: %w", name, readErr)
		}

		for i := range s.Nodes() {
			if s.Nodes()[i].Name == name {
				s.Nodes()[i].Regions = regions
				s.Nodes()[i].DefaultScope = scope
			}
		}
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				w.Nodes[i].Regions = regions
				w.Nodes[i].DefaultScope = scope
			}
		}
		w.Console, w.ConsoleNode = buf.Snapshot(), name
		return map[string]any{"node": name, "regions": regions, "default_scope": scope}, nil
	})
}

// parseRegionReadback pulls the region names and the default scope out of the
// console lines a node answered "region list allowed" and "region default"
// with. The list is a comma-separated line - "*,sco,fif" or "-none-" - and the
// default is " default scope is <name>" or "<null>". The wildcard is dropped:
// it is not a named region a node holds, it is the allow-anything flag.
func parseRegionReadback(lines []string) (regions []string, scope string, err error) {
	regions = []string{}
	sawList := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "default scope is"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "default scope is"))
			// "default scope is now X" comes back from a set; a bare read says
			// "default scope is X". Handle either.
			v = strings.TrimSpace(strings.TrimPrefix(v, "now"))
			if v != "" && v != "<null>" {
				scope = strings.TrimPrefix(v, "#")
			}
		case line == "-none-":
			sawList = true
		case line == "region list allowed" || line == "region default":
			// the echoes of the questions
			continue
		case strings.HasPrefix(line, "Err"):
			// "Err - ??" is what a firmware without region support answers.
			continue
		case isRegionCSV(line):
			sawList = true
			for _, tok := range strings.Split(line, ",") {
				tok = strings.TrimSpace(strings.TrimPrefix(tok, "#"))
				if tok == "" || tok == "*" {
					continue
				}
				regions = append(regions, tok)
			}
		}
	}
	if !sawList {
		return nil, "", fmt.Errorf("the node did not answer 'region list allowed'; " +
			"its firmware may not support regions")
	}
	return regions, scope, nil
}

// isRegionCSV reports whether a line looks like the comma-separated region
// names a node answers "region list allowed" with: short tokens, optionally a
// leading wildcard, and nothing that is plainly prose.
func isRegionCSV(line string) bool {
	if strings.ContainsAny(line, " \t") && !strings.Contains(line, ",") {
		return false
	}
	for _, tok := range strings.Split(line, ",") {
		tok = strings.TrimSpace(strings.TrimPrefix(tok, "#"))
		if tok == "*" || tok == "" {
			continue
		}
		if len(tok) > 16 {
			return false
		}
		for _, r := range tok {
			if !isRegionNameChar(r) {
				return false
			}
		}
	}
	return true
}

// isRegionNameChar reports whether a rune can appear in a region name: letters,
// digits, underscore or hyphen. A region name is short and plain, and anything
// else marks a line as prose rather than a region list.
func isRegionNameChar(r rune) bool {
	return r == '_' || r == '-' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
