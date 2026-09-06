package workbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/ui/workbench/licences"
	"github.com/MeshBench/meshbench/internal/ui/workbench/nodeview"
)

// Every panel has a capture step, and every capture step names a panel.
//
// tools/shots drives the workbench once per step and writes a picture, and
// issue 318's human pass walks the same list. Both are only worth having if
// the list is the whole application: a panel added without a step is a panel
// nobody photographs, and it goes stale without anybody noticing - which is
// the failure the documentation rule exists to stop, one layer down.
//
// Checked against panelMenus rather than against a list kept here, for the
// reason the walkthrough gives for reading the same table: a hand-written copy
// was already three panels short of the real one.
func TestEveryPanelHasACaptureStep(t *testing.T) {
	steps := loadShotSteps(t)

	// Both forms, because they are different windows: a panel that draws
	// docked and not in a window of its own is a real fault this window set
	// has had, and one picture cannot show both.
	docked, popped := map[string]bool{}, map[string]bool{}
	for _, s := range steps["panels"] {
		switch s.Flag() {
		case "-panel":
			docked[s.Panel()] = true
		case "-pop-out":
			popped[s.Panel()] = true
		}
	}
	for name := range panelMenus {
		if !docked[name] {
			t.Errorf("%q has no docked capture step, so nothing takes its "+
				"picture: add it to tools/shots/steps.json", name)
		}
		if !popped[name] {
			t.Errorf("%q has no popped-out capture step, so nothing shows it "+
				"in a window of its own", name)
		}
	}
	// And the other way: a step naming a panel that has gone is a step whose
	// command fails with "no such panel", which reads as a broken capture run
	// rather than as a list that needs an entry removed.
	for _, s := range steps["panels"] {
		if _, ok := panelMenus[s.Panel()]; !ok {
			t.Errorf("step %q names %q, which is not a panel any more",
				s.Name, s.Panel())
		}
	}
}

// Every step says what it runs and what somebody should see.
//
// A step with no expectation is a screenshot nobody can judge: the point of
// the pass is that a person compares the picture against a sentence, and a
// missing sentence turns the step into "look at it and hope".
func TestEveryCaptureStepIsRunnableAndJudgeable(t *testing.T) {
	for bucket, steps := range loadShotSteps(t) {
		for _, s := range steps {
			switch {
			case s.Name == "":
				t.Errorf("%s: a step with no name has nowhere to write its picture", bucket)
			case len(s.Flags) == 0:
				t.Errorf("%s/%s: no flags, so there is nothing to run", bucket, s.Name)
			case s.What == "":
				t.Errorf("%s/%s: no description", bucket, s.Name)
			case s.Expect == "":
				t.Errorf("%s/%s: no expected result, so the picture cannot be "+
					"judged against anything", bucket, s.Name)
			}
		}
	}
}

// Every node-window step names a real tab, and every tab has a step.
//
// The bucket had no tie to Tab at all, so it drifted the moment Antenna was
// added to the enum between Radio and Stats: the steps addressed tabs by index,
// the -node-tab help spelled the same list out by hand and missed it too, and
// the two agreed with each other while disagreeing with the code. Five steps
// photographed the tab after the one they were named for and Output was never
// photographed - and every picture looked like a working node window, because
// it was one.
//
// Checked against nodeview.TabNames for the reason the panel sweep is checked
// against panelMenus: a hand-written copy of a list is a copy that goes stale.
func TestEveryNodeTabHasACaptureStep(t *testing.T) {
	steps := loadShotSteps(t)

	want := map[string]bool{}
	for _, n := range nodeview.TabNames() {
		want[n] = true
	}
	got := map[string]bool{}
	for _, s := range steps["node-window"] {
		tab := s.Value("-node-tab")
		if tab == "" {
			t.Errorf("step %q names no tab, so it photographs whichever one the "+
				"window happens to open on", s.Name)
			continue
		}
		if !want[tab] {
			t.Errorf("step %q names %q, which is not a tab: there is %s",
				s.Name, tab, strings.Join(nodeview.TabNames(), ", "))
		}
		got[tab] = true
	}
	for _, n := range nodeview.TabNames() {
		if got[n] || n == "Hardware" {
			// Hardware is drawn from an emulated board's own model, so a node
			// running a host build does not grow it and the window falls back
			// to Console. No shipped fixture the sweep runs has an emulated
			// node, and standing one up boots QEMU - which is a board probe,
			// not a screenshot. The board view covers the same hardware from
			// the other side, and has its own bucket.
			continue
		}
		t.Errorf("the %s tab has no capture step, so nothing takes its "+
			"picture: add one to tools/shots/steps.json", n)
	}
}

// Every configuration and licence step names a section that exists.
//
// Neither flag validated its argument, so five of six configuration section
// names were wrong - "rf" for "RF Simulation", and four that are not sections
// at all - and nothing said so. With the flags also failing to open their
// panel, all eleven steps in the two buckets produced byte-identical pictures
// of a page neither is about.
func TestEverySectionStepNamesASectionThatExists(t *testing.T) {
	steps := loadShotSteps(t)

	for _, s := range steps["configuration"] {
		name := s.Value("-config-section")
		if !slices.ContainsFunc(ConfigSections(), func(c string) bool {
			return strings.EqualFold(c, name)
		}) {
			t.Errorf("step %q names configuration section %q, which does not "+
				"exist: there is %s", s.Name, name,
				strings.Join(ConfigSections(), ", "))
		}
	}
	for _, s := range steps["licences"] {
		if name := s.Value("-licence-section"); !licences.HasSection(name) {
			t.Errorf("step %q names licence section %q, which does not exist: "+
				"there is %s", s.Name, name,
				strings.Join(licences.SectionIDs(), ", "))
		}
	}

	// And the other way, so a section added later gets a picture. Both lists
	// come from the code rather than from a copy kept here, for the reason the
	// panel sweep reads panelMenus.
	tookConfig, tookLic := map[string]bool{}, map[string]bool{}
	for _, s := range steps["configuration"] {
		tookConfig[strings.ToLower(s.Value("-config-section"))] = true
	}
	for _, s := range steps["licences"] {
		tookLic[strings.ToLower(s.Value("-licence-section"))] = true
	}
	for _, c := range ConfigSections() {
		if !tookConfig[strings.ToLower(c)] {
			t.Errorf("the %q configuration section has no capture step", c)
		}
	}
	for _, l := range licences.SectionIDs() {
		// project is drawn as the panel's header rather than as a chip, so
		// there is no filtered view of it to photograph.
		if l == "project" || tookLic[strings.ToLower(l)] {
			continue
		}
		t.Errorf("the %q licence section has no capture step", l)
	}
}

// Every board-view step names a node the fixture the sweep runs has.
//
// All three named citizen71, which is a node of the live ScotMesh network and
// is in none of the seven shipped fixtures. node.boardview refused every time,
// correctly and on stderr, and shots.py counted the run a success because the
// workbench went on running perfectly well without a board view.
func TestEveryBoardViewStepNamesANodeTheFixtureHas(t *testing.T) {
	steps := loadShotSteps(t)
	have := shotFixtureNodes(t)

	for _, s := range steps["board-view"] {
		name := s.Value("-board-view")
		if name == "" {
			t.Errorf("step %q names no node", s.Name)
			continue
		}
		if !have[name] {
			t.Errorf("step %q opens the board view on %q, which is not in the "+
				"fixture the sweep runs: node.boardview refuses and the capture "+
				"is of a plain workbench", s.Name, name)
		}
	}
	for _, s := range steps["node-window"] {
		if name := s.Value("-node-window"); name != "" && !have[name] {
			t.Errorf("step %q opens a node window on %q, which is not in the "+
				"fixture the sweep runs", s.Name, name)
		}
	}
}

// shotFixtureNodes is every node name in the fixture tools/shots runs against.
func shotFixtureNodes(t *testing.T) map[string]bool {
	t.Helper()
	// The same default shots.py carries; a step pointed at a node is pointed at
	// one of these.
	path := filepath.Join("..", "..", "..", "fixtures", "fixture-fife-strict.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the sweep's fixture is missing: %v", err)
	}
	var f struct {
		Nodes []struct {
			Name string `json:"Name"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("the sweep's fixture does not parse: %v", err)
	}
	out := map[string]bool{}
	for _, n := range f.Nodes {
		out[n.Name] = true
	}
	return out
}

// A step that cannot run says what it is waiting for, and a step that names its
// own fixture names one that is there.
func TestASkippedStepSaysWhatItNeeds(t *testing.T) {
	for bucket, steps := range loadShotSteps(t) {
		for _, s := range steps {
			if s.Fixture != "" {
				p := filepath.Join("..", "..", "..", "fixtures", s.Fixture+".json")
				if _, err := os.Stat(p); err != nil {
					t.Errorf("%s/%s names fixture %q, which is not in fixtures/",
						bucket, s.Name, s.Fixture)
				}
			}
			if s.Needs != "" && len(strings.Fields(s.Needs)) < 4 {
				t.Errorf("%s/%s skips itself without saying enough about why: %q",
					bucket, s.Name, s.Needs)
			}
		}
	}
}

type shotStep struct {
	Name    string   `json:"name"`
	What    string   `json:"what"`
	Flags   []string `json:"flags"`
	Expect  string   `json:"expect"`
	Fixture string   `json:"fixture,omitempty"`
	Needs   string   `json:"needs,omitempty"`
	Then    string   `json:"then,omitempty"`
}

// Panel is the panel this step is about, read from the flags rather than from
// the step's name: the name is a slug, and a slug does not survive a round
// trip through "Live feed".
func (s shotStep) Panel() string {
	for i, f := range s.Flags {
		if (f == "-panel" || f == "-pop-out") && i+1 < len(s.Flags) {
			return s.Flags[i+1]
		}
	}
	return ""
}

// Value is what follows a named flag, or "" where the step does not carry it.
func (s shotStep) Value(flag string) string {
	for i, f := range s.Flags {
		if f == flag && i+1 < len(s.Flags) {
			return s.Flags[i+1]
		}
	}
	return ""
}

// Flag is which of the two ways this step opens its panel.
func (s shotStep) Flag() string {
	for _, f := range s.Flags {
		if f == "-panel" || f == "-pop-out" {
			return f
		}
	}
	return ""
}

func loadShotSteps(t *testing.T) map[string][]shotStep {
	t.Helper()
	// Up out of internal/ui/workbench to the tree root.
	path := filepath.Join("..", "..", "..", "tools", "shots", "steps.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the capture manifest is missing: %v", err)
	}
	var out map[string][]shotStep
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("the capture manifest does not parse: %v", err)
	}
	return out
}
