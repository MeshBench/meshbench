package ui

import (
	"fmt"
	"github.com/A13xB0/meshcoresim/internal/scenario"
	"strconv"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"
)

// The Bench view: define a sweep, watch it run, read what differed.
//
// Laid out as three questions in the order they get asked - what am I varying,
// what is it doing now, and what came out - because an experiment is a session
// somebody sits through rather than a dialogue they dismiss.

// benchDefaults seeds the sweep builder with the experiment that prompted all
// this, so the view opens on something real rather than an empty form.
func (a *App) benchDefaults() *experiment {
	var senders []string
	for i := range a.Nodes {
		if a.Nodes[i].Kind == scenario.Companion && len(senders) < 6 {
			senders = append(senders, a.Nodes[i].Name)
		}
	}
	return &experiment{
		// One neutral arm: whatever the scenario already says. Varying a
		// parameter replaces it, so the first gesture produces clean labels
		// rather than crossing onto a guess nobody asked for.
		Arms:     []expArm{{Label: "baseline", PathHashMode: -1}},
		Seeds:    []uint64{4417, 9001},
		Senders:  senders,
		Channel:  "#sco",
		Scope:    "#sco",
		SendAtMs: 45000,
		RunForMs: 40000,
	}
}

func (a *App) ensureExperiment() *experiment {
	if a.exp == nil {
		a.exp = a.benchDefaults()
	}
	return a.exp
}

// drawSweep is the definition: what is being varied, over how many repeats, and
// what that will cost before anybody agrees to it.
func (a *App) drawSweep() {
	e := a.ensureExperiment()

	textDim("scenario")
	imgui.SameLine()
	imgui.Text(fmt.Sprintf("%d nodes", len(a.Nodes)))
	textDim("senders")
	imgui.SameLine()
	if len(e.Senders) == 0 {
		imgui.Text("none - an experiment needs at least one companion to originate traffic")
	} else {
		imgui.Text(fmt.Sprintf("%d, %s at %.0f s", len(e.Senders), e.Channel, float64(e.SendAtMs)/1000))
	}

	imgui.Separator()
	textDim("arms")
	for i := range e.Arms {
		imgui.PushIDInt(int32(i))
		// Removing one arm of a dozen is routine, not dangerous. A red button
		// per row turned a list into an alarm.
		if imgui.SmallButton("x") && len(e.Arms) > 1 {
			e.Arms = append(e.Arms[:i], e.Arms[i+1:]...)
			imgui.PopID()
			break
		}
		imgui.SameLine()
		imgui.SetNextItemWidth(190)
		imgui.InputTextWithHint("##label", "name", &e.Arms[i].Label, 0, nil)
		if sum := e.Arms[i].summary(); sum != "" {
			imgui.SameLine()
			textDim(sum)
		}
		imgui.PopID()
	}

	imgui.Separator()
	// The gesture that matters: an arm is a diff, so you pick a parameter and
	// the values to try rather than building arms by hand. Nobody does that
	// twice.
	textDim("vary a parameter across arms")
	imgui.SetNextItemWidth(200)
	if imgui.BeginCombo("##param", a.benchUI.param) {
		for _, p := range []string{"path.hash.mode", "loop.detect", "cad", "firmware"} {
			if imgui.SelectableBool(p) {
				a.benchUI.param = p
				a.benchUI.values = defaultValuesFor(p)
			}
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	imgui.SetNextItemWidth(220)
	imgui.InputTextWithHint("##values", "values, comma separated", &a.benchUI.values, 0, nil)
	imgui.SameLine()
	if primaryButton("add arms", imgui.NewVec2(0, 0)) {
		a.addArmsVarying(a.benchUI.param, a.benchUI.values)
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Adds one arm per value, crossed with the arms already here.\n\n" +
			"Two parameters crossed is every combination: three hash sizes by two\n" +
			"firmware versions is six arms, and with three seeds that is eighteen runs.")
	}

	imgui.Separator()
	textDim("repeats")
	imgui.SetNextItemWidth(220)
	imgui.InputTextWithHint("##seeds", "seeds, comma separated", &a.benchUI.seeds, 0, nil)
	imgui.SameLine()
	if imgui.Button("set") {
		if s := parseSeeds(a.benchUI.seeds); len(s) > 0 {
			e.Seeds = s
		}
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Repeats of one seed are identical by design, so three runs of\n" +
			"the same seed look like agreement and are one measurement.")
	}

	imgui.Separator()
	// Say what it costs before it is started, not after.
	imgui.Text(fmt.Sprintf("%d arms x %d seeds = %d runs, about %s",
		len(e.Arms), len(e.Seeds), e.runsTotal(), e.estimate().Round(time.Minute)))

	if e.running {
		if dangerButton("stop", imgui.NewVec2(0, 0)) {
			e.running = false
			e.status = "stopped"
			a.playing = false
		}
		imgui.SameLine()
		label := "pause"
		if e.paused {
			label = "resume"
		}
		if imgui.Button(label) {
			e.paused = !e.paused
		}
		return
	}
	if primaryButton("run sweep", imgui.NewVec2(0, 0)) {
		if err := a.startExperiment(); err != nil {
			a.status = err.Error()
		}
	}
	if len(e.results) > 0 {
		imgui.SameLine()
		if imgui.Button("export report") {
			path, err := a.exportExperiment(e)
			if err != nil {
				a.status = err.Error()
			} else {
				a.status = "written to " + path
			}
		}
	}
}

func (a expArm) summary() string {
	var parts []string
	if a.RepeaterVersion != "" {
		parts = append(parts, strings.TrimPrefix(a.RepeaterVersion, "repeater-"))
	}
	if a.PathHashMode >= 0 {
		parts = append(parts, fmt.Sprintf("%d-byte hash", a.PathHashMode+1))
	}
	if a.LoopDetect != "" {
		parts = append(parts, "loop "+a.LoopDetect)
	}
	if a.CAD != "" {
		parts = append(parts, "cad "+a.CAD)
	}
	return strings.Join(parts, " · ")
}

func defaultValuesFor(p string) string {
	switch p {
	case "path.hash.mode":
		return "0, 1, 2"
	case "loop.detect":
		return "off, minimal, moderate, strict"
	case "cad":
		return "off, on"
	case "firmware":
		return "1.16.0, 1.17.0"
	}
	return ""
}

// addArmsVarying crosses the existing arms with one parameter's values.
func (a *App) addArmsVarying(param, values string) {
	e := a.ensureExperiment()
	var vals []string
	for _, v := range strings.Split(values, ",") {
		if v = strings.TrimSpace(v); v != "" {
			vals = append(vals, v)
		}
	}
	if param == "" || len(vals) == 0 {
		a.status = "pick a parameter and at least one value"
		return
	}
	base := e.Arms
	// The untouched baseline is a placeholder, not a choice: crossing onto it
	// doubles every arm and compounds its label into nonsense. Replace it.
	if len(base) == 0 || (len(base) == 1 && base[0].isPristine()) {
		base = []expArm{{PathHashMode: -1}}
	}
	var out []expArm
	for _, b := range base {
		for _, v := range vals {
			arm := b
			switch param {
			case "path.hash.mode":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 || n > 2 {
					continue
				}
				arm.PathHashMode = int32(n)
				arm.Label = joinLabel(b.Label, fmt.Sprintf("%d-byte", n+1))
			case "loop.detect":
				arm.LoopDetect = v
				arm.Label = joinLabel(b.Label, "loop "+v)
			case "cad":
				arm.CAD = v
				arm.Label = joinLabel(b.Label, "cad "+v)
			case "firmware":
				arm.RepeaterVersion = "repeater-v" + v
				arm.CompanionVersion = "companion-v" + v
				arm.Label = joinLabel(b.Label, v)
			}
			out = append(out, arm)
		}
	}
	if len(out) > 0 {
		e.Arms = out
	}
}

func joinLabel(a, b string) string {
	if a == "" {
		return b
	}
	return a + " · " + b
}

func parseSeeds(s string) []uint64 {
	var out []uint64
	for _, v := range strings.Split(s, ",") {
		if n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// drawRuns is the queue: one row per run, so a stall is attributable to a run
// rather than to the sweep in general.
func (a *App) drawRuns() {
	e := a.ensureExperiment()
	if imgui.BeginTableV("##runs", 4, imgui.TableFlagsRowBg|imgui.TableFlagsBordersInnerH|
		imgui.TableFlagsScrollY, imgui.NewVec2(0, 0), 0) {
		imgui.TableSetupColumnV("arm", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableSetupColumnV("seed", imgui.TableColumnFlagsWidthFixed, 90, 0)
		imgui.TableSetupColumnV("state", imgui.TableColumnFlagsWidthFixed, 110, 0)
		imgui.TableSetupColumnV("result", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableHeadersRow()

		done := len(e.results)
		i := 0
		for ai, arm := range e.Arms {
			for _, seed := range e.Seeds {
				imgui.TableNextRow()
				imgui.TableNextColumn()
				imgui.Text(arm.Label)
				imgui.TableNextColumn()
				imgui.Text(fmt.Sprint(seed))
				imgui.TableNextColumn()
				switch {
				case i < done:
					if e.results[i].Err != "" {
						textBad("failed")
					} else if e.results[i].Flag != "" {
						textBad("flagged")
					} else {
						textDim("done")
					}
				case i == done && e.running:
					imgui.Text(phaseName(e.phase))
				default:
					textDim("queued")
				}
				imgui.TableNextColumn()
				if i < done {
					r := e.results[i]
					switch {
					case r.Err != "":
						textBad(r.Err)
					case r.Flag != "":
						textBad(r.Flag)
					default:
						imgui.Text(fmt.Sprintf("tx %d  rx %d  reached %d", r.TX, r.RX, r.Reached))
					}
				}
				i++
				_ = ai
			}
		}
		imgui.EndTable()
	}
}

func phaseName(p expPhase) string {
	switch p {
	case expPrepare:
		return "preparing"
	case expStartFirmware, expWaitFirmware:
		return "firmware"
	case expSettle:
		return "settling"
	case expConnect:
		return "connecting"
	case expSend:
		return "sending"
	case expRun:
		return "running"
	case expCollect:
		return "collecting"
	}
	return "…"
}

// drawExperimentLog is the narration: one line per thing the runner did, with
// the arm and seed on every line so a stall is attributable rather than
// mysterious.
func (a *App) drawExperimentLog() {
	e := a.ensureExperiment()
	if e.status != "" {
		imgui.Text(e.status)
		imgui.Separator()
	}
	if imgui.BeginChildStr("##explog") {
		pushMono()
		for _, l := range e.log {
			imgui.Text(l)
		}
		popMono()
		if e.running && imgui.ScrollY() >= imgui.ScrollMaxY()-4 {
			imgui.SetScrollHereYV(1)
		}
	}
	imgui.EndChild()
}

// drawMatrix is the answer: arms against metrics, read as deltas from a pinned
// baseline, with the line that says whether any of it means anything.
func (a *App) drawMatrix() {
	e := a.ensureExperiment()
	sums := e.summarise()
	if len(sums) == 0 {
		textDimWrap("No runs yet. Define a sweep and run it; results appear here as each run finishes.")
		return
	}
	if e.baselineArm >= len(sums) {
		e.baselineArm = 0
	}
	textDim("baseline")
	imgui.SameLine()
	imgui.SetNextItemWidth(220)
	if imgui.BeginCombo("##baseline", sums[e.baselineArm].Arm) {
		for i, s := range sums {
			if imgui.SelectableBool(s.Arm) {
				e.baselineArm = i
			}
		}
		imgui.EndCombo()
	}
	base := sums[e.baselineArm]

	if imgui.BeginTableV("##matrix", 8, imgui.TableFlagsRowBg|imgui.TableFlagsBordersInnerH,
		imgui.NewVec2(0, 0), 0) {
		for _, c := range []string{"arm", "runs", "tx", "rx", "reached", "collisions", "airtime", "to quiet"} {
			imgui.TableSetupColumnV(c, imgui.TableColumnFlagsWidthStretch, 0, 0)
		}
		imgui.TableHeadersRow()
		for i, s := range sums {
			imgui.TableNextRow()
			imgui.TableNextColumn()
			imgui.Text(s.Arm)
			imgui.TableNextColumn()
			if s.Flagged > 0 {
				textBad(fmt.Sprintf("%d (%d flagged)", s.Runs, s.Flagged))
			} else {
				imgui.Text(fmt.Sprint(s.Runs))
			}
			cell := func(v, ref float64, unit string) {
				imgui.TableNextColumn()
				if i == e.baselineArm || ref == 0 {
					imgui.Text(fmt.Sprintf("%.0f%s", v, unit))
					return
				}
				d := (v - ref) / ref * 100
				txt := fmt.Sprintf("%+.1f%%", d)
				switch {
				case d > 0.5:
					textGood(txt)
				case d < -0.5:
					textBad(txt)
				default:
					textDim(txt)
				}
			}
			cell(s.TX, base.TX, "")
			cell(s.RX, base.RX, "")
			cell(s.Reached, base.Reached, "")
			cell(s.Coll, base.Coll, "")
			cell(s.Airtime/1000, base.Airtime/1000, " s")
			cell(s.SpanMs/1000, base.SpanMs/1000, " s")
		}
		imgui.EndTable()
	}

	// The most important line in the view.
	if warn := e.notAResultYet(); warn != "" {
		imgui.Separator()
		textBadWrap("! " + warn)
	}
	if !e.running && len(e.results) >= e.runsTotal() && e.runsTotal() > 0 {
		imgui.Separator()
		v := a.verdictFor(e)
		if v.Difference {
			textGoodWrap(v.Headline)
		} else {
			textBadWrap(v.Headline)
			for _, l := range v.Investigation {
				textDimWrap("· " + l)
			}
		}
	}
}

// drawTimelines is the comparison at a glance: one cell per arm on a shared
// axis. Overlaying four floods produces a solid block; small multiples let the
// eye do the work.
func (a *App) drawTimelines() {
	e := a.ensureExperiment()
	if len(e.results) == 0 {
		textDimWrap("Receptions per second after the burst, one panel per arm, on a shared axis. " +
			"Runs appear here as they finish.")
		return
	}
	byArm := map[string][]int{}
	var order []string
	peak := 1
	for _, r := range e.results {
		if _, seen := byArm[r.Arm]; !seen {
			order = append(order, r.Arm)
			byArm[r.Arm] = make([]int, len(r.perSecond))
		}
		for i, v := range r.perSecond {
			if i < len(byArm[r.Arm]) {
				byArm[r.Arm][i] += v
				if byArm[r.Arm][i] > peak {
					peak = byArm[r.Arm][i]
				}
			}
		}
	}
	avail := imgui.ContentRegionAvail()
	cols := 2
	w := (avail.X - 12) / float32(cols)
	h := float32(90)
	for i, arm := range order {
		if i%cols != 0 {
			imgui.SameLine()
		}
		if imgui.BeginChildStrV(fmt.Sprintf("##tl%d", i), imgui.NewVec2(w, h+34),
			imgui.ChildFlagsFrameStyle, 0) {
			textDim(arm)
			series := byArm[arm]
			vals := make([]float32, len(series))
			for j, v := range series {
				vals[j] = float32(v)
			}
			if len(vals) > 0 {
				imgui.PlotHistogramFloatPtrV("##h", &vals[0], int32(len(vals)), 0, "",
					0, float32(peak), imgui.NewVec2(w-20, h-24), 4)
			}
		}
		imgui.EndChild()
	}
	textDim(fmt.Sprintf("0 to %d s after the burst, peak %d receptions/s, summed over seeds",
		len(e.results[0].perSecond), peak))
}
