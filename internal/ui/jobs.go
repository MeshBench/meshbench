package ui

import (
	"fmt"
	"sync/atomic"

	"github.com/AllenDang/cimgui-go/imgui"
)

// jobRow is one long-running operation, described uniformly.
type jobRow struct {
	label    string
	progress string
	cancel   func()
}

// activeJobs collects everything currently running in the background.
//
// One shape for all of them, because the alternative was four differently
// shaped progress indicators in four corners — a warm percentage in the
// toolbar, a coverage note in the status line, an import bar in a window, an
// inference bar in another — and no way to see, in one place, what the
// application is busy doing or to tell any of it to stop.
func (a *App) activeJobs() []jobRow {
	var jobs []jobRow
	if done, total := a.warmDone.Load(), a.warmTotal.Load(); total > 0 && done < total {
		jobs = append(jobs, jobRow{
			label:    "warming links",
			progress: fmt.Sprintf("%d%%", done*100/total),
			cancel:   a.warmCancel,
		})
	}
	if a.cov.running {
		jobs = append(jobs, jobRow{
			label:    "coverage: " + a.cov.node,
			progress: a.cov.mode.String(),
			cancel:   a.cov.cancel,
		})
	}
	if a.imp.job != nil {
		n := atomic.LoadInt64(&a.imp.job.count)
		jobs = append(jobs, jobRow{
			label:    "import from " + a.imp.source,
			progress: fmt.Sprintf("%d records", n),
			cancel:   a.imp.job.cancel,
		})
	}
	if a.infer.running {
		jobs = append(jobs, jobRow{
			label:    "reading traffic",
			progress: fmt.Sprintf("%d packets", a.infer.fetched.Load()),
		})
	}
	if a.ab.running {
		jobs = append(jobs, jobRow{
			label:    "A/B bisect",
			progress: a.ab.verA + " vs " + a.ab.verB,
			cancel:   a.ab.cancel,
		})
	}
	if a.val.running {
		jobs = append(jobs, jobRow{
			label:    "validating against reality",
			progress: "fetching receptions",
			cancel:   a.val.cancel,
		})
	}
	a.live.mu.Lock()
	liveRunning, liveInjected := a.live.running, a.live.injected
	a.live.mu.Unlock()
	if liveRunning {
		n := liveInjected
		jobs = append(jobs, jobRow{
			label:    "live feed",
			progress: fmt.Sprintf("%d injected", n),
			cancel:   a.live.cancel,
		})
	}
	if s := a.fetchState(); s != "" {
		jobs = append(jobs, jobRow{label: "terrain", progress: s})
	}
	return jobs
}

// drawJobsPopupBody is the popup itself, opened from the status bar - one
// place for everything in flight, always at the same end of the same bar.
func (a *App) drawJobsPopupBody() {
	jobs := a.activeJobs()
	if !imgui.BeginPopup("##jobs") {
		return
	}
	for i, j := range jobs {
		imgui.Text(j.label)
		imgui.SameLine()
		imgui.TextDisabled(j.progress)
		if j.cancel != nil {
			imgui.SameLine()
			if imgui.SmallButton(fmt.Sprintf("cancel##job%d", i)) {
				j.cancel()
			}
		}
	}
	imgui.EndPopup()
}

// provisionNode sends the on-start commands to one running node, now.
//
// The right-click verb for "I changed the provisioning, apply it to this one"
// — the fleet window does many nodes, the Provisioning window does all of
// them on boot, and this does one without opening either.
func (a *App) provisionNode(i int) {
	if i < 0 || i >= len(a.Nodes) {
		return
	}
	name := a.Nodes[i].Name
	if a.eng == nil || a.eng.FirmwareCount() == 0 {
		a.winProvision = true
		a.status = "no firmware running - set up what nodes are told on boot instead"
		return
	}
	cmds := a.startupCommands(i)
	if len(cmds) == 0 {
		a.winProvision = true
		a.status = "nothing to provision - tick something in the Provisioning window"
		return
	}
	for _, cmd := range cmds {
		if err := a.typeAt(name, cmd); err != nil {
			a.status = name + ": " + err.Error()
			return
		}
	}
	a.stepEngine(50)
	a.status = fmt.Sprintf("%s: %d commands sent", name, len(cmds))
}
