// A node's name reaches the handler that needs it, whatever is in the name.
//
// node.reflashed used to recover the node's name by cutting the message it
// carried at the first space. That is the node's name only when the name has no
// space in it - and the networks this simulator is built for are imported from
// real meshes, where "GM0KVE KINROSS Repeater" is an ordinary name and
// "Aberdeenshire Observer" is another.
//
// So changing such a node from a host build to a board image refreshed a node
// called "GM0KVE", which is nothing. The stats knew the node had a board and
// the node list did not, and the board view - which asks the list - refused to
// open on a board that was sitting there drawing its own panel.
package session

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// The list a node window reads is refreshed for a node whose name has spaces.
//
// refreshNodeBuild is what copies the build a node has been changed to back
// into that list, and it is looked up by name. The name it was given used to be
// the front of a sentence, so this never ran for most of the nodes in a real
// imported network.
func TestTheNodeListIsRefreshedForANameWithSpaces(t *testing.T) {
	for _, name := range []string{"Deck", "GM0KVE KINROSS Repeater"} {
		n := repeaterNode(name)
		n.Firmware = scenario.FirmwareRef{Version: "v1.17.1", Board: "Heltec_v3"}
		s := &Sim{nodes: []scenario.Node{n}}
		w := &state.World{Nodes: []state.Node{{Name: name}}}

		s.refreshNodeBuild(w, name)
		if got := w.Nodes[0].Board; got != "Heltec_v3" {
			t.Errorf("%q: the list says the board is %q after a change to a "+
				"board image; the board view asks this list and refuses on an "+
				"empty answer", name, got)
		}
	}
}

// The names that broke it, and the one that did not.
func TestANodeNameSurvivesBeingCarriedAsAField(t *testing.T) {
	for _, name := range []string{
		"Deck",
		"GM0KVE KINROSS Repeater",
		"Aberdeenshire Observer",
		"sco-fif-montrave",
		"West Lomond ⛰",
	} {
		// What the reflash sends, as it sends it.
		p := map[string]any{"node": name, "message": name + " now runs v1.17.1"}
		got, _ := p["node"].(string)
		if got != name {
			t.Errorf("the field carried %q for a node called %q", got, name)
		}
		// And what the old way would have made of it, which is the thing this
		// test exists to keep from coming back.
		cut, _, _ := strings.Cut(p["message"].(string), " ")
		if strings.Contains(name, " ") && cut == name {
			t.Errorf("%q: the message cut at the first space still produced "+
				"the whole name, so this test is not testing anything", name)
		}
	}
}
