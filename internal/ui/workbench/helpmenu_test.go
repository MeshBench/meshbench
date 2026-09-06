package workbench

import (
	"strings"
	"testing"
)

// Help has a way to reach the manual.
//
// It held one authored item - "What this run assumes" - and nothing anywhere
// pointed at the published documentation. That is the one thing a Help menu is
// for, and this project holds itself to a hard rule about keeping that site
// current: a page per window, a screenshot on each, the verb beside the
// control. Somebody with the window in front of them had to already know the
// site existed.
func TestHelpMenuReachesTheManual(t *testing.T) {
	var help []string
	for _, m := range workbenchMenus() {
		if m.Name != "Help" {
			continue
		}
		for _, it := range m.Items {
			help = append(help, it.Action)
		}
	}
	if len(help) == 0 {
		t.Fatal("there is no Help menu")
	}
	found := false
	for _, a := range help {
		if a == "help.manual" {
			found = true
		}
	}
	if !found {
		t.Errorf("Help has no route to the manual; it offers: %s",
			strings.Join(help, ", "))
	}
}

// And the address it opens is the published one, checked here so a typo is a
// red build rather than a browser tab nobody can explain.
func TestTheManualURLIsThePublishedSite(t *testing.T) {
	if !strings.HasPrefix(manualURL, "https://") {
		t.Errorf("the manual is opened over %q, which is not https", manualURL)
	}
	if !strings.Contains(manualURL, "meshbench.github.io/docs") {
		t.Errorf("the manual URL does not point at the published site: %q", manualURL)
	}
}
