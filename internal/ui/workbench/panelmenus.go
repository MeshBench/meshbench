// Where each panel is offered: the menu it belongs to, and the group it sits
// in there.
//
// One table rather than a field set at each registration, because two things
// read it - the menus themselves, and the test that every panel is reachable
// from exactly one of them - and a copy is a copy that drifts. This is the
// whole inventory: a panel missing from here is a panel nobody can open, and
// the test says so by name rather than leaving it to be discovered.
package workbench

import "github.com/MeshBench/meshbench/internal/ui/shell"

type panelHome struct{ Menu, Section string }

// panelMenus is every panel, filed where somebody would look for it.
var panelMenus = map[string]panelHome{
	// What a session is made of.
	"Import":        {"File", "Import & Export"},
	"Firmware":      {"File", "Import & Export"},
	"Resources":     {"File", "Import & Export"},
	"Runs":          {"File", "Open & Save"},
	"Map":           {"View", "The window"},
	"Configuration": {"View", "Preferences"},

	// Watching one run.
	"Events":         {"Simulation", "Watch"},
	"Scoreboard":     {"Simulation", "Watch"},
	"Logs":           {"Simulation", "Watch"},
	"Timelines":      {"Simulation", "Watch"},
	"Schedule":       {"Simulation", "Traffic"},
	"Live feed":      {"Simulation", "Traffic"},
	"Energy":         {"Simulation", "Cost"},
	"Sweep":          {"Simulation", "Compare"},
	"Results":        {"Simulation", "Compare"},
	"Matrix":         {"Simulation", "Compare"},
	"Experiment log": {"Simulation", "Compare"},

	// The nodes themselves.
	"Nodes":           {"Mesh", "The mesh"},
	"Nodes running":   {"Mesh", "The mesh"},
	"Fleet":           {"Mesh", "Commands"},
	"Provisioning":    {"Mesh", "Commands"},
	"Console":         {"Mesh", "One node"},
	"Companion bench": {"Mesh", "One node"},

	// The questions asked of a run.
	"Link":            {"Analysis", "Radio"},
	"Budget":          {"Analysis", "Radio"},
	"Waterfall":       {"Analysis", "Radio"},
	"Packet":          {"Analysis", "Packets"},
	"Packet timeline": {"Analysis", "Packets"},
	"Inspector":       {"Analysis", "Packets"},
	"Planning":        {"Analysis", "Siting"},
	"Boundary":        {"Analysis", "Siting"},
	"Compare":         {"Analysis", "Against reality"},
	"Validate":        {"Analysis", "Against reality"},

	"Licences": {"Help", ""},
}

// homed fills in where a panel is offered, so registration says what a panel
// is and this says where it is found.
func homed(p *shell.Panel) *shell.Panel {
	h := panelMenus[p.Name]
	p.Menu, p.Section = h.Menu, h.Section
	return p
}
