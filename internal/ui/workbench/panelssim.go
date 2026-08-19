// The panels that watch a running simulation.
package workbench

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func addSimPanels(d panelDeps) {
	events := &eventsPanel{OnOpenPacket: d.openPacket}
	scores := &scorePanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Events", Windowable: true, Draw: events.Draw}))
	d.sh.Add(homed(&shell.Panel{Name: "Scoreboard", Windowable: true, Draw: scores.Draw}))
	logs := &logsPanel{do: d.do}
	d.sh.Add(homed(&shell.Panel{Name: "Logs", Windowable: true, Draw: logs.Draw}))
	tl := &comp.Timeline{}
	d.sh.Add(homed(&shell.Panel{Name: "Packet timeline", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return tl.Layout(t, gtx, s)
		}}))
	d.sh.Add(homed(&shell.Panel{Name: "Budget", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Budget{}.Layout(t, gtx, s)
		}}))
	d.sh.Add(homed(&shell.Panel{Name: "Matrix", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Matrix{}.Layout(t, gtx, s)
		}}))
	d.sh.Add(homed(&shell.Panel{Name: "Energy", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return comp.Energy{}.Layout(t, gtx, s)
		}}))
	wf := &comp.Waterfall{}
	d.sh.Add(homed(&shell.Panel{Name: "Waterfall", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			return wf.Layout(t, gtx, s)
		}}))
	feed := &feedPanel{}
	feed.OnPull = func() {
		go func() { _, _ = d.st.Do(d.ctx, "feed.pull", *d.importFlag) }()
	}
	d.sh.Add(homed(&shell.Panel{Name: "Live feed", Windowable: true,
		Draw: d.withControls(d.feedCtl.Draw, feed.Draw)}))
	tls := &timelinesPanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Timelines", Windowable: true, Draw: tls.Draw}))
	sched := &schedulePanel{}
	d.sh.Add(homed(&shell.Panel{Name: "Schedule", Windowable: true,
		Draw: d.withControls(d.schedCtl.Draw, sched.Draw)}))
}
