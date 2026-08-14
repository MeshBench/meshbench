package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/MeshBench/meshbench/internal/provider"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// importState is the Import window: where a network comes from, what arriving
// would look like, and how it lands on what is already here.
type importState struct {
	source string // "corescope" | "beacon" | "file"
	url    string
	token  string

	health *healthProbe
	job    *importJob

	preview  *importPreview
	strategy scenario.MergeStrategy

	status   string
	warnings []string

	// offer shows the get-going strip after a commit: the three things almost
	// every import wants next, each with its cost stated, none auto-run.
	offer bool
}

// healthProbe is one in-flight Health() call for the selected source.
type healthProbe struct {
	source, url string
	done        chan error
	msg         string
	ok, running bool
}

// importJob is one in-flight fetch, running off the frame thread.
type importJob struct {
	count  int64 // atomic: records fetched so far
	done   chan importOutcome
	cancel context.CancelFunc
}

type importOutcome struct {
	nodes    []scenario.Node
	warnings []string
	describe string
	err      error
}

// importPreview is what a fetch would bring in — held here, drawn on the map,
// and not in the scenario until the operator commits it.
type importPreview struct {
	nodes    []scenario.Node
	describe string
	// Bounding box of the previewed nodes, drawn on the map so "is that the
	// right network" is answered by looking.
	minLat, minLon, maxLat, maxLon float64
}

// importSources is what the window offers: the provider registry's sources
// first, then the local ones no provider models.
func (a *App) importSources() []provider.Provider {
	reg := provider.NewRegistry()
	url := strings.TrimRight(a.imp.url, "/")
	_ = reg.Register(&provider.CoreScope{BaseURL: url, Token: a.imp.token})
	_ = reg.Register(&provider.Beacon{BaseURL: url, Token: a.imp.token})
	return reg.All()
}

func capString(c provider.Capability) string {
	var parts []string
	if c.Has(provider.CapNodes) {
		parts = append(parts, "nodes")
	}
	if c.Has(provider.CapPackets) {
		parts = append(parts, "packets")
	}
	if c.Has(provider.CapRegions) {
		parts = append(parts, "regions")
	}
	if c.Has(provider.CapLive) {
		parts = append(parts, "live")
	}
	return strings.Join(parts, ", ")
}

// drawImportBody is the Import tool: source → health → preview → merge.
//
// Preview-then-commit throughout. Nothing here touches the scenario until the
// commit button, and the commit button says what it will do first — the old
// Load panel's silent append-or-replace checkbox is the thing this replaces.
func (a *App) drawImportBody() {
	s := &a.imp
	if s.source == "" {
		s.source = "corescope"
	}
	if s.strategy == "" {
		s.strategy = scenario.MergeAddNew
	}

	imgui.SeparatorText("Source")
	for _, p := range a.importSources() {
		if imgui.RadioButtonBool(p.Name(), s.source == p.Name()) {
			s.source, s.health, s.preview = p.Name(), nil, nil
		}
		imgui.SameLine()
		textDim(capString(provider.CapabilitiesOf(p)))
	}
	if imgui.RadioButtonBool("saved network", s.source == "saved") {
		s.source, s.health, s.preview = "saved", nil, nil
	}
	imgui.SameLine()
	textDim("one you saved here before - milliseconds, no assumptions re-made")
	if imgui.RadioButtonBool("file", s.source == "file") {
		s.source, s.health, s.preview = "file", nil, nil
	}
	imgui.SameLine()
	textDim("a path to an export or a saved network")

	// The list, back where it was and where it belongs. Moving it to the File
	// menu made a network someone saved yesterday effectively unfindable.
	if s.source == "saved" {
		a.drawSavedNetworkList()
		return
	}

	imgui.SetNextItemWidth(-1)
	hint := "https://corescope.example/"
	if s.source == "file" {
		hint = "path to the file"
	}
	imgui.InputTextWithHint("##impurl", hint, &s.url, 0, nil)
	if s.source != "file" {
		imgui.SetNextItemWidth(-1)
		imgui.InputTextWithHint("##imptoken", "token, if it needs one", &s.token, 0, nil)
		a.drawSourceHealth()
	}

	imgui.SeparatorText("Boundary")
	if imgui.Button("choose areas...") {
		a.showPanel("Boundary")
	}
	imgui.SameLine()
	if n := len(a.bnd.chosen); n > 0 {
		textDim(fmt.Sprintf("%d area(s) filter the preview", n))
	} else {
		textDim("none set - the whole source arrives")
	}
	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##impboundary", "...or a GeoJSON file path", &a.boundaryPath, 0, nil)

	imgui.Spacing()
	if s.job != nil {
		a.pollImport()
	}
	if s.job != nil {
		fetched := atomic.LoadInt64(&s.job.count)
		imgui.ProgressBarV(-1*float32(imgui.Time()), imgui.NewVec2(-1, 0),
			fmt.Sprintf("fetching... %d records", fetched))
	} else if primaryButton("fetch preview", imgui.NewVec2(0, 0)) {
		a.startImportFetch()
	}
	if s.status != "" {
		textWrap(s.status)
	}

	a.drawImportOffers()
	a.drawImportPreview()
	a.drawImportWarnings()

	// Inference is part of importing, not a separate ceremony: the same source
	// that named the nodes can prove what they are configured to do.
	if s.source == "corescope" && s.url != "" {
		imgui.Spacing()
		// Forced open while a walk is running: a command that starts work has
		// to reveal the work, and this section was collapsed by default - so
		// inference driven from outside looked like nothing happening at all.
		if a.infer.running || a.infer.result != nil {
			imgui.SetNextItemOpen(true)
		}
		if imgui.CollapsingHeaderBoolPtr("Read the traffic too", nil) {
			a.drawInferBody()
		}
	}
}

// drawSourceHealth shows whether the selected source would answer, before
// anything is fetched from it.
func (a *App) drawSourceHealth() {
	s := &a.imp
	if s.health != nil && s.health.running {
		select {
		case err := <-s.health.done:
			s.health.running = false
			if err != nil {
				s.health.msg, s.health.ok = err.Error(), false
			} else {
				s.health.msg, s.health.ok = "reachable", true
			}
		default:
		}
	}
	if imgui.SmallButton("check") && s.url != "" {
		h := &healthProbe{source: s.source, url: s.url, running: true, done: make(chan error, 1)}
		s.health = h
		var target provider.Checker
		for _, p := range a.importSources() {
			if p.Name() == h.source {
				target, _ = p.(provider.Checker)
			}
		}
		go func() {
			if target == nil {
				h.done <- fmt.Errorf("this source has no health check")
				return
			}
			h.done <- target.Health(context.Background())
		}()
	}
	imgui.SameLine()
	switch {
	case s.health == nil:
		textDim("health unknown")
	case s.health.running:
		textDim("checking...")
	case s.health.ok:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.4, 0.85, 0.45, 1))
		imgui.Text("ok: " + s.health.msg)
		imgui.PopStyleColor()
	default:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
		textWrap("unhealthy: " + s.health.msg)
		imgui.PopStyleColor()
	}
}

// drawImportPreview is the counts, the names, the merge choice and the commit.
func (a *App) drawImportPreview() {
	s := &a.imp
	if s.preview == nil {
		return
	}
	p := s.preview
	imgui.SeparatorText(fmt.Sprintf("Preview - %d nodes", len(p.nodes)))
	textDim(p.describe)
	textDim("bounding box drawn on the map; nothing is in the scenario yet")

	if imgui.BeginChildStrV("##impnames", imgui.NewVec2(0, 120), imgui.ChildFlagsFrameStyle, 0) {
		for _, n := range p.nodes {
			imgui.Text(n.Name)
		}
	}
	imgui.EndChild()

	if len(a.Nodes) > 0 {
		imgui.SeparatorText("Merge onto what is here")
		for _, st := range []struct {
			s     scenario.MergeStrategy
			label string
		}{
			{scenario.MergeAddNew, "add only new - nothing here is touched"},
			{scenario.MergeReplaceMatching, "replace matching - the import wins where both have a node"},
			{scenario.MergeReplaceAll, "replace all - start over from the import"},
		} {
			if imgui.RadioButtonBool(st.label, s.strategy == st.s) {
				s.strategy = st.s
			}
		}
		plan := scenario.PlanMerge(a.Nodes, p.nodes, s.strategy)
		imgui.Text(plan.String())
	} else {
		textDim("the scenario is empty - everything previewed arrives")
	}

	if primaryButton("commit", imgui.NewVec2(0, 0)) {
		a.commitImport()
	}
	imgui.SameLine()
	if imgui.Button("discard preview") {
		s.preview = nil
	}
}

func (a *App) drawImportWarnings() {
	if len(a.imp.warnings) == 0 {
		return
	}
	imgui.Spacing()
	imgui.SeparatorText("What had to be assumed")
	if imgui.BeginChildStrV("##impwarn", imgui.NewVec2(0, 110), 0, 0) {
		for _, w := range a.imp.warnings {
			imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
			textWrap(w)
			imgui.PopStyleColor()
		}
	}
	imgui.EndChild()
}

// startImportFetch runs the fetch off the frame thread and lands a preview.
func (a *App) startImportFetch() {
	s := &a.imp
	s.status, s.warnings, s.preview = "", nil, nil
	if s.url == "" {
		s.status = "give a URL or a path first"
		return
	}
	region, err := a.regionOrNil()
	if err != nil {
		s.status = err.Error()
		return
	}
	opts := scenario.ImportOptions{
		DefaultBoard:     a.placeBoard,
		Radio:            a.defaultRadio(),
		MaxUncertaintyKm: 1,
		Region:           region,
	}
	job := &importJob{done: make(chan importOutcome, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), loadTimeout)
	job.cancel = cancel
	s.job = job
	source, url, token := s.source, strings.TrimRight(s.url, "/"), s.token
	go func() {
		defer cancel()
		job.done <- fetchImport(ctx, source, url, token, opts,
			func(n int) { atomic.StoreInt64(&job.count, int64(n)) })
	}()
}

// fetchImport does the work a preview needs, App-free — it runs on a worker
// goroutine, and a worker holding the App is a data race wearing a convenience.
func fetchImport(ctx context.Context, source, url, token string,
	opts scenario.ImportOptions, progress func(int)) importOutcome {
	if source == "file" {
		if nodes, ok := decodeNetworkFile(url, opts.Region); ok {
			return importOutcome{nodes: nodes,
				describe: fmt.Sprintf("%d nodes from a saved network", len(nodes))}
		}
	}
	records, err := fetchProviderRecords(ctx, source, url, token, progress)
	if err != nil {
		return importOutcome{err: err}
	}
	res, err := scenario.Import(records, opts)
	if err != nil {
		return importOutcome{err: err}
	}
	out := importOutcome{describe: res.Describe()}
	for _, imp := range res.Nodes {
		out.nodes = append(out.nodes, imp.Node)
		for _, w := range imp.Warnings {
			out.warnings = append(out.warnings, imp.Node.Name+": "+w)
		}
	}
	return out
}

// pollImport collects a finished fetch on the frame thread.
func (a *App) pollImport() {
	s := &a.imp
	select {
	case out := <-s.job.done:
		s.job = nil
		if out.err != nil {
			s.status = out.err.Error()
			return
		}
		if len(out.nodes) == 0 {
			s.status = out.describe
			if s.status == "" {
				s.status = "the source answered, but nothing survived the filters"
			}
			return
		}
		p := &importPreview{nodes: out.nodes, describe: out.describe}
		p.minLat, p.maxLat = out.nodes[0].Position.Lat, out.nodes[0].Position.Lat
		p.minLon, p.maxLon = out.nodes[0].Position.Lon, out.nodes[0].Position.Lon
		for _, n := range out.nodes[1:] {
			p.minLat, p.maxLat = min(p.minLat, n.Position.Lat), max(p.maxLat, n.Position.Lat)
			p.minLon, p.maxLon = min(p.minLon, n.Position.Lon), max(p.maxLon, n.Position.Lon)
		}
		s.preview, s.warnings, s.status = p, out.warnings, ""
	default:
	}
}

// commitImport is the only place in this file that changes the scenario.
func (a *App) commitImport() {
	s := &a.imp
	if s.preview == nil {
		return
	}
	plan := scenario.PlanMerge(a.Nodes, s.preview.nodes, s.strategy)
	a.Nodes = scenario.Merge(a.Nodes, s.preview.nodes, s.strategy)
	a.msel = nil
	s.preview = nil
	s.status = plan.String()
	s.offer = true
	a.view.FitTo(a.Nodes, a.view.Width, a.view.Height)
	a.terrainDirty = true
	a.buildEngine()
	a.selectFirstLink()
}

// drawImportPreviewBox marks the previewed import's extent on the map.
func (a *App) drawImportPreviewBox(origin imgui.Vec2, w, h float32) {
	p := a.imp.preview
	if p == nil {
		return
	}
	dl := imgui.WindowDrawList()
	dl.PushClipRectV(origin, imgui.NewVec2(origin.X+w, origin.Y+h), true)
	defer dl.PopClipRect()
	x0, y0 := a.view.LatLonToScreen(p.maxLat, p.minLon)
	x1, y1 := a.view.LatLonToScreen(p.minLat, p.maxLon)
	tl := imgui.NewVec2(origin.X+float32(x0), origin.Y+float32(y0))
	br := imgui.NewVec2(origin.X+float32(x1), origin.Y+float32(y1))
	col := imgui.ColorU32Vec4(imgui.NewVec4(0.95, 0.75, 0.25, 0.9))
	dl.AddRectV(tl, br, col, 0, 2, 0)
	dl.AddTextVec2(imgui.NewVec2(tl.X+4, tl.Y+2), col,
		fmt.Sprintf("import preview: %d nodes", len(p.nodes)))
}

// decodeNetworkFile reads a saved-network file, applying the boundary. The
// second return distinguishes "not that kind of file" from an empty network.
func decodeNetworkFile(path string, region *scenario.Region) ([]scenario.Node, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var nodes []scenario.Node
	if err := json.Unmarshal(b, &nodes); err != nil || len(nodes) == 0 || nodes[0].Name == "" {
		return nil, false
	}
	if region != nil {
		kept := nodes[:0]
		for _, n := range nodes {
			if region.Participates(n.Position) {
				kept = append(kept, n)
			}
		}
		nodes = kept
	}
	return nodes, true
}

// loadTimeout is how long a provider gets before the fetch is told it failed —
// long enough for a slow deployment, short enough that "fetch" does not look
// like it did nothing.
const loadTimeout = 30 * time.Second

// fetchProviderRecords resolves the source by name and fetches its nodes. It
// takes values rather than the App for the same reason fetchImport does.
func fetchProviderRecords(ctx context.Context, source, baseURL, token string,
	progress func(int)) ([]provider.NodeRecord, error) {
	switch source {
	case "corescope":
		return (&provider.CoreScope{BaseURL: baseURL, Token: token, Progress: progress}).Nodes(ctx)
	case "beacon":
		return (&provider.Beacon{BaseURL: baseURL, Token: token, Progress: progress}).Nodes(ctx)
	case "file":
		b, err := os.ReadFile(baseURL)
		if err != nil {
			return nil, err
		}
		var records []provider.NodeRecord
		if err := json.Unmarshal(b, &records); err != nil {
			return nil, fmt.Errorf("%s is neither a saved network nor a provider export: %w", baseURL, err)
		}
		return records, nil
	default:
		return nil, fmt.Errorf("pick a source first")
	}
}

// drawImportOffers is what almost every import wants next. Offered, never
// auto-run: each has a cost, and the cost is stated on the button's tooltip.
func (a *App) drawImportOffers() {
	s := &a.imp
	if !s.offer {
		return
	}
	imgui.SeparatorText("Get going")
	if imgui.Button("infer boundary") {
		a.inferAreaFromNodes()
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Finds the place the imported nodes sit in and sets it as the\n" +
			"study area. Cost: one lookup against OpenStreetMap's Nominatim.")
	}
	imgui.SameLine()
	if imgui.Button("get terrain") {
		a.fetchVisibleTerrain()
	}
	if imgui.IsItemHovered() {
		tip := "Downloads elevation tiles for the area. Cached permanently."
		if est, ok := a.terrainEstimate(); ok {
			tip += fmt.Sprintf("\nCost: %d tiles, %d already cached, roughly %d MB to fetch",
				est.Tiles, est.Cached, est.BytesRough/1_000_000)
		}
		imgui.SetTooltip(tip)
	}
	imgui.SameLine()
	if a.eng != nil && a.eng.FirmwareCount() == 0 {
		if imgui.Button("start MeshCore now") {
			a.attachFirmware()
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip(fmt.Sprintf("One MeshCore process per node, %d here. Play starts\n"+
				"them anyway - this is for starting without running the clock.",
				len(a.Nodes)))
		}
	}
	imgui.SameLine()
	if imgui.SmallButton("dismiss##offers") {
		s.offer = false
	}
}

// drawSavedNetworkList is the saved networks, described and one click to use.
//
// Load replaces, add merges: what happens is decided where the click happens,
// not by a checkbox somewhere else on the panel.
func (a *App) drawSavedNetworkList() {
	imgui.SeparatorText("Saved networks")
	if len(a.Nodes) > 0 {
		imgui.SetNextItemWidth(180)
		imgui.InputTextWithHint("##savename", defaultSaveName(len(a.Nodes)), &a.saveName, 0, nil)
		imgui.SameLine()
		if imgui.Button("save what is here") {
			a.saveNetwork()
		}
	}
	rows := a.savedNetworks()
	if len(rows) == 0 {
		textDimWrap("nothing saved yet - import a network, then save it and reopening " +
			"takes milliseconds instead of a fetch")
		return
	}
	if !imgui.BeginTableV("##savednets", 4,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY|
			imgui.TableFlagsResizable|imgui.TableFlagsReorderable,
		imgui.NewVec2(0, 260), 0) {
		return
	}
	imgui.TableSetupColumnV("network", imgui.TableColumnFlagsWidthStretch, 0, 0)
	pad := imgui.CurrentStyle().FramePadding().X*2 + 8
	imgui.TableSetupColumnV("nodes", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("00000").X+pad, 0)
	imgui.TableSetupColumnV("saved", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("00 Jan 00:00").X+pad, 0)
	imgui.TableSetupColumnV("", imgui.TableColumnFlagsWidthFixed,
		imgui.CalcTextSize("load  add  sure?").X+pad*3, 0)
	imgui.TableHeadersRow()
	for i, n := range rows {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(n.name)
		imgui.TableSetColumnIndex(1)
		imgui.Text(fmt.Sprint(n.nodes))
		imgui.TableSetColumnIndex(2)
		textDim(age(n.saved))
		imgui.TableSetColumnIndex(3)
		if imgui.SmallButton(fmt.Sprintf("load##sv%d", i)) {
			a.loadSavedNet(n.name, true)
			a.imp.status = a.status
		}
		imgui.SameLine()
		if imgui.SmallButton(fmt.Sprintf("add##sv%d", i)) {
			a.loadSavedNet(n.name, false)
			a.imp.status = a.status
		}
		imgui.SameLine()
		lbl := "x"
		if a.confirmDelete == n.name {
			lbl = "sure?"
		}
		if imgui.SmallButton(fmt.Sprintf("%s##svd%d", lbl, i)) {
			if a.confirmDelete == n.name {
				_ = os.Remove(filepath.Join(scenarioDir(), n.name+".json"))
				a.confirmDelete = ""
				a.savedDirty = true
			} else {
				a.confirmDelete = n.name
			}
		}
	}
	imgui.EndTable()
}
