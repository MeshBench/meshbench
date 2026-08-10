package ui

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// drawNodeInspector is the one place a node's settings are drawn.
//
// The Inspector panel and every node window's Settings tab call this same
// function, so a node's settings look identical everywhere they appear — two
// hand-maintained copies had already drifted (the panel had no radio preset,
// the window had no ground elevation).
func (a *App) drawNodeInspector(i int) {
	if i < 0 || i >= len(a.Nodes) {
		return
	}
	n := &a.Nodes[i]

	imgui.Text(n.Name)
	imgui.SameLine()
	textDim(kindLabel(n.Kind))
	if n.PublicKey != "" {
		textDim("key " + shortPubKey(n.PublicKey))
	}
	imgui.Text(fmt.Sprintf("%.5f, %.5f", n.Position.Lat, n.Position.Lon))
	if n.UncertaintyKm > 0.2 {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		textWrap(fmt.Sprintf("position known to +/-%.1f km - results involving "+
			"this node carry that", n.UncertaintyKm))
		imgui.PopStyleColor()
	}

	changed := numF64("height m", &n.HeightAGLm, 0, 300, "%.1f")
	if n.Kind.Transmits() {
		changed = numF64("tx dBm", &n.TxPowerDBm, 2, 30, "%.0f") || changed
	} else {
		textDim("an SDR observer transmits nothing")
	}
	if changed {
		// Recompute rather than cache. The height slider is the whole point of
		// the panel — "what would five more metres buy" — and an answer that
		// lags the slider by a frame is an answer to the previous question.
		a.recompute()
	}
	if ground, ok := a.Terrain.ElevationM(n.Position.Lat, n.Position.Lon); ok {
		textDim(fmt.Sprintf("ground %.0f m, antenna top %.0f m AMSL",
			ground, ground+n.HeightAGLm))
	} else {
		textDim("no terrain here - download tiles for this area")
	}

	if n.Kind.Transmits() {
		imgui.SeparatorText("Hardware")
		a.drawBoardCombo(n)
		imgui.SeparatorText("Radio")
		a.drawPresetCombo(n)
	}
	if n.Kind == scenario.Emitter {
		a.drawEmitterControls(n)
		return
	}

	// Region facts: what was observed of the real node, shown because they are
	// what the firmware will be told on boot.
	if len(n.Regions) > 0 || n.DefaultScope != "" || n.FloodMaxSeen > 0 {
		imgui.SeparatorText("Regions (observed)")
		if len(n.Regions) > 0 {
			imgui.Text("holds: " + strings.Join(n.Regions, ", "))
		}
		if n.DefaultScope != "" {
			imgui.Text("default scope: " + n.DefaultScope)
		}
		if n.FloodMaxSeen > 0 {
			textDim(fmt.Sprintf("seen relaying %d hops - a floor on its flood.max",
				n.FloodMaxSeen))
		}
	}

	imgui.SeparatorText("Firmware")
	a.drawFirmwarePicker(n)

	if n.Kind.Transmits() && a.cfg.energyEnabled {
		imgui.Spacing()
		if imgui.SmallButton("does it survive December?") {
			if p := a.panelByName("Energy"); p != nil {
				p.open = true
				imgui.SetWindowFocusStr("Energy")
			}
		}
	}
}

func shortPubKey(k string) string {
	if len(k) > 12 {
		return k[:12] + "..."
	}
	return k
}

// drawSelected is the Inspector panel: the selection and nothing else.
func (a *App) drawSelected() {
	imgui.Spacing()
	if len(a.msel) > 1 {
		a.drawBulkEditor()
		return
	}
	imgui.SeparatorText("Selected")
	if a.selected < 0 {
		textDim("click a node on the map to inspect it")
		textDim("ctrl-click a second node for a link")
		textDim("shift-click nodes to edit several at once")
		return
	}
	a.drawNodeInspector(a.selected)
}

// drawBulkEditor edits the shared subset of a multi-selection.
//
// Only what every selected node has in common is offered — height, transmit
// power, radio preset. Anything per-identity (name, position, firmware role)
// stays per-node, because "set 12 nodes' names" is not an operation with a
// right answer.
func (a *App) drawBulkEditor() {
	imgui.SeparatorText(fmt.Sprintf("Selected - %d nodes", len(a.msel)))
	kinds := map[string]int{}
	for _, i := range a.msel {
		if i >= 0 && i < len(a.Nodes) {
			kinds[kindLabel(a.Nodes[i].Kind)]++
		}
	}
	var parts []string
	for k, c := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s", c, k))
	}
	textDim(strings.Join(parts, ", "))
	textDim("edits apply to every selected node")

	// The sliders start from the first selected node's values; dragging writes
	// to all of them. A blank control would give the drag nowhere to start.
	first := &a.Nodes[a.msel[0]]
	h := first.HeightAGLm
	if numF64("height m", &h, 0, 300, "%.1f") {
		for _, i := range a.msel {
			a.Nodes[i].HeightAGLm = h
		}
		a.recompute()
	}
	tx := first.TxPowerDBm
	if numF64("tx dBm", &tx, 2, 30, "%.0f") {
		for _, i := range a.msel {
			if a.Nodes[i].Kind.Transmits() {
				a.Nodes[i].TxPowerDBm = tx
			}
		}
		a.recompute()
	}

	imgui.SeparatorText("Radio")
	if imgui.BeginCombo("##bulkpreset", "apply a preset to all...") {
		for _, p := range scenario.RadioPresets {
			label := fmt.Sprintf("%s  (%.3f MHz, %g kHz, SF%d, CR4/%d)",
				p.Label, p.FreqMHz, p.BwKHz, p.SF, p.CR)
			if imgui.SelectableBool(label) {
				for _, i := range a.msel {
					if a.Nodes[i].Kind.Transmits() {
						a.applyPreset(&a.Nodes[i], p)
					}
				}
				a.status = fmt.Sprintf("%d nodes set to %s", len(a.msel), p.Label)
			}
		}
		imgui.EndCombo()
	}

	if imgui.SmallButton("clear selection") {
		a.msel = nil
	}
}

// toggleMulti adds or removes a node from the multi-selection.
//
// Seeded from the single selection: shift-clicking a second node while one is
// selected should read as "these two", not as a fresh set of one.
func (a *App) toggleMulti(i int) {
	if i < 0 || i >= len(a.Nodes) {
		return
	}
	if len(a.msel) == 0 && a.selected >= 0 && a.selected != i {
		a.msel = append(a.msel, a.selected)
	}
	for at, v := range a.msel {
		if v == i {
			a.msel = append(a.msel[:at], a.msel[at+1:]...)
			return
		}
	}
	a.msel = append(a.msel, i)
}

// drawEmitterControls is the interferer's own settings: what it radiates,
// where in the band, and how much of the time.
func (a *App) drawEmitterControls(n *scenario.Node) {
	imgui.SeparatorText("Emitter")
	textWrap("An external interference source. Its power reaches every receiver " +
		"through the same terrain as the mesh - a mast behind a hill interferes " +
		"less - and raises their noise floor by whatever lands in their passband.")
	changed := numF64("ERP dBm", &n.TxPowerDBm, 0, 80, "%.0f")
	freqMHz := float32(n.Radio.CentreHz / 1e6)
	if numF32("centre MHz", &freqMHz, 1, 6000, "%.3f") {
		n.Radio.CentreHz = float64(freqMHz) * 1e6
		changed = true
	}
	bwKHz := float32(n.Radio.BandwidthHz / 1e3)
	if numF32("bandwidth kHz", &bwKHz, 1, 20000, "%.1f") {
		n.Radio.BandwidthHz = float64(bwKHz) * 1e3
		changed = true
	}
	changed = numF64("duty %", &n.EmitterDutyPct, 0, 100, "%.0f") || changed
	if changed && a.eng != nil {
		a.eng.InvalidateLinks()
	}
	textDim("out-of-band power contributes nothing here; front-end\n" +
		"blocking is not modelled, which flatters strong neighbours")
}

// drawBoardCombo names the hardware, which is what the energy model needs and
// what nothing recorded until now.
//
// An imported node inherits the import's default board; a placed one takes
// the placement board. Both are guesses until somebody says otherwise, so the
// picker is here and the energy panel refuses to invent a battery for a node
// whose board is unknown.
func (a *App) drawBoardCombo(n *scenario.Node) {
	current := n.Board
	if current == "" {
		current = "unknown"
	}
	imgui.SetNextItemWidth(-70)
	if imgui.BeginCombo("board", current) {
		for _, b := range scenario.Boards() {
			if imgui.SelectableBool(b.Name) {
				n.Board = b.Name
				n.NoiseFigureDB = b.NoiseFigureDB
				if n.TxPowerDBm > b.MaxTxDBm {
					n.TxPowerDBm = b.MaxTxDBm
				}
				a.recompute()
			}
		}
		imgui.EndCombo()
	}
	if b, err := scenario.BoardByName(n.Board); err == nil {
		note := fmt.Sprintf("%s / %s, max %.0f dBm", b.MCU, b.Radio, b.MaxTxDBm)
		if b.Panel.PeakW > 0 {
			note += fmt.Sprintf(", %.0f W panel, %.0f mAh", b.Panel.PeakW, b.Battery.CapacityMAh)
		}
		textDim(note)
	}
}
