package scenario_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A front end can only be exercised by an emulated node, and until this fixture
// existed no shipped scenario had one at all.
//
// The reason is in bridge/main.cpp: a native node reports its front-end line as
// "not answered" rather than "module out", because SimHal owns an array of pins
// and nothing drives an enable line into it - reporting the line low would be
// true of the pin and false about the board. So the whole transmit branch of
// effectiveRF is unreachable on a native node, and every node in the Scotland
// and Fife fixtures is a RAK4631, which has no module in any case.
//
// That is how an eight-cell sweep ran the W3 code path exactly zero times.
func TestTheFEMFixtureCanActuallyExerciseAFrontEnd(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "fixture-fem-e22.json"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var f struct {
		Nodes []scenario.Node `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse: %v", err)
	}

	var withFEM, emulated int
	for _, n := range f.Nodes {
		if n.FEM == nil {
			continue
		}
		withFEM++
		// Emulated, or the front end is decoration: the enable line reaches the
		// chip model as a GPIO from QEMU, and a native node never drives one.
		if n.Firmware.Board == "" {
			t.Errorf("%s carries a front end but runs the host build, "+
				"which can never report the enable line", n.Name)
			continue
		}
		emulated++
		if n.FEM.TxLossDB <= 0 {
			t.Errorf("%s: a front end that costs nothing when it is out cannot "+
				"show the fault this fixture exists for", n.Name)
		}
	}
	if withFEM == 0 {
		t.Fatal("no node carries a front end, so W3 is still unexercised")
	}
	if emulated == 0 {
		t.Fatal("no emulated node carries a front end")
	}
	t.Logf("%d nodes with a front end, %d of them emulated", withFEM, emulated)
}
