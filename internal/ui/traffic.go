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
// runSeed is the seed every run is reproducible from.
func (a *App) runSeed() uint64 {
	if a.seed == 0 {
		a.seed = 4417
	}
	return a.seed
}

func (a *App) buildEngine() {
	// Listeners belong to the engine that made them; a rebuild invalidates the
	// bridges behind them.
	a.closeCompanions()
	if a.eng != nil {
		_ = a.eng.Close()
	}
	a.eng = engine.New(a.Terrain, engine.Config{
		FreqMHz: a.freqMHz, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: a.runSeed(),
		ExcessPathLossDB: a.excessLossDB,
	})
	for _, n := range a.Nodes {
		a.eng.Add(n, nil)
	}
	a.ensureConfig()
	a.eng.StaggerBoot = a.cfg.bootSpread
	a.playing = false
	a.scrubMs = 0
	if a.cfg.autoWarm {
		a.startWarm()
	}
}

// startWarm computes the link matrix in the background, all cores.
//
// Kicked on every engine rebuild, because a rebuild empties the cache — and an
// empty cache bills its 45,000 terrain profiles to whoever sends the first
// message, on the frame thread, which read as "runs fast until I send, then
// stuck". Warming pays the same bill up front, in parallel, with a progress
// figure in the toolbar.
func (a *App) startWarm() {
	if a.warmCancel != nil {
		a.warmCancel()
	}
	// Below a handful of nodes the lazy fill is invisible; warming would only
	// add goroutine churn to every placement click.
	if len(a.Nodes) < 10 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.warmCancel = cancel
	eng := a.eng
	a.warmDone.Store(0)
	a.warmTotal.Store(int64(len(a.Nodes) * (len(a.Nodes) - 1) / 2))
	go func() {
		defer cancel()
		eng.WarmLinks(ctx, func(done, total int) {
			a.warmDone.Store(int64(done))
			a.warmTotal.Store(int64(total))
		})
		a.warmDone.Store(a.warmTotal.Load())
	}()
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
	if a.speed == 0 {
		a.speed = 1
	}
	imgui.SetNextItemWidth(70)
	if imgui.BeginCombo("##speed", fmt.Sprintf("%gx", a.speed)) {
		for _, v := range []float32{0.25, 1, 4, 12} {
			if imgui.SelectableBool(fmt.Sprintf("%gx", v)) {
				a.speed = v
			}
		}
		imgui.EndCombo()
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Simulated time against wall time. 1x is what the real network\n" +
			"would be doing right now; every firmware process pays per tick, so a\n" +
			"large scenario at 12x is a large CPU bill.")
	}
	imgui.SameLine()
	imgui.TextDisabled(fmt.Sprintf("t = %.2f s", float64(a.eng.NowMs())/1000))

	if a.playing {
		// Steps owed = wall time elapsed x speed. Fractions carry over, and the
		// cap keeps a stall (a dragged window, a slow frame) from being repaid
		// as a burst that freezes the next frame too.
		speed := a.speed
		if a.companionAttached() {
			// A real client is attached, and its timeouts are in wall time.
			// ADR-0008 requires this coupling be visible rather than hidden,
			// which is what the toolbar's "1x LOCKED" is for.
			speed = 1
		}
		a.stepDebt += imgui.CurrentIO().DeltaTime() * speed * 1000 / float32(a.eng.Config.StepMs)
		if a.stepDebt > 60 {
			a.stepDebt = 60
		}
		if n := int(a.stepDebt); n > 0 {
			a.stepDebt -= float32(n)
			a.stepEngine(n)
		}
	}
}

func (a *App) stepEngine(ticks int) {
	for i := 0; i < ticks; i++ {
		// Scheduled traffic fires against the simulated clock, before the tick
		// it belongs to — so a send at 5.00 s is at 5.00 s in every run.
		a.runSchedule()
		if err := a.eng.Step(context.Background()); err != nil {
			a.status = err.Error()
			a.playing = false
			return
		}
	}
	a.scrubMs = a.eng.NowMs()
}

// evFilterKey is what the filtered index depends on.
type evFilterKey struct {
	count int
	scrub uint32
	show  [5]bool
}

// evClass buckets an event for filtering and colour, once.
type evClass int

const (
	evTx evClass = iota
	evRx
	evHalfDuplex
	evInterference
	evBelowFloor
)

func classify(ev engine.Event) evClass {
	switch ev.Kind {
	case "tx":
		return evTx
	case "rx":
		return evRx
	}
	switch {
	case strings.Contains(ev.Detail, "half duplex"):
		return evHalfDuplex
	case strings.Contains(ev.Detail, "interferer"):
		return evInterference
	default:
		return evBelowFloor
	}
}

// ensureEvFilters turns every filter on the first time anything asks.
func (a *App) ensureEvFilters() {
	if !a.evFilterInit {
		a.evFilterInit = true
		for i := range a.evShow {
			a.evShow[i] = true
		}
	}
}

// events is the engine ledger, snapshotted once per change rather than copied
// every frame: fifty thousand events copied sixty times a second is how a
// window dies of success.
func (a *App) events() []engine.Event {
	if a.eng == nil {
		return nil
	}
	if n := a.eng.EventCount(); n != len(a.evSnapshot) {
		a.evSnapshot = a.eng.Events()
	}
	return a.evSnapshot
}

// drawTimeline is the event log: every event the run has produced, filterable,
// and rendered only where the scrollbar actually is.
//
// Nothing is dropped. The old version showed the newest 400 rows and threw the
// scrollbar away; this one keeps the whole ledger and pays only for the rows on
// screen, which is the difference between "the table only holds so much" and a
// table that holds the run.
func (a *App) drawTimeline() {
	if a.eng == nil {
		return
	}
	events := a.events()
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

	// The filters. Ticks, not a mode: any combination, and the counts say what
	// each tick is hiding.
	a.ensureEvFilters()
	counts := [5]int{}
	for _, ev := range events {
		if ev.AtMs <= a.scrubMs {
			counts[classify(ev)]++
		}
	}
	for i, label := range [5]string{"sent", "received", "half duplex", "interference", "below floor"} {
		if i > 0 {
			imgui.SameLine()
		}
		imgui.Checkbox(fmt.Sprintf("%s (%d)", label, counts[i]), &a.evShow[i])
	}

	// Filtered index, rebuilt only when the inputs change.
	key := evFilterKey{count: len(events), scrub: a.scrubMs, show: a.evShow}
	if key != a.evKey {
		a.evKey = key
		a.evFiltered = a.evFiltered[:0]
		for i, ev := range events {
			if ev.AtMs <= a.scrubMs && a.evShow[classify(ev)] {
				a.evFiltered = append(a.evFiltered, i)
			}
		}
	}

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

	// The clipper renders the visible rows and skips the rest by height, so a
	// hundred-thousand-row ledger scrolls like a hundred-row one.
	clip := imgui.NewListClipper()
	clip.Begin(int32(len(a.evFiltered)))
	for clip.Step() {
		for row := clip.DisplayStart(); row < clip.DisplayEnd(); row++ {
			// Newest first: the most recent event is the one being asked about.
			ev := events[a.evFiltered[len(a.evFiltered)-1-int(row)]]
			imgui.TableNextRow()
			imgui.TableSetColumnIndex(0)
			// The whole row opens the packet. Dissecting what actually flew is
			// two clicks from noticing it in the log, which is where the
			// question gets asked.
			sel := a.pkt.selected && a.pkt.id == ev.PacketID
			if imgui.SelectableBoolPtrV(fmt.Sprintf("%.2f##ev%d", float64(ev.AtMs)/1000, row),
				&sel, imgui.SelectableFlagsSpanAllColumns, imgui.NewVec2(0, 0)) {
				a.pkt.id, a.pkt.selected = ev.PacketID, true
			}
			// The row's verbs, where the question gets asked.
			if imgui.BeginPopupContextItem() {
				if imgui.MenuItemBool("open packet") {
					a.pkt.id, a.pkt.selected = ev.PacketID, true
				}
				if imgui.MenuItemBool("follow this message") {
					a.pkt.id, a.pkt.selected, a.pkt.wantJourney = ev.PacketID, true, true
				}
				if ev.From != "" && ev.To != "" && ev.From != ev.To {
					if imgui.MenuItemBool("open both consoles") {
						a.openNodeWindow(ev.From)
						a.openNodeWindow(ev.To)
					}
				}
				imgui.EndPopup()
			}
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
	}
	clip.End()
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
		imgui.TextDisabled("no nodes - place some with the toolbar, or File > Import a network")
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
