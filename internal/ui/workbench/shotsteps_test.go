package workbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

type shotStep struct {
	Name   string   `json:"name"`
	What   string   `json:"what"`
	Flags  []string `json:"flags"`
	Expect string   `json:"expect"`
	Then   string   `json:"then,omitempty"`
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
