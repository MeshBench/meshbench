package ui

import (
	"fmt"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// drawNodesTableWindow is the whole network in one editable table — HopReach's
// "Repeaters & settings" modal, rebuilt on real firmware.
//
// One row per node: identity, radio preset, power, height, and — once a run has
// happened — what that node's airtime actually bought. The bulk row at the top
// fills a column for every listed node at once, which is how "set the whole
// mesh to UK Narrow" is one action rather than four hundred.
func (a *App) drawNodesTableWindow() {
	if !a.winNodesTable {
		return
	}
	imgui.SetNextWindowSizeV(a.windowSize(120, 26), imgui.CondFirstUseEver)
	open := a.winNodesTable
	if imgui.BeginV("Nodes & settings", &open, 0) {
		a.drawNodesTableBody()
	}
	imgui.End()
	a.winNodesTable = open
}

func (a *App) drawNodesTableBody() {
	imgui.SetNextItemWidth(220)
	imgui.InputTextWithHint("##tablefilter", "filter by name or kind", &a.nodeFilter, 0, nil)
	imgui.SameLine()

	// The bulk row: apply one preset to everything the filter matches. Scoped
	// to the filter on purpose — "everything in view" is what an operator
	// means, and it makes "all the repeaters" one filter word away.
	imgui.SetNextItemWidth(240)
	if imgui.BeginCombo("apply preset to listed", "choose...") {
		for _, p := range scenario.RadioPresets {
			label := fmt.Sprintf("%s  (%.3f MHz, %g kHz, SF%d, CR4/%d)",
				p.Label, p.FreqMHz, p.BwKHz, p.SF, p.CR)
			if imgui.SelectableBool(label) {
				count := 0
				for _, i := range a.matchingNodes() {
					if a.Nodes[i].Kind.Transmits() {
						a.applyPreset(&a.Nodes[i], p)
						count++
					}
				}
				a.status = fmt.Sprintf("%s applied to %d nodes", p.Label, count)
			}
		}
		imgui.EndCombo()
	}

	haveRun := a.eng != nil && len(a.eng.Events()) > 0
	cols := int32(6)
	if haveRun {
		cols = 8
	}
	if !imgui.BeginTableV("##nodestable", cols,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY|
			imgui.TableFlagsResizable, imgui.NewVec2(0, 0), 0) {
		return
	}
	imgui.TableSetupColumnV("node", imgui.TableColumnFlagsWidthFixed, 130, 0)
	imgui.TableSetupColumnV("kind", imgui.TableColumnFlagsWidthFixed, 80, 0)
	imgui.TableSetupColumnV("radio", imgui.TableColumnFlagsWidthStretch, 0, 0)
	imgui.TableSetupColumnV("tx dBm", imgui.TableColumnFlagsWidthFixed, 60, 0)
	imgui.TableSetupColumnV("height m", imgui.TableColumnFlagsWidthFixed, 70, 0)
	imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed, 60, 0)
	if haveRun {
		// Result columns appear only once there are results, exactly as in
		// HopReach: before a run they would be a wall of dashes.
		imgui.TableSetupColumnV("duty", imgui.TableColumnFlagsWidthFixed, 60, 0)
		imgui.TableSetupColumnV("unique/redundant", imgui.TableColumnFlagsWidthFixed, 110, 0)
	}
	imgui.TableHeadersRow()

	scores := map[string]int{}
	var board []scenario.Node
	_ = board
	if haveRun {
		for i, s := range a.eng.Scoreboard() {
			scores[s.Name] = i
		}
	}

	match := a.matchingNodes()
	shown := match
	if len(shown) > maxNodeRows {
		shown = shown[:maxNodeRows]
	}
	for _, i := range shown {
		a.drawNodesTableRow(i, haveRun, scores)
	}
	imgui.EndTable()
	if len(match) > len(shown) {
		textDim(fmt.Sprintf("%d more - narrow the filter", len(match)-len(shown)))
	}
}

func (a *App) drawNodesTableRow(i int, haveRun bool, scores map[string]int) {
	n := &a.Nodes[i]
	imgui.TableNextRow()

	imgui.TableSetColumnIndex(0)
	sel := i == a.selected
	if imgui.SelectableBoolPtrV(n.Name+"##row", &sel, imgui.SelectableFlagsSpanAllColumns|
		imgui.SelectableFlagsAllowOverlap, imgui.NewVec2(0, 0)) {
		a.SelectNode(i, false)
	}
	// The row is also a door to the node's own window, because the table is
	// where a node gets found and the window is where it gets watched.
	if imgui.IsItemHovered() && imgui.IsMouseDoubleClicked(imgui.MouseButtonLeft) {
		a.openNodeWindow(n.Name)
	}

	imgui.TableSetColumnIndex(1)
	textDim(kindLabel(n.Kind))

	imgui.TableSetColumnIndex(2)
	if n.Kind.Transmits() {
		imgui.SetNextItemWidth(-1)
		a.drawPresetComboInline(n, i)
	} else {
		textDim("-")
	}

	imgui.TableSetColumnIndex(3)
	if n.Kind.Transmits() {
		imgui.SetNextItemWidth(-1)
		v := float32(n.TxPowerDBm)
		if imgui.DragFloatV(fmt.Sprintf("##tx%d", i), &v, 0.2, 2, 30, "%.0f", 0) {
			n.TxPowerDBm = float64(v)
			a.recompute()
		}
	}

	imgui.TableSetColumnIndex(4)
	imgui.SetNextItemWidth(-1)
	h := float32(n.HeightAGLm)
	if imgui.DragFloatV(fmt.Sprintf("##h%d", i), &h, 0.2, 1, 60, "%.1f", 0) {
		n.HeightAGLm = float64(h)
		a.recompute()
	}

	imgui.TableSetColumnIndex(5)
	if imgui.SmallButton(fmt.Sprintf("open##%d", i)) {
		a.openNodeWindow(n.Name)
	}

	if haveRun {
		if idx, ok := scores[n.Name]; ok {
			s := a.eng.Scoreboard()[idx]
			imgui.TableSetColumnIndex(6)
			col := imgui.NewVec4(0.7, 0.75, 0.85, 1)
			if s.DutyCyclePct > 1 {
				col = imgui.NewVec4(0.9, 0.4, 0.4, 1)
			}
			imgui.PushStyleColorVec4(imgui.ColText, col)
			imgui.Text(fmt.Sprintf("%.2f%%", s.DutyCyclePct))
			imgui.PopStyleColor()
			imgui.TableSetColumnIndex(7)
			imgui.Text(fmt.Sprintf("%d / %d", s.UniqueDelivery, s.RedundantRelay))
		}
	}
}

// drawPresetComboInline is the compact preset cell for the table.
func (a *App) drawPresetComboInline(n *scenario.Node, i int) {
	current := fmt.Sprintf("%.3f/%g/SF%d", n.Radio.CentreHz/1e6, n.Radio.BandwidthHz/1000, n.Radio.SpreadFactor)
	for _, p := range scenario.RadioPresets {
		if p.Matches(n.Radio) {
			current = p.Label
			break
		}
	}
	if imgui.BeginCombo(fmt.Sprintf("##preset%d", i), current) {
		for _, p := range scenario.RadioPresets {
			label := fmt.Sprintf("%s  (%.3f MHz, %g kHz, SF%d, CR4/%d)",
				p.Label, p.FreqMHz, p.BwKHz, p.SF, p.CR)
			if imgui.SelectableBool(label) {
				a.applyPreset(n, p)
			}
		}
		imgui.EndCombo()
	}
}
