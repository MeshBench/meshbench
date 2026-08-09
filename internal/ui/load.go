package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// loadTimeout is how long a provider gets before the window is told it failed.
// Long enough for a slow deployment, short enough that "load" does not look
// like it did nothing.
const loadTimeout = 30 * time.Second

// drawLoadPanel brings a real network in.
//
// A workbench that can only hold nodes someone typed is a toy. The networks
// people want to reason about already exist in a CoreScope or Beacon
// deployment, and getting them in has to be one dialogue rather than an export
// and a conversion.
func (a *App) drawLoadPanel() {
	imgui.SeparatorText("Load a network")

	sources := []string{"corescope", "beacon", "file"}
	imgui.SetNextItemWidth(120)
	if imgui.BeginCombo("##source", a.loadSource) {
		for _, s := range sources {
			if imgui.SelectableBool(s) {
				a.loadSource = s
			}
		}
		imgui.EndCombo()
	}

	imgui.SetNextItemWidth(-1)
	hint := "https://corescope.example/"
	if a.loadSource == "file" {
		hint = "path to an export"
	}
	imgui.InputTextWithHint("##url", hint, &a.loadURL, 0, nil)

	if a.loadSource != "file" {
		imgui.SetNextItemWidth(-1)
		imgui.InputTextWithHint("##token", "token, if it needs one", &a.loadToken, 0, nil)
	}

	replace := a.loadReplace
	if imgui.Checkbox("replace what is here", &replace) {
		a.loadReplace = replace
	}

	if imgui.Button("load") {
		a.loadNetwork()
	}
	if a.loadStatus != "" {
		imgui.TextWrapped(a.loadStatus)
	}

	// Warnings are shown, not counted. An import that quietly assumed a mast
	// height or accepted a five-kilometre position has changed the answer, and
	// a number in a corner does not tell anyone which node it changed it for.
	if len(a.loadWarnings) > 0 {
		imgui.Spacing()
		imgui.SeparatorText("What had to be assumed")
		if imgui.BeginChildStrV("##warn", imgui.NewVec2(0, 140), 0, 0) {
			for _, w := range a.loadWarnings {
				imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
				imgui.TextWrapped(w)
				imgui.PopStyleColor()
			}
		}
		imgui.EndChild()
	}
}

func (a *App) loadNetwork() {
	a.loadStatus = ""
	a.loadWarnings = nil
	if a.loadURL == "" {
		a.loadStatus = "give a URL or a path first"
		return
	}

	radio := scenario.RadioConfig{
		CentreHz: a.freqMHz * 1e6, BandwidthHz: 250e3, SpreadFactor: 10, CodingRate: 1,
	}
	opts := scenario.ImportOptions{
		DefaultBoard: a.placeBoard, Radio: radio, MaxUncertaintyKm: 1,
	}

	records, err := a.fetchRecords()
	if err != nil {
		a.loadStatus = err.Error()
		return
	}
	res, err := scenario.Import(records, opts)
	if err != nil {
		a.loadStatus = err.Error()
		return
	}
	if len(res.Nodes) == 0 {
		a.loadStatus = res.Describe()
		return
	}

	if a.loadReplace {
		a.Nodes = nil
	}
	for _, imp := range res.Nodes {
		a.Nodes = append(a.Nodes, imp.Node)
		for _, w := range imp.Warnings {
			a.loadWarnings = append(a.loadWarnings, imp.Node.Name+": "+w)
		}
	}
	a.loadStatus = res.Describe()

	// A new network is a new geometry and a new run.
	a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
	a.terrainDirty = true
	a.buildEngine()
	a.selectFirstLink()
}

func (a *App) fetchRecords() ([]provider.NodeRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	defer cancel()

	switch a.loadSource {
	case "corescope":
		return (&provider.CoreScope{BaseURL: strings.TrimRight(a.loadURL, "/"), Token: a.loadToken}).Nodes(ctx)
	case "beacon":
		return (&provider.Beacon{BaseURL: strings.TrimRight(a.loadURL, "/"), Token: a.loadToken}).Nodes(ctx)
	case "file":
		b, err := os.ReadFile(a.loadURL)
		if err != nil {
			return nil, err
		}
		var records []provider.NodeRecord
		if err := json.Unmarshal(b, &records); err != nil {
			return nil, fmt.Errorf("%s is not a provider export: %w", a.loadURL, err)
		}
		return records, nil
	default:
		return nil, fmt.Errorf("pick a source first")
	}
}
