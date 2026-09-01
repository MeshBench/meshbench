// The panels that ask questions of a simulation: validation, planning,
// comparison, links and sweeps.
package workbench

import (
	"github.com/MeshBench/meshbench/internal/ui/shell"
)

func addStudyPanels(d panelDeps) {
	d.sh.Add(homed(&shell.Panel{Name: "Validate", Windowable: true,
		Draw: d.withControls(d.validCtl.Draw, (&validatePanel{}).Draw)}))
	imp := &importPanel{}
	// The study area is chosen in Import, where it decides what is fetched.
	imp.OnRemove = func(name string) {
		d.do("boundary.remove", map[string]any{"name": name})
	}
	// Search, then offer what came back: a search for "Fife" returns two, and
	// adding by the typed text picks whichever matched first. The chooser is
	// how everything else in this application picks from a list.
	d.importCtl.OnArea = func(query string) {
		ask := d.chooserIn("Import")
		go func() {
			res, err := d.st.Do(d.ctx, "boundary.set", map[string]any{"query": query})
			if err != nil {
				d.do("ui.said", "boundary: "+err.Error())
				return
			}
			m, _ := res.(map[string]any)
			names, _ := m["names"].([]string)
			if len(names) == 0 {
				d.do("ui.said", "nothing matches "+query)
				return
			}
			ask("Add which area to the study?", names, func(name string) {
				d.do("boundary.accept", map[string]any{"name": name})
			})
		}()
	}
	d.sh.Add(homed(&shell.Panel{Name: "Import", Windowable: true,
		Draw: d.withControls(d.importCtl.Draw, imp.Draw)}))
	plan := &planPanel{}
	plan.OnRun = func() {
		go func() { _, _ = d.st.Do(d.ctx, "plan.routes", nil) }()
	}
	d.sh.Add(homed(&shell.Panel{Name: "Planning", Windowable: true,
		Draw: d.withControls(d.planCtl.Draw, plan.Draw)}))
	cmpP := &comparePanel{}
	cmpP.OnSave = func() {
		go func() { _, _ = d.st.Do(d.ctx, "run.save", "run") }()
	}
	d.sh.Add(homed(&shell.Panel{Name: "Compare", Windowable: true, Draw: cmpP.Draw}))
	bounds := &boundaryPanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Boundary", Windowable: true,
		Draw: d.withControls(d.boundCtl.Draw, bounds.Draw)}))
	d.sh.Add(homed(&shell.Panel{Name: "Link", Windowable: true,
		Draw: (&linkPanel{}).Draw}))
	runs := &runsPanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Runs", Windowable: true, Draw: runs.Draw}))
	// Controls and results are two panels, not one.
	//
	// Together in a 340dp rail they made each other useless: the fields were
	// too narrow to read a version string in, and the table underneath got
	// whatever was left, which was nothing. The Bench view puts the controls in
	// a fixed column and gives the table the middle.
	d.sh.Add(homed(&shell.Panel{Name: "Sweep", Windowable: true, Draw: d.sweepCtl.Draw}))
	d.sh.Add(homed(&shell.Panel{Name: "Results", Windowable: true, Draw: (&sweepResults{}).Draw}))
}
