package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/dsp"
	"github.com/A13xB0/meshcoresim/internal/provider"
	"github.com/A13xB0/meshcoresim/internal/replay"
	"github.com/A13xB0/meshcoresim/internal/scenario"
	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// validateState is the ADR-0015 chain: fetch what really happened, replay it
// against the model, publish the residuals, and offer the calibration.
type validateState struct {
	lookbackH int32
	running   bool
	cancel    context.CancelFunc
	done      chan validateOutcome

	report    *replay.Report
	excluded  string
	err       string
	fetchedAt time.Time

	// shadow keeps the comparison fresh: refetch on a timer, so the model is
	// continuously watched by reality rather than checked once and believed.
	shadow bool
}

type validateOutcome struct {
	report   replay.Report
	excluded string
	err      error
}

// shadowEvery is how often shadow mode refetches. Wall time, because the
// observations arrive in wall time.
const shadowEvery = 5 * time.Minute

// drawValidateBody is the Validate panel.
func (a *App) drawValidateBody() {
	s := &a.val
	if s.lookbackH == 0 {
		s.lookbackH = 24
	}

	textWrap("Replays real receptions against the RF model and reports the " +
		"residuals - the only view in which the model can be caught being wrong. " +
		"Reads receptions from the Import window's CoreScope source.")

	if a.imp.source != "corescope" || a.imp.url == "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		textWrap("Set the Import window's source to corescope and give it a URL first.")
		imgui.PopStyleColor()
		return
	}
	textDim("reading from " + a.imp.url)

	imgui.SetNextItemWidth(110)
	imgui.InputIntV("hours to look back", &s.lookbackH, 0, 0, 0)
	if s.lookbackH < 1 {
		s.lookbackH = 1
	}

	if s.done != nil {
		a.pollValidation()
	}
	if s.running {
		imgui.ProgressBarV(-1*float32(imgui.Time()), imgui.NewVec2(-1, 0), "comparing...")
	} else if imgui.Button("fetch and compare") {
		a.startValidation()
	}
	imgui.SameLine()
	imgui.Checkbox("shadow mode", &s.shadow)
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Refetches every five minutes and recompares, so the agreement\n" +
			"figure tracks reality instead of the moment someone last pressed\n" +
			"the button.")
	}
	if s.shadow && !s.running && !s.fetchedAt.IsZero() && time.Since(s.fetchedAt) > shadowEvery {
		a.startValidation()
	}

	if s.err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.9, 0.4, 0.4, 1))
		textWrap(s.err)
		imgui.PopStyleColor()
	}
	if s.report == nil {
		textDimWrap("nothing compared yet - fetch, and the residual table appears here")
		return
	}
	a.drawValidationReport()
}

// drawValidationReport is the residual table and the calibration offer.
func (a *App) drawValidationReport() {
	s := &a.val
	rep := s.report

	imgui.SeparatorText("Agreement")
	textWrap(rep.Summary())
	if s.excluded != "" {
		textDim("excluded: " + s.excluded)
	}
	if a.eng != nil && a.eng.Config.ExcessPathLossDB != 0 {
		textDim(fmt.Sprintf("channel currently calibrated: +%.1f dB excess path loss",
			a.eng.Config.ExcessPathLossDB))
	}

	if cal, err := replay.Calibrate(*rep); err == nil {
		if imgui.Button(fmt.Sprintf("apply +%.1f dB excess path loss (from %d observations)",
			cal.ExcessLossDB, cal.Samples)) {
			a.excessLossDB = cal.ExcessLossDB
			a.buildEngine()
			a.status = fmt.Sprintf("channel calibrated: +%.1f dB excess path loss, fitted to %d "+
				"real receptions", cal.ExcessLossDB, cal.Samples)
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip("The one-parameter correction ADR-0015 allows: the mean bias,\n" +
				"applied as extra loss on every path. Fitted to observations,\n" +
				"not chosen - and always displayed while active.")
		}
		if a.excessLossDB != 0 {
			imgui.SameLine()
			if imgui.SmallButton("remove calibration") {
				a.excessLossDB = 0
				a.buildEngine()
			}
		}
	} else {
		textDim(err.Error())
	}

	if !imgui.BeginTableV("##residuals", 5,
		imgui.TableFlagsBorders|imgui.TableFlagsRowBg|imgui.TableFlagsScrollY|
			imgui.TableFlagsResizable|imgui.TableFlagsReorderable,
		imgui.NewVec2(0, 0), 0) {
		return
	}
	for _, h := range []string{"origin", "receiver", "observed SNR", "predicted SNR", "verdict"} {
		imgui.TableSetupColumnV(h, imgui.TableColumnFlagsWidthStretch, 0, 0)
	}
	imgui.TableHeadersRow()
	for _, row := range rep.Rows {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		imgui.Text(row.Origin)
		imgui.TableSetColumnIndex(1)
		imgui.Text(row.Receiver)
		imgui.TableSetColumnIndex(2)
		if row.HasSNR {
			imgui.Text(fmt.Sprintf("%+.1f", row.ObservedSNR))
		} else if row.Observed {
			textDim("heard")
		} else {
			textDim("silent")
		}
		imgui.TableSetColumnIndex(3)
		imgui.Text(fmt.Sprintf("%+.1f", row.PredictedSNR))
		imgui.TableSetColumnIndex(4)
		col := imgui.NewVec4(0.7, 0.75, 0.85, 1)
		if strings.Contains(row.Verdict, "OPTIMISTIC") {
			col = imgui.NewVec4(0.9, 0.4, 0.4, 1)
		} else if strings.Contains(row.Verdict, "PESSIMISTIC") {
			col = imgui.NewVec4(0.95, 0.72, 0.25, 1)
		} else if strings.Contains(row.Verdict, "agrees") {
			col = imgui.NewVec4(0.45, 0.85, 0.5, 1)
		}
		imgui.PushStyleColorVec4(imgui.ColText, col)
		textWrap(row.Verdict)
		imgui.PopStyleColor()
	}
	imgui.EndTable()
}

// startValidation fetches receptions and compares, off the frame thread.
func (a *App) startValidation() {
	s := &a.val
	s.err, s.running = "", true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	done := make(chan validateOutcome, 1)
	s.done = done

	// Snapshots for the worker: the frame thread owns the App.
	nodes := make([]scenario.Node, len(a.Nodes))
	copy(nodes, a.Nodes)
	url, token := strings.TrimRight(a.imp.url, "/"), a.imp.token
	since := time.Now().Add(-time.Duration(s.lookbackH) * time.Hour)
	freq := a.freqMHz
	terr := a.Terrain

	go func() {
		defer cancel()
		cs := &provider.CoreScope{BaseURL: url, Token: token}
		cctx, ccancel := context.WithTimeout(ctx, 2*time.Minute)
		defer ccancel()
		rx, err := cs.Receptions(cctx, since)
		if err != nil {
			done <- validateOutcome{err: err}
			return
		}
		report, excluded := compareAgainstModel(rx, nodes, terr, freq)
		done <- validateOutcome{report: report, excluded: excluded}
	}()
}

func (a *App) pollValidation() {
	s := &a.val
	select {
	case out := <-s.done:
		s.done, s.running, s.cancel = nil, false, nil
		s.fetchedAt = time.Now()
		if out.err != nil {
			s.err = out.err.Error()
			return
		}
		s.report, s.excluded = &out.report, out.excluded
	default:
	}
}

// compareAgainstModel builds the predictor over a node snapshot and runs the
// comparison. App-free, so it can live on a worker goroutine.
func compareAgainstModel(rx []provider.Reception, nodes []scenario.Node,
	terr Terrain, freqMHz float64) (replay.Report, string) {
	byName := map[string]*scenario.Node{}
	for i := range nodes {
		byName[strings.ToLower(nodes[i].Name)] = &nodes[i]
	}

	// HAM-34, without exception: a node placed to worse than a kilometre
	// measures nothing and is excluded from validation, not merely flagged.
	const maxUncertaintyKm = 1.0
	uncertain := 0
	eligible := rx[:0]
	for _, r := range rx {
		o, okO := byName[strings.ToLower(r.Origin)]
		v, okR := byName[strings.ToLower(r.Receiver)]
		if okO && o.UncertaintyKm > maxUncertaintyKm ||
			okR && v.UncertaintyKm > maxUncertaintyKm {
			uncertain++
			continue
		}
		eligible = append(eligible, r)
	}

	// Memoised per pair: the terrain profile is the expensive part and a busy
	// packet stream repeats the same links thousands of times.
	type key [2]string
	type answer struct {
		snr     float64
		decodes bool
		ok      bool
	}
	memo := map[key]answer{}
	predict := func(origin, receiver string) (float64, bool, bool) {
		k := key{strings.ToLower(origin), strings.ToLower(receiver)}
		if v, seen := memo[k]; seen {
			return v.snr, v.decodes, v.ok
		}
		from, okF := byName[k[0]]
		to, okT := byName[k[1]]
		if !okF || !okT || !from.Kind.Transmits() {
			memo[k] = answer{}
			return 0, false, false
		}
		snr, decodes, ok := predictSNR(*from, *to, terr, freqMHz)
		memo[k] = answer{snr: snr, decodes: decodes, ok: ok}
		return snr, decodes, ok
	}

	// Online means "heard at least one packet in the window" — the only guard
	// available against survivorship.
	seen := map[string]bool{}
	var online []string
	for _, r := range eligible {
		if !seen[r.Receiver] {
			seen[r.Receiver] = true
			online = append(online, r.Receiver)
		}
	}

	rep := replay.Compare(eligible, predict, online)
	excluded := ""
	if uncertain > 0 {
		excluded = fmt.Sprintf("%d receptions involving nodes placed worse than %.0f km",
			uncertain, maxUncertaintyKm)
	}
	for reason, n := range rep.Excluded {
		if excluded != "" {
			excluded += "; "
		}
		excluded += fmt.Sprintf("%d %s", n, reason)
	}
	return rep, excluded
}

// predictSNR is the model's answer for one directed link, through the same
// terrain and antenna machinery the Budget panel uses.
func predictSNR(from, to scenario.Node, terr Terrain, freqMHz float64) (float64, bool, bool) {
	r := &coverage.Raster{
		South: to.Position.Lat, North: to.Position.Lat,
		West: to.Position.Lon, East: to.Position.Lon,
		Width: 1, Height: 1, Cells: make([]coverage.Cell, 1),
		FreqMHz: freqMHz,
	}
	sens := sensitivityFor(from.Radio)
	fixed := coverage.Endpoint{
		Name: from.Name, Lat: from.Position.Lat, Lon: from.Position.Lon,
		HeightAGLm: from.HeightAGLm, TxPowerDBm: from.TxPowerDBm,
		SensitivityDBm: sens,
		GainTowardsDBi: from.Antenna.GainTowardsDBi,
	}
	opts := coverage.Options{
		RemoteHeightAGLm: to.HeightAGLm, RemoteTxPowerDBm: to.TxPowerDBm,
		RemoteGainDBi:        to.Antenna.Pattern.PeakDBi() - to.Antenna.FeedlineDB,
		RemoteSensitivityDBm: sens, ProfileStepM: 30,
	}
	if err := coverage.Compute(fixed, terr, r, opts); err != nil {
		return 0, false, false
	}
	cell := r.At(0, 0)
	if cell.NoData {
		return 0, false, false
	}
	// Margin is against sensitivity; SNR is against the noise floor. Move
	// between them explicitly, with the same figures the engine uses.
	sf := from.Radio.SpreadFactor
	rxDBm := cell.OutboundMarginDB + sens
	noise := dsp.NoiseFloorDBm(from.Radio.BandwidthHz, 6)
	snr := rxDBm - noise
	need, known := dsp.RequiredSNRdB[sf]
	if !known {
		need = -20
	}
	return snr, snr >= need, true
}

// keep terrain import honest: Terrain is the app-level interface.
var _ = terrain.Estimate{}
