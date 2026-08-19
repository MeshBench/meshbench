// The panels about the application itself: its configuration, its log of
// experiments, and what it is licensed under.
package workbench

import (
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

// addAppPanels hands back the Configuration panel because Run keeps wiring it
// after registration - the settings page and the menu both reach into it.
func addAppPanels(d panelDeps) *configPanel {
	cfg := &configPanel{do: d.do, choose: d.chooserIn("Configuration")}
	if *d.cfgSection != "" {
		cfg.Open(*d.cfgSection)
	}
	logp := &logPanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Configuration", Windowable: true, Draw: cfg.Draw}))
	d.sh.Add(homed(&shell.Panel{Name: "Experiment log", Windowable: true, Draw: logp.Draw}))
	lic := &licPanel{}
	// A chip is a click, and a click cannot be captured; the flag is how a
	// screenshot of one section gets taken.
	lic.openAt = *d.licSection
	d.sh.Add(homed(&shell.Panel{Name: "Licences", Windowable: true, Draw: lic.Draw}))
	return cfg
}
