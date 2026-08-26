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
	// Resources sits under Firmware because it is the same question about
	// everything that is not firmware, and somebody who has found one will
	// look for the other beside it.
	res := &resourcesPanel{}
	res.Refresh = func() {
		go func() { _, _ = d.st.Do(d.ctx, "resource.list", nil) }()
	}
	res.OnAction = func(verb string, params map[string]any) {
		go func() {
			if _, err := d.st.Do(d.ctx, verb, params); err != nil {
				_, _ = d.st.Do(d.ctx, "ui.said", verb+": "+err.Error())
			}
			_, _ = d.st.Do(d.ctx, "resource.list", nil)
		}()
	}
	// Opening the panel re-asks the cache what is there, so a download or a
	// wipe made elsewhere shows the moment it is looked at rather than only
	// after a manual Rescan.
	d.sh.Add(homed(&shell.Panel{Name: "Resources", Windowable: true, Draw: res.Draw, OnReveal: res.Refresh}))

	lic := &licPanel{}
	// A chip is a click, and a click cannot be captured; the flag is how a
	// screenshot of one section gets taken.
	lic.openAt = *d.licSection
	d.sh.Add(homed(&shell.Panel{Name: "Licences", Windowable: true, Draw: lic.Draw}))
	return cfg
}
