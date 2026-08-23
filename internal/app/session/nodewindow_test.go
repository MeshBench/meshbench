package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// tabStrictUI answers about tabs the way the workbench does: by name, and no.
//
// The stub the parity tests share takes any tab it is handed, which is why they
// all passed while nobody could open a window. A double is only worth having if
// it refuses what the real one refuses.
type tabStrictUI struct {
	stubUI
	asked string
}

func (u *tabStrictUI) OpenNodeWindow(_, tab string) (string, error) {
	u.asked = tab
	known := []string{"Console", "Companion", "SDR", "Settings", "Radio",
		"Stats", "Activity", "Connect", "Hardware"}
	if tab == "" {
		return "Console", nil
	}
	for _, k := range known {
		if strings.EqualFold(k, tab) {
			return k, nil
		}
	}
	return "", fmt.Errorf("no tab called %q - there is %s", tab, strings.Join(known, ", "))
}

// Does double-clicking a node open its window?
//
// Reported: it opens nothing at all. Every route to a node window passes the
// node as a bare string - the map, the list, the bench panel, the startup flag
// - and the optional tab was read with the helper that answers with the bare
// parameter whatever field it is asked for. So the verb asked for a tab named
// after the node, the window refused by name, and the refusal went to the log
// where nobody was looking.
func TestANodeWindowOpensFromABareName(t *testing.T) {
	ui := &tabStrictUI{}
	st, sm := Boot(Options{NoPrefs: true})
	sm.SetUI(ui)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.Run(ctx)

	if _, err := st.Do(ctx, "nodes.place", map[string]any{
		"name": "Jazzy", "kind": "companion", "lat": 56.25, "lon": -3.10}); err != nil {
		t.Fatal(err)
	}

	got, err := st.Do(ctx, "node.window", "Jazzy")
	if err != nil {
		t.Fatalf("opening a window by bare name: %v", err)
	}
	if ui.asked != "" {
		t.Errorf("the tab asked for was %q, want none - the name is not a tab", ui.asked)
	}
	if m, ok := got.(map[string]any); ok {
		if m["node"] != "Jazzy" {
			t.Errorf("opened %v, want Jazzy", m["node"])
		}
		if m["tab"] != "Console" {
			t.Errorf("opened on %v, want the default tab", m["tab"])
		}
	}

	// And a tab that is named is still honoured, which is the thing the bare
	// string must not be allowed to imitate.
	if _, err := st.Do(ctx, "node.window", map[string]any{
		"node": "Jazzy", "tab": "Radio"}); err != nil {
		t.Fatalf("opening on a named tab: %v", err)
	}
	if ui.asked != "Radio" {
		t.Errorf("the tab asked for was %q, want Radio", ui.asked)
	}
}
