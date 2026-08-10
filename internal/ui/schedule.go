package ui

import (
	"fmt"

	"github.com/A13xB0/meshcoresim/internal/engine"

	"github.com/AllenDang/cimgui-go/imgui"
)

// send is one scheduled transmission.
type send struct {
	node    string
	atMs    uint32
	everyMs uint32 // 0 for one-shot
	command string
	fired   uint32 // simulated time of the last firing, so repeats are not re-fired
	done    bool
}

// scheduleState is the send list plus the stress ramp.
type scheduleState struct {
	sends []send
	// draft is the row being composed.
	draftNode    string
	draftAtS     float32
	draftEveryS  float32
	draftCommand string

	// asserts is what must be true for this run to pass.
	asserts     []engine.Assertion
	draftAssert engine.Assertion

	// baseline is a snapshot to compare a later run against.
	baseline   []engine.Event
	baselineAt uint32

	// Stress: ramp the offered load until delivery falls over, and report the
	// knee rather than a pass/fail.
	stressOn      bool
	stressEveryMs uint32
	stressStepMs  uint32
	stressNext    uint32
}

func (a *App) drawScheduleBody() {
	s := &a.sched

	if s.draftNode == "" && len(a.Nodes) > 0 {
		s.draftNode = a.Nodes[0].Name
	}
	if s.draftCommand == "" {
		s.draftCommand = "advert"
	}

	imgui.SetNextItemWidth(150)
	if imgui.BeginCombo("from", s.draftNode) {
		for i := range a.Nodes {
			if a.Nodes[i].Kind.Transmits() && imgui.SelectableBool(a.Nodes[i].Name) {
				s.draftNode = a.Nodes[i].Name
			}
		}
		imgui.EndCombo()
	}
	imgui.SetNextItemWidth(90)
	imgui.InputFloat("at s", &s.draftAtS)
	imgui.SameLine()
	imgui.SetNextItemWidth(90)
	imgui.InputFloat("every s", &s.draftEveryS)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("0 for a one-shot")
	}
	imgui.SetNextItemWidth(-90)
	imgui.InputTextWithHint("##cmd", "a CLI line, e.g. advert", &s.draftCommand, 0, nil)
	imgui.SameLine()
	if imgui.Button("add") && s.draftNode != "" {
		s.sends = append(s.sends, send{
			node: s.draftNode, atMs: uint32(s.draftAtS * 1000),
			everyMs: uint32(s.draftEveryS * 1000), command: s.draftCommand,
		})
	}

	imgui.SeparatorText("Schedule")
	if len(s.sends) == 0 {
		imgui.TextDisabled("nothing scheduled; runs will be quiet unless the firmware sends on its own")
	}
	for i := 0; i < len(s.sends); i++ {
		e := s.sends[i]
		repeat := "once"
		if e.everyMs > 0 {
			repeat = fmt.Sprintf("every %.1f s", float64(e.everyMs)/1000)
		}
		imgui.Text(fmt.Sprintf("%.1f s  %-14s %-10s %s",
			float64(e.atMs)/1000, e.node, repeat, e.command))
		imgui.SameLine()
		if imgui.SmallButton(fmt.Sprintf("remove##s%d", i)) {
			s.sends = append(s.sends[:i], s.sends[i+1:]...)
			i--
		}
	}

	imgui.SeparatorText("Assertions")
	imgui.TextWrapped("What must be true for this run to count as a pass. Without one, a " +
		"comparison between two firmware versions is a human reading two logs.")

	imgui.SetNextItemWidth(150)
	if imgui.BeginCombo("##akind", string(s.draftAssert.Kind)) {
		for _, k := range []engine.AssertKind{
			engine.AssertReceives, engine.AssertDelivered,
			engine.AssertDutyBelow, engine.AssertRelaysAtMost,
		} {
			if imgui.SelectableBool(string(k)) {
				s.draftAssert.Kind = k
			}
		}
		imgui.EndCombo()
	}
	imgui.SameLine()
	switch s.draftAssert.Kind {
	case engine.AssertReceives:
		imgui.SetNextItemWidth(120)
		if imgui.BeginCombo("node##a", s.draftAssert.Node) {
			for i := range a.Nodes {
				if imgui.SelectableBool(a.Nodes[i].Name) {
					s.draftAssert.Node = a.Nodes[i].Name
				}
			}
			imgui.EndCombo()
		}
		imgui.SameLine()
		within := int32(s.draftAssert.WithinMs / 1000)
		imgui.SetNextItemWidth(80)
		if imgui.InputInt("s##a", &within) {
			s.draftAssert.WithinMs = uint32(within) * 1000
		}
	case engine.AssertDelivered:
		v := int32(s.draftAssert.AtLeast)
		imgui.SetNextItemWidth(80)
		if imgui.InputInt("at least##a", &v) {
			s.draftAssert.AtLeast = int(v)
		}
	case engine.AssertDutyBelow:
		v := float32(s.draftAssert.MaxPct)
		imgui.SetNextItemWidth(80)
		if imgui.InputFloat("max %##a", &v) {
			s.draftAssert.MaxPct = float64(v)
		}
	case engine.AssertRelaysAtMost:
		imgui.SetNextItemWidth(120)
		if imgui.BeginCombo("node##ar", s.draftAssert.Node) {
			for i := range a.Nodes {
				if imgui.SelectableBool(a.Nodes[i].Name) {
					s.draftAssert.Node = a.Nodes[i].Name
				}
			}
			imgui.EndCombo()
		}
		imgui.SameLine()
		v := int32(s.draftAssert.AtMost)
		imgui.SetNextItemWidth(80)
		if imgui.InputInt("at most##a", &v) {
			s.draftAssert.AtMost = int(v)
		}
	}
	imgui.SameLine()
	if imgui.Button("add##assert") && s.draftAssert.Kind != "" {
		s.asserts = append(s.asserts, s.draftAssert)
	}

	if len(s.asserts) > 0 && a.eng != nil {
		results := a.eng.Check(s.asserts)
		passed := 0
		for i, r := range results {
			col := imgui.NewVec4(0.9, 0.4, 0.4, 1)
			verdict := "FAIL"
			if r.Passed {
				col, verdict, passed = imgui.NewVec4(0.45, 0.85, 0.5, 1), "PASS", passed+1
			}
			imgui.PushStyleColorVec4(imgui.ColText, col)
			imgui.Text(verdict)
			imgui.PopStyleColor()
			imgui.SameLine()
			imgui.Text(r.Assertion.String())
			imgui.SameLine()
			imgui.TextDisabled("- " + r.Detail)
			imgui.SameLine()
			if imgui.SmallButton(fmt.Sprintf("remove##as%d", i)) {
				s.asserts = append(s.asserts[:i], s.asserts[i+1:]...)
				break
			}
		}
		imgui.TextDisabled(fmt.Sprintf("%d of %d passing at %.1f s",
			passed, len(results), float64(a.engNowMs())/1000))
	}
}

// engNowMs is the simulated clock, or zero before an engine exists.
func (a *App) engNowMs() uint32 {
	if a.eng == nil {
		return 0
	}
	return a.eng.NowMs()
}

// runSchedule fires anything now due. Called once per engine step.
//
// Commands go through each node's own CLI rather than an injection API, so a
// scheduled send is the same event a person typing would produce — including
// the firmware's own delays before it reaches the air.
func (a *App) runSchedule() {
	if a.eng == nil {
		return
	}
	now := a.eng.NowMs()
	s := &a.sched

	for i := range s.sends {
		e := &s.sends[i]
		if e.done || now < e.atMs {
			continue
		}
		if e.fired != 0 && (e.everyMs == 0 || now-e.fired < e.everyMs) {
			continue
		}
		if err := a.typeAt(e.node, e.command); err != nil {
			// A node without firmware cannot be scheduled at. Said once and
			// then retired, rather than every tick for the rest of the run.
			a.status = fmt.Sprintf("scheduled send to %s: %v", e.node, err)
			e.done = true
			continue
		}
		e.fired = now
		if e.everyMs == 0 {
			e.done = true
		}
	}

	if s.stressOn && now >= s.stressNext {
		// Every node that can transmit, so the load is the network's rather
		// than one node's — a stress test that hammers a single repeater
		// measures that repeater's duty cycle, not the mesh's capacity.
		for i := range a.Nodes {
			if a.Nodes[i].Kind.Transmits() {
				_ = a.typeAt(a.Nodes[i].Name, "advert")
			}
		}
		// Interval halves each round: a linear ramp spends most of its time in
		// the regime that already works.
		if s.stressEveryMs > 500 {
			s.stressEveryMs /= 2
		}
		s.stressNext = now + s.stressEveryMs
	}
}

// drawCompareBody is the Compare panel: baseline divergence and the stress
// ramp. Both are measurements of a run, which is why they left the Schedule
// window — a schedule says what will happen, these say what it meant.
func (a *App) drawCompareBody() {
	s := &a.sched
	if a.eng == nil {
		imgui.TextDisabled("no simulation yet - press play in the strip above")
		return
	}
	imgui.SeparatorText("Compare with a baseline")
	imgui.TextWrapped("Snapshot this run, change something - firmware, settings, geometry - " +
		"then run again with the same seed and see where the two first disagree.")
	if imgui.Button("snapshot as baseline") && a.eng != nil {
		s.baseline = a.eng.Events()
		s.baselineAt = a.engNowMs()
	}
	if len(s.baseline) > 0 {
		imgui.SameLine()
		imgui.TextDisabled(fmt.Sprintf("baseline: %d events to %.1f s",
			len(s.baseline), float64(s.baselineAt)/1000))
		if a.eng != nil {
			d := engine.Diverge(s.baseline, a.eng.Events())
			if !d.Found {
				imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.45, 0.85, 0.5, 1))
				imgui.TextWrapped("identical so far")
				imgui.PopStyleColor()
			} else {
				imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
				imgui.TextWrapped(fmt.Sprintf("first divergence at %.2f s, %s",
					float64(d.AtMs)/1000, d.Node))
				imgui.PopStyleColor()
				imgui.TextDisabled("baseline: " + d.A)
				imgui.TextDisabled("now:      " + d.B)
			}
		}
	}

	imgui.SeparatorText("Stress test")
	imgui.TextWrapped("Ramps the offered load until delivery stops keeping up, and reports the " +
		"knee - how much traffic this particular network can actually carry.")
	if s.stressOn {
		imgui.Text(fmt.Sprintf("running: one message every %.1f s", float64(s.stressEveryMs)/1000))
		if imgui.Button("stop") {
			s.stressOn = false
		}
	} else if imgui.Button("start ramp") {
		s.stressOn = true
		s.stressEveryMs, s.stressStepMs = 10_000, 0
		s.stressNext = a.engNowMs()
	}

	a.drawABSection()
}
