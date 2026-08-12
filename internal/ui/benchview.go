package ui

import (
	"fmt"
	"github.com/A13xB0/meshcoresim/internal/scenario"
	"math"
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
	return &experiment{
		// Senders are left empty on purpose: choosing which companions
		// originate the traffic is the operator's decision, and quietly picking
		// six is how an experiment ends up measuring something nobody asked for.
		// The picker offers "spread across the network" for the common case.
		// One neutral arm: whatever the scenario already says. Varying a
		// parameter replaces it, so the first gesture produces clean labels
		// rather than crossing onto a guess nobody asked for.
		Arms:     []expArm{{Label: "baseline"}},
		Seeds:    []uint64{4417, 9001},
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

	// What is being tested, and on what. These were read-only for a while and
	// simply appeared filled in, which is the same silent-configuration fault
	// this whole view exists to guard against: an operator could not tell where
	// the senders came from, let alone choose different ones.
	textDim("scenario")
	imgui.SameLine()
	imgui.SetNextItemWidth(190)
	label := a.benchUI.project
	if label == "" {
		label = fmt.Sprintf("this scenario, %d nodes", len(a.Nodes))
	}
	if imgui.BeginCombo("##project", label) {
		for _, pr := range listProjects() {
			if imgui.SelectableBool(pr.name) {
				if err := a.openProject(pr.name); err != nil {
					a.status = err.Error()
				} else {
					a.benchUI.project = pr.name
					e.Senders = nil // they belonged to the network being replaced
				}
			}
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	textDim(fmt.Sprintf("%d nodes", len(a.Nodes)))

	// Senders as a list with a remove on each row, not a dropdown that only
	// ever adds. Choosing who transmits is the experiment's design, and a
	// control you cannot undo in is the wrong shape for it.
	textDim("senders")
	imgui.SameLine()
	if imgui.SmallButton("spread 6") {
		e.Senders = a.spreadSenders(6)
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Six companions as far apart as the network allows.\n\n" +
			"A cluster of neighbours contends with itself rather than with the mesh,\n" +
			"which is not the contention worth measuring.")
	}
	imgui.SameLine()
	if imgui.SmallButton("all") {
		e.Senders = nil
		for i := range a.Nodes {
			if a.Nodes[i].Kind == scenario.Companion {
				e.Senders = append(e.Senders, a.Nodes[i].Name)
			}
		}
	}
	imgui.SameLine()
	if imgui.SmallButton("none") {
		e.Senders = nil
	}
	imgui.SameLine()
	imgui.SetNextItemWidth(150)
	if imgui.BeginCombo("##addsender", "add one") {
		for i := range a.Nodes {
			if a.Nodes[i].Kind != scenario.Companion || containsStr(e.Senders, a.Nodes[i].Name) {
				continue
			}
			if imgui.SelectableBool(a.Nodes[i].Name) {
				e.Senders = append(e.Senders, a.Nodes[i].Name)
			}
		}
		imgui.EndCombo()
	}
	if len(e.Senders) == 0 {
		textBadWrap("An experiment needs a companion to originate traffic.")
	}
	for i := 0; i < len(e.Senders); i++ {
		imgui.PushIDInt(int32(1000 + i))
		if imgui.SmallButton("x") {
			e.Senders = append(e.Senders[:i], e.Senders[i+1:]...)
			imgui.PopID()
			i--
			continue
		}
		imgui.SameLine()
		imgui.TextUnformatted(e.Senders[i])
		imgui.PopID()
	}

	textDim("fired")
	imgui.SameLine()
	spread := int32(e.SpreadMs / 1000)
	imgui.SetNextItemWidth(80)
	if imgui.InputIntV("##spread", &spread, 0, 0, 0) && spread >= 0 {
		e.SpreadMs = uint32(spread) * 1000
	}
	imgui.SameLine()
	if e.SpreadMs == 0 {
		textDim("s apart (0 = all on the same instant)")
	} else {
		textDim("s apart")
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("All at once is maximum contention - every sender talking over every\n" +
			"other. Spreading them is the same traffic arriving politely, and the\n" +
			"difference between the two is often larger than any firmware setting.")
	}

	textDim("message size")
	imgui.SameLine()
	size := int32(e.Bytes)
	imgui.SetNextItemWidth(80)
	if imgui.InputIntV("##bytes", &size, 0, 0, 0) && size >= 0 {
		e.Bytes = int(size)
	}
	imgui.SameLine()
	if e.Bytes == 0 {
		textDim("bytes (0 = a short label)")
	} else {
		textDim("bytes")
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Airtime scales with payload, and airtime is what collides.\n\n" +
			"A 12-byte message and a 180-byte one are different experiments; vary this\n" +
			"to ask whether a setting helps more when the channel is busier.")
	}

	textDim("observer")
	imgui.SameLine()
	imgui.SetNextItemWidth(190)
	obs := e.Observer
	if obs == "" {
		obs = "none"
	}
	if imgui.BeginCombo("##observer", obs) {
		if imgui.SelectableBool("none") {
			e.Observer = ""
		}
		for i := range a.Nodes {
			if a.Nodes[i].Kind != scenario.Companion || containsStr(e.Senders, a.Nodes[i].Name) {
				continue
			}
			if imgui.SelectableBool(a.Nodes[i].Name) {
				e.Observer = a.Nodes[i].Name
			}
		}
		imgui.EndCombo()
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("A companion that only listens.\n\n" +
			"The matrix answers how much of the mesh a message reached; this answers\n" +
			"the question a person actually asks - did it arrive?")
	}

	textDim("on channel")
	imgui.SameLine()
	imgui.SetNextItemWidth(110)
	if imgui.BeginCombo("##chan", e.Channel) {
		for _, r := range a.knownRegions() {
			if imgui.SelectableBool(r) {
				e.Channel = r
			}
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	textDim("scoped")
	imgui.SameLine()
	imgui.SetNextItemWidth(110)
	scopeLabel := e.Scope
	if scopeLabel == "" {
		scopeLabel = "unscoped"
	}
	if imgui.BeginCombo("##scope", scopeLabel) {
		if imgui.SelectableBool("unscoped") {
			e.Scope = ""
		}
		for _, r := range a.knownRegions() {
			if imgui.SelectableBool(r) {
				e.Scope = r
			}
		}
		imgui.EndCombo()
	}

	textDim("fire at")
	imgui.SameLine()
	sendS := int32(e.SendAtMs / 1000)
	imgui.SetNextItemWidth(70)
	if imgui.InputIntV("##sendat", &sendS, 0, 0, 0) && sendS > 0 {
		e.SendAtMs = uint32(sendS) * 1000
	}
	imgui.SameLine()
	textDim("s, measure for")
	imgui.SameLine()
	runS := int32(e.RunForMs / 1000)
	imgui.SetNextItemWidth(70)
	if imgui.InputIntV("##runfor", &runS, 0, 0, 0) && runS > 0 {
		e.RunForMs = uint32(runS) * 1000
	}
	imgui.SameLine()
	textDim("s")
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Every arm fires at the same simulated instant.\n\n" +
			"Setting up the senders costs a different number of engine steps each run,\n" +
			"so \"settle then send\" lands the burst at a different moment in each arm -\n" +
			"a different advert phase, and a different result.")
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
		for _, p := range []string{"companion path hash", "repeater path hash",
			"loop.detect", "cad", "firmware", "spread"} {
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
	imgui.TextUnformatted(fmt.Sprintf("%d arms x %d seeds = %d runs, about %s",
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
	if a.PathHashMode != nil {
		parts = append(parts, fmt.Sprintf("companion sends %d-byte", *a.PathHashMode+1))
	}
	if a.RepPathHash != nil {
		parts = append(parts, fmt.Sprintf("repeaters %d-byte", *a.RepPathHash+1))
	}
	if a.LoopDetect != "" {
		parts = append(parts, "loop "+a.LoopDetect)
	}
	if a.CAD != "" {
		parts = append(parts, "cad "+a.CAD)
	}
	if a.SpreadMs != nil {
		if *a.SpreadMs == 0 {
			parts = append(parts, "all at once")
		} else {
			parts = append(parts, fmt.Sprintf("over %.0fs", float64(*a.SpreadMs)/1000))
		}
	}
	return strings.Join(parts, " · ")
}

func defaultValuesFor(p string) string {
	switch p {
	case "companion path hash", "repeater path hash":
		return "0, 1, 2"
	case "loop.detect":
		return "off, minimal, moderate, strict"
	case "cad":
		return "off, on"
	case "firmware":
		return "1.16.0, 1.17.0"
	case "spread":
		return "0, 5, 20"
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
		base = []expArm{{}}
	}
	var out []expArm
	for _, b := range base {
		for _, v := range vals {
			arm := b
			switch param {
			case "companion path hash":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 || n > 2 {
					continue
				}
				arm.PathHashMode = i32(int32(n))
				arm.Label = joinLabel(b.Label, fmt.Sprintf("%d-byte", n+1))
			case "repeater path hash":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 || n > 2 {
					continue
				}
				arm.RepPathHash = i32(int32(n))
				arm.Label = joinLabel(b.Label, fmt.Sprintf("rpt %d-byte", n+1))
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
			case "spread":
				n, err := strconv.Atoi(v)
				if err != nil || n < 0 {
					continue
				}
				arm.SpreadMs = i32(int32(n) * 1000)
				if n == 0 {
					arm.Label = joinLabel(b.Label, "all at once")
				} else {
					arm.Label = joinLabel(b.Label, fmt.Sprintf("over %ds", n))
				}
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
		imgui.TableFlagsScrollY|imgui.TableFlagsResizable|imgui.TableFlagsSizingFixedFit,
		imgui.NewVec2(0, 0), 0) {
		imgui.TableSetupColumnV("arm", imgui.TableColumnFlagsWidthFixed, 0, 0)
		imgui.TableSetupColumnV("seed", imgui.TableColumnFlagsWidthFixed, 0, 0)
		imgui.TableSetupColumnV("state", imgui.TableColumnFlagsWidthFixed, 0, 0)
		imgui.TableSetupColumnV("result", imgui.TableColumnFlagsWidthStretch, 0, 0)
		imgui.TableHeadersRow()

		done := len(e.results)
		i := 0
		for ai, arm := range e.Arms {
			for _, seed := range e.Seeds {
				imgui.TableNextRow()
				imgui.TableNextColumn()
				imgui.TextUnformatted(arm.Label)
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
						imgui.TextUnformatted(fmt.Sprintf("%d msgs  R %d/%d  C %d/%d",
							r.Messages, r.RepHit, r.RepChances, r.CompHit, r.CompChances))
						if len(r.RepPerMsg) > 0 {
							// Each message on its own, so one that got nowhere
							// is visible rather than averaged away.
							imgui.SameLine()
							var parts []string
							for i := range r.RepPerMsg {
								parts = append(parts, fmt.Sprintf("%d+%d", r.RepPerMsg[i], r.CompPerMsg[i]))
							}
							textDim("[" + strings.Join(parts, " ") + "]")
						}
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
		imgui.TextUnformatted(e.status)
		imgui.Separator()
	}
	if imgui.BeginChildStr("##explog") {
		pushMono()
		for _, l := range e.log {
			imgui.TextUnformatted(l)
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

	// Sized to their contents, draggable, and left alone afterwards: imgui
	// keeps a resized width in its settings, so a column widened once does not
	// snap back every time a run finishes and adds a row.
	if imgui.BeginTableV("##matrix", 11, imgui.TableFlagsRowBg|imgui.TableFlagsBordersInnerH|
		imgui.TableFlagsResizable|imgui.TableFlagsSizingFixedFit|imgui.TableFlagsNoHostExtendX,
		imgui.NewVec2(0, 0), 0) {
		cols := []string{"arm", "runs", "tx", "to repeaters", "to companions",
			"msgs", "dupes", "worst dupe", "collisions", "airtime", "to quiet"}
		for i, c := range cols {
			flags := imgui.TableColumnFlagsWidthFixed
			if i == len(cols)-1 {
				// The last one takes the slack, so there is never a ragged gap
				// at the right edge.
				flags = imgui.TableColumnFlagsWidthStretch
			}
			imgui.TableSetupColumnV(c, flags, 0, 0)
		}
		imgui.TableHeadersRow()
		for i, s := range sums {
			imgui.TableNextRow()
			imgui.TableNextColumn()
			imgui.TextUnformatted(s.Arm)
			imgui.TableNextColumn()
			if s.Flagged > 0 {
				textBad(fmt.Sprintf("%d (%d flagged)", s.Runs, s.Flagged))
			} else {
				imgui.Text(fmt.Sprint(s.Runs))
			}
			cell := func(v, ref float64, unit string) {
				imgui.TableNextColumn()
				if i == e.baselineArm || ref == 0 {
					imgui.TextUnformatted(fmt.Sprintf("%.0f%s", v, unit))
					return
				}
				d := (v - ref) / ref * 100
				txt := fmt.Sprintf("%+.1f%%", d)
				// Every metric this renders - transmissions, duplicates,
				// collisions, airtime, time to quiet - is one where less is
				// better. Green for "went up" read as approval and it was
				// pointing the wrong way on all five: an arm that cost 8% more
				// airtime showed the increase in the colour used for progress.
				switch {
				case d > 0.5:
					textBad(txt)
				case d < -0.5:
					textGood(txt)
				default:
					textDim(txt)
				}
			}
			cell(s.TX, base.TX, "")
			// Delivery, split: reaching the repeaters is whether the mesh
			// carried it, reaching the companions is whether anybody read it.
			imgui.TableNextColumn()
			imgui.TextUnformatted(fmt.Sprintf("%.1f%%", s.RepPct))
			imgui.TableNextColumn()
			imgui.TextUnformatted(fmt.Sprintf("%.1f%%", s.CompPct))
			imgui.TableNextColumn()
			imgui.TextUnformatted(fmt.Sprintf("%.0f", s.Messages))
			cell(s.Dupe, base.Dupe, "")
			imgui.TableNextColumn()
			imgui.TextUnformatted(fmt.Sprintf("%.1f", s.WorstDupe))
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

// spreadSenders picks companions as far apart as the network allows.
//
// The obvious choice - the first few in the list - gave four neighbours in
// Angus, which contend with each other rather than with the mesh. Contention
// between distant senders is the thing worth measuring.
func (a *App) spreadSenders(n int) []string {
	var comps []int
	for i := range a.Nodes {
		if a.Nodes[i].Kind == scenario.Companion {
			comps = append(comps, i)
		}
	}
	if len(comps) == 0 {
		return nil
	}
	chosen := []int{comps[0]}
	for len(chosen) < n && len(chosen) < len(comps) {
		best, bestD := -1, -1.0
		for _, c := range comps {
			if containsInt(chosen, c) {
				continue
			}
			d := math.Inf(1)
			for _, s := range chosen {
				dx := a.Nodes[c].Position.Lat - a.Nodes[s].Position.Lat
				dy := (a.Nodes[c].Position.Lon - a.Nodes[s].Position.Lon) * 0.55
				d = math.Min(d, math.Hypot(dx, dy))
			}
			if d > bestD {
				best, bestD = c, d
			}
		}
		if best < 0 {
			break
		}
		chosen = append(chosen, best)
	}
	var out []string
	for _, i := range chosen {
		out = append(out, a.Nodes[i].Name)
	}
	return out
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// drawBenchConfig shows what each arm will actually apply, and what the network
// it is applied to looks like.
//
// Without it the sweep is a set of labels: an operator cannot tell what the
// repeaters are set to, which is precisely how a run gets misread as an RF
// result when it was a configuration that never landed.
func (a *App) drawBenchConfig() {
	e := a.ensureExperiment()
	a.ensureConfig()

	textDim("the network")
	nRep, nComp, withRegions := 0, 0, 0
	regions := map[string]int{}
	for i := range a.Nodes {
		switch a.Nodes[i].Kind {
		case scenario.Companion:
			nComp++
		default:
			nRep++
		}
		if len(a.Nodes[i].Regions) > 0 {
			withRegions++
			for _, r := range a.Nodes[i].Regions {
				regions[r]++
			}
		}
	}
	imgui.TextUnformatted(fmt.Sprintf("%d repeaters, %d companions", nRep, nComp))
	imgui.TextUnformatted(fmt.Sprintf("%d nodes carry transport regions", withRegions))
	if len(regions) > 0 {
		var parts []string
		for _, r := range a.knownRegions() {
			if n := regions[r]; n > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", r, n))
			}
		}
		textDimWrap(strings.Join(parts, "   "))
	}
	if withRegions == 0 {
		textBadWrap("No node carries a region. On a transport-scoped mesh nothing will relay " +
			"scoped traffic - run the inference in Plan, or send unscoped.")
	}

	imgui.Separator()
	textDim("what every arm holds constant")
	// Editable, because "make sure every repeater is on minimal loop detect"
	// is a statement about the experiment's conditions rather than about any
	// one arm, and repeating it in every arm is how it gets forgotten in one.
	imgui.SetNextItemWidth(150)
	loop := e.Base.LoopDetect
	if loop == "" {
		loop = "leave as the scenario has it"
	}
	if imgui.BeginCombo("loop.detect", loop) {
		if imgui.SelectableBool("leave as the scenario has it") {
			e.Base.LoopDetect = ""
		}
		for _, v := range []string{"off", "minimal", "moderate", "strict"} {
			if imgui.SelectableBool(v) {
				e.Base.LoopDetect = v
			}
		}
		imgui.EndCombo()
	}
	imgui.SetNextItemWidth(150)
	hash := "leave as the scenario has it"
	if e.Base.RepPathHash != nil {
		hash = fmt.Sprintf("%d byte(s) per hop", *e.Base.RepPathHash+1)
	}
	if imgui.BeginCombo("repeater path hash", hash) {
		if imgui.SelectableBool("leave as the scenario has it") {
			e.Base.RepPathHash = nil
		}
		for m := int32(0); m <= 2; m++ {
			if imgui.SelectableBool(fmt.Sprintf("%d byte(s) per hop", m+1)) {
				e.Base.RepPathHash = i32(m)
			}
		}
		imgui.EndCombo()
	}
	textDimWrap("A repeater's own setting only affects the adverts it originates. What a " +
		"message carries is stamped by the companion that sent it, and every hop honours " +
		"that - so vary the companion's and hold this one still.")
	imgui.SetNextItemWidth(150)
	cad := e.Base.CAD
	if cad == "" {
		cad = "leave as the scenario has it"
	}
	if imgui.BeginCombo("cad", cad) {
		if imgui.SelectableBool("leave as the scenario has it") {
			e.Base.CAD = ""
		}
		for _, v := range []string{"off", "on"} {
			if imgui.SelectableBool(v) {
				e.Base.CAD = v
			}
		}
		imgui.EndCombo()
	}
	textDimWrap("Applied to every arm before its own settings, so an arm overrides only what " +
		"it names. Hardware CAD is off by default in both 1.16 and 1.17.")

	imgui.Separator()
	textDim("the radio, from the scenario")
	if len(a.Nodes) > 0 {
		r := a.Nodes[0].Radio
		imgui.TextUnformatted(fmt.Sprintf("%.3f MHz  SF%d  %.0f kHz  CR4/%d",
			r.CentreHz/1e6, r.SpreadFactor, r.BandwidthHz/1000, r.CodingRate+4))
	}
	imgui.TextUnformatted(fmt.Sprintf("clock set on boot: %v", a.cfg.setClockOnStart))
	imgui.TextUnformatted(fmt.Sprintf("names and positions set: %v", a.cfg.setNameOnStart))

	imgui.Separator()
	textDim("what each arm changes")
	if imgui.BeginTableV("##armcfg", 5, imgui.TableFlagsRowBg|imgui.TableFlagsBordersInnerH|
		imgui.TableFlagsResizable|imgui.TableFlagsSizingFixedFit,
		imgui.NewVec2(0, 0), 0) {
		cfgCols := []string{"arm", "repeater fw", "companion fw", "companion sends", "loop / cad"}
		for i, c := range cfgCols {
			flags := imgui.TableColumnFlagsWidthFixed
			if i == len(cfgCols)-1 {
				flags = imgui.TableColumnFlagsWidthStretch
			}
			imgui.TableSetupColumnV(c, flags, 0, 0)
		}
		imgui.TableHeadersRow()
		for _, arm := range e.Arms {
			imgui.TableNextRow()
			imgui.TableNextColumn()
			imgui.TextUnformatted(arm.Label)
			imgui.TableNextColumn()
			textOrDash(arm.RepeaterVersion)
			imgui.TableNextColumn()
			textOrDash(arm.CompanionVersion)
			imgui.TableNextColumn()
			if arm.PathHashMode == nil {
				textDim("unchanged")
			} else {
				imgui.TextUnformatted(fmt.Sprintf("%d byte(s)/hop", *arm.PathHashMode+1))
			}
			imgui.TableNextColumn()
			both := strings.TrimSpace(arm.LoopDetect + " " + arm.CAD)
			textOrDash(both)
		}
		imgui.EndTable()
	}
	textDimWrap("Anything shown as - is left exactly as the scenario has it, so the difference " +
		"between two rows is the experiment's independent variable.")
}

func textOrDash(s string) {
	if s == "" {
		textDim("-")
		return
	}
	imgui.Text(s)
}
