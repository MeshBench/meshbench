package workbench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// Every step says what it opens and what somebody should see.
//
// Two shapes, because the pass covers two things. A workbench step carries
// flags and launches the binary; a documentation step carries a URL and is a
// page somebody reads against the application it describes. Both are steps in
// the same pass: the manual is part of the change, so a release is not tested
// until the pages have been walked too.
//
// A step with no expectation is a screenshot nobody can judge: the point of
// the pass is that a person compares what they see against a sentence, and a
// missing sentence turns the step into "look at it and hope".
func TestEveryCaptureStepIsRunnableAndJudgeable(t *testing.T) {
	for bucket, steps := range loadShotSteps(t) {
		for _, s := range steps {
			switch {
			case s.Name == "":
				t.Errorf("%s: a step with no name has nowhere to write its picture", bucket)
			case len(s.Flags) == 0 && s.URL == "":
				t.Errorf("%s/%s: neither flags to run nor a page to open, so "+
					"there is nothing to do", bucket, s.Name)
			case len(s.Flags) > 0 && s.URL != "":
				t.Errorf("%s/%s: both flags and a URL; a step is one or the "+
					"other, and which it is decides how it is checked",
					bucket, s.Name)
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
	URL    string   `json:"url,omitempty"`
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

// Every published page is in the pass.
//
// The manual is part of the change, so a release is not tested until somebody
// has walked the pages against the build. Read from the documentation site's
// own navigation table, for the same reason the panel check reads panelMenus:
// a hand-kept list of forty pages is a list that is wrong by the second one.
//
// Skipped, rather than failed, when the documentation repository is not beside
// this one. It is a separate repository and a contributor need not have it -
// but on the machine that cuts a release it is there, and that is the machine
// this matters on.
func TestEveryPublishedPageIsInThePass(t *testing.T) {
	nav, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"meshbench-docs", "gen.py"))
	if err != nil {
		t.Skip("no documentation checkout beside this one:", err)
	}
	body := string(nav)
	i := strings.Index(body, "NAV = [")
	j := strings.Index(body[i:], "\n]")
	if i < 0 || j < 0 {
		t.Skip("the documentation's navigation table has moved")
	}
	pages := regexp.MustCompile(`\("([a-z0-9-]+\.html)", "`).
		FindAllStringSubmatch(body[i:i+j], -1)
	if len(pages) == 0 {
		t.Skip("no pages found in the navigation table")
	}

	inPass := map[string]bool{}
	for _, s := range loadShotSteps(t)["docs"] {
		if k := strings.LastIndex(s.URL, "/"); k >= 0 {
			inPass[s.URL[k+1:]] = true
		}
	}
	for _, p := range pages {
		if !inPass[p[1]] {
			t.Errorf("%s is published and is in no step, so the pass can pass "+
				"without anybody having read it", p[1])
		}
	}
}
