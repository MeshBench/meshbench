package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/capture"
	"github.com/A13xB0/meshcoresim/internal/engine"
)

// buildEngine rebuilds the simulation from the current scenario.
//
// Rebuilt rather than mutated. A node moved on the map changes every path loss
// that involves it, and an engine that carried its old link cache forward would
// answer with the geometry of a network that no longer exists.
func (a *App) buildEngine() {
	if a.eng != nil {
		_ = a.eng.Close()
	}
	a.eng = engine.New(a.Terrain, engine.Config{
		FreqMHz: a.freqMHz, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 4417,
	})
	for _, n := range a.Nodes {
		a.eng.Add(n, nil)
	}
	a.playing = false
	a.scrubMs = 0
}

// drawRunControls is the transport: what the simulation is doing right now.
func (a *App) drawRunControls() {
	if a.eng == nil {
		a.buildEngine()
	}

	label := "run"
	if a.playing {
		label = "pause"
	}
	if imgui.Button(label) {
		a.playing = !a.playing
	}
	imgui.SameLine()
	if imgui.Button("step") {
		a.stepEngine(20)
	}
	imgui.SameLine()
	if imgui.Button("reset") {
		a.buildEngine()
	}

	imgui.SameLine()
	// Real firmware is a choice, because it needs a MeshCore build on the
	// machine and starting one process per node is not free. Without it the
	// channel, the collisions and the ledger are all still real; what is
	// missing is that the relay decisions are MeshCore's own.
	if a.eng.FirmwareCount() > 0 {
		imgui.TextDisabled(fmt.Sprintf("%d nodes on real firmware", a.eng.FirmwareCount()))
	} else if imgui.Button("run real firmware") {
		if err := a.eng.AttachNative(context.Background(), 4417); err != nil {
			a.status = err.Error()
		} else {
			a.status = fmt.Sprintf("%d nodes running MeshCore", a.eng.FirmwareCount())
		}
	}

	imgui.SameLine()
	// Sending from the selected node is how a scenario is exercised without
	// firmware attached. With firmware, the node decides for itself and this is
	// how a message is introduced from outside.
	if from, _ := a.Link(); from >= 0 {
		if imgui.Button("send from " + a.Nodes[from].Name) {
			a.eng.Inject(from, []byte(fmt.Sprintf("msg-%d", a.eng.NowMs())))
		}
	} else {
		imgui.TextDisabled("select a node to send from")
	}

	imgui.SameLine()
	imgui.TextDisabled(fmt.Sprintf("t = %.2f s", float64(a.eng.NowMs())/1000))

	if a.playing {
		// Twenty ticks a frame: fast enough that a flood plays at a watchable
		// speed and small enough that the window stays responsive.
		a.stepEngine(20)
	}
}

func (a *App) stepEngine(ticks int) {
	for i := 0; i < ticks; i++ {
		if err := a.eng.Step(context.Background()); err != nil {
			a.status = err.Error()
			a.playing = false
			return
		}
	}
	a.scrubMs = a.eng.NowMs()
}

// drawTimeline is the event log, scrubbable.
//
// A simulation that reports only final counts cannot answer "why did that not
// arrive", which is the only question anyone has. Every row here is one event
// with its cause in words.
func (a *App) drawTimeline() {
	if a.eng == nil {
		return
	}
	events := a.eng.Events()
	if len(events) == 0 {
		imgui.TextDisabled("no traffic yet - press run, or send from a selected node")
		return
	}

	now := a.eng.NowMs()
	scrub := int32(a.scrubMs)
	imgui.SetNextItemWidth(-120)
	if imgui.SliderInt("##scrub", &scrub, 0, int32(now)) {
		a.scrubMs = uint32(scrub)
		a.playing = false
	}
	imgui.SameLine()
	imgui.TextDisabled(fmt.Sprintf("%.2f s", float64(a.scrubMs)/1000))

	if !imgui.BeginTableV("##events", 5,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	imgui.TableSetupColumnV("t", imgui.TableColumnFlagsWidthFixed, 60, 0)
	imgui.TableSetupColumnV("from", imgui.TableColumnFlagsWidthFixed, 110, 0)
	imgui.TableSetupColumnV("to", imgui.TableColumnFlagsWidthFixed, 110, 0)
	imgui.TableSetupColumnV("SNR", imgui.TableColumnFlagsWidthFixed, 70, 0)
	imgui.TableSetupColumnV("what happened", imgui.TableColumnFlagsWidthStretch, 0, 0)
	imgui.TableHeadersRow()

	shown := 0
	for i := len(events) - 1; i >= 0 && shown < 400; i-- {
		ev := events[i]
		if ev.AtMs > a.scrubMs {
			continue
		}
		shown++
		imgui.TableNextRow()

		imgui.TableSetColumnIndex(0)
		imgui.Text(fmt.Sprintf("%.2f", float64(ev.AtMs)/1000))
		imgui.TableSetColumnIndex(1)
		imgui.Text(ev.From)
		imgui.TableSetColumnIndex(2)
		imgui.Text(ev.To)
		imgui.TableSetColumnIndex(3)
		if ev.Kind != "tx" {
			imgui.Text(fmt.Sprintf("%+.1f", ev.SNRdB))
		}
		imgui.TableSetColumnIndex(4)
		imgui.PushStyleColorVec4(imgui.ColText, eventColour(ev))
		text := ev.Detail
		if ev.Kind == "tx" {
			text = "transmitted: " + text
		}
		imgui.Text(text)
		imgui.PopStyleColor()
	}
	imgui.EndTable()
}

func eventColour(ev engine.Event) imgui.Vec4 {
	if ev.Kind == "tx" {
		return imgui.NewVec4(0.7, 0.75, 0.85, 1)
	}
	if ev.Outcome == capture.Accepted {
		return imgui.NewVec4(0.45, 0.85, 0.5, 1)
	}
	if ev.Kind == "miss" {
		// Not all misses are the same, and colouring them alike throws away the
		// distinction the engine went to trouble to make.
		if strings.Contains(ev.Detail, "half duplex") {
			return imgui.NewVec4(0.6, 0.65, 0.95, 1)
		}
		if strings.Contains(ev.Detail, "interferer") {
			return imgui.NewVec4(0.95, 0.72, 0.25, 1)
		}
		return imgui.NewVec4(0.85, 0.45, 0.45, 1)
	}
	return imgui.NewVec4(0.7, 0.75, 0.85, 1)
}

// drawScoreboard ranks nodes by what their airtime bought.
func (a *App) drawScoreboard() {
	if a.eng == nil {
		return
	}
	board := a.eng.Scoreboard()
	if len(board) == 0 {
		imgui.TextDisabled("no nodes")
		return
	}

	imgui.TextWrapped("Unique deliveries against redundant relays is the number that matters: " +
		"a repeater can be busy, legal, and reaching nobody who had not already heard the message.")
	imgui.Spacing()

	if !imgui.BeginTableV("##score", 6,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg, imgui.NewVec2(0, 0), 0) {
		return
	}
	for _, h := range []string{"node", "sent", "heard", "airtime", "duty", "unique / redundant"} {
		imgui.TableSetupColumnV(h, imgui.TableColumnFlagsWidthStretch, 0, 0)
	}
	imgui.TableHeadersRow()

	for _, s := range board {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(s.Name)
		imgui.TableSetColumnIndex(1)
		imgui.Text(fmt.Sprint(s.Sent))
		imgui.TableSetColumnIndex(2)
		imgui.Text(fmt.Sprint(s.Heard))
		imgui.TableSetColumnIndex(3)
		imgui.Text(fmt.Sprintf("%.0f ms", s.AirtimeMs))
		imgui.TableSetColumnIndex(4)
		// The legal limit in most of Europe is 1%. A node past it is not a
		// tuning problem, it is a compliance one, so it is coloured as such.
		col := imgui.NewVec4(0.7, 0.75, 0.85, 1)
		if s.DutyCyclePct > 1 {
			col = imgui.NewVec4(0.9, 0.4, 0.4, 1)
		}
		imgui.PushStyleColorVec4(imgui.ColText, col)
		imgui.Text(fmt.Sprintf("%.2f%%", s.DutyCyclePct))
		imgui.PopStyleColor()
		imgui.TableSetColumnIndex(5)
		if s.UniqueDelivery+s.RedundantRelay == 0 {
			imgui.TextDisabled("-")
		} else {
			ratio := float64(s.UniqueDelivery) / float64(s.UniqueDelivery+s.RedundantRelay)
			col := imgui.NewVec4(0.45, 0.85, 0.5, 1)
			if ratio < 0.34 {
				col = imgui.NewVec4(0.9, 0.4, 0.4, 1)
			} else if ratio < 0.67 {
				col = imgui.NewVec4(0.95, 0.72, 0.25, 1)
			}
			imgui.PushStyleColorVec4(imgui.ColText, col)
			imgui.Text(fmt.Sprintf("%d / %d", s.UniqueDelivery, s.RedundantRelay))
			imgui.PopStyleColor()
		}
	}
	imgui.EndTable()
}

// drawConsole is the selected node's serial console.
//
// A real UART, not a simulated one: the point of running real firmware is that
// its own command interface is how a repeater is configured, and a workbench
// that cannot reach it can build a mesh but not administer one.
func (a *App) drawConsole() {
	from, _ := a.Link()
	if from < 0 {
		imgui.TextDisabled("select a node")
		return
	}
	n := a.Nodes[from]
	imgui.Text(n.Name + " console")

	if a.eng == nil {
		return
	}
	var fw bool
	for _, en := range a.eng.Nodes() {
		if en.Spec.Name == n.Name && en.Firmware != nil {
			fw = true
		}
	}
	if !fw {
		imgui.TextWrapped("This node has no firmware attached, so there is no console to reach. " +
			"A console is a real UART on a running MeshCore build; without one there is nothing " +
			"on the other end, and a simulated prompt would be a lie about what is running.")
		imgui.Spacing()
		imgui.TextDisabled("Press \"run real firmware\" on the Traffic tab.")
		return
	}

	// What the running build reports about itself. This is the node's own
	// state, read back over the bridge — not MeshCore's full repeater CLI,
	// which lives in the repeater application we do not link
	// (docs/shortcomings.md 3.5).
	if en, ok := a.eng.NodeByName(n.Name); ok {
		imgui.TextDisabled(fmt.Sprintf("sent %d   heard %d   airtime %.0f ms",
			en.Sent, en.Heard, en.AirtimeMs))
	}

	imgui.InputTextWithHint("##cmd", "type a MeshCore command", &a.consoleInput, 0, nil)
	imgui.SameLine()
	if imgui.Button("send") {
		a.consoleLog = append(a.consoleLog, "> "+a.consoleInput)
		a.consoleInput = ""
	}
	if imgui.BeginChildStrV("##consolelog", imgui.NewVec2(0, 0), 0, 0) {
		for _, line := range a.consoleLog {
			imgui.Text(line)
		}
	}
	imgui.EndChild()
}
