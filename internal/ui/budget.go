package ui

import (
	"fmt"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/pathview"
	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// budgetTerm is one line of the link budget.
type budgetTerm struct {
	Label string
	DB    float64
	// Running is the accumulated dBm after this term, so the bar chart can be
	// read as a waterfall rather than as unrelated numbers.
	Running float64
	// Gain marks a term that adds rather than costs, for colour.
	Gain bool
}

// drawLinkBudget is every term of the budget, in order, both directions.
//
// The UX spec's demand, and the reason it is a demand: "no result without its
// provenance". A margin of +4.2 dB is a number to be trusted or not; the same
// margin shown as transmit power minus feedline plus antenna gain minus
// free-space loss minus diffraction is a number that can be argued with, which
// is the only kind an engineer will accept.
//
// Both directions side by side, always. Reachability is asymmetric and the UI
// must make that impossible to forget.
func (a *App) drawLinkBudget() {
	from, to := a.Link()
	if from < 0 || to < 0 {
		textDim("select two nodes: click one, then ctrl-click another")
		return
	}
	n1, n2 := a.Nodes[from], a.Nodes[to]
	if !n1.Kind.Transmits() && !n2.Kind.Transmits() {
		textDim("neither of these transmits")
		return
	}

	fwd, err := a.budgetTerms(n1, n2)
	if err != nil {
		textWrap(err.Error())
		return
	}
	rev, err := a.budgetTerms(n2, n1)
	if err != nil {
		textWrap(err.Error())
		return
	}

	// The asymmetric case, said out loud. A link that closes one way and not the
	// other is the single most misleading thing a pair of numbers can hide, and
	// somebody reading only the near column will deploy it.
	fm := marginOf(fwd, n2)
	rm := marginOf(rev, n1)
	if (fm >= 0) != (rm >= 0) {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		if fm >= 0 {
			textWrap(fmt.Sprintf("Asymmetric: %s can hear %s, but not the other way.",
				n2.Name, n1.Name))
		} else {
			textWrap(fmt.Sprintf("Asymmetric: %s can hear %s, but not the other way.",
				n1.Name, n2.Name))
		}
		imgui.PopStyleColor()
	}

	if imgui.BeginTableV("##budgets", 2, imgui.TableFlagsBordersInnerV, imgui.NewVec2(0, 0), 0) {
		imgui.TableNextRow()
		imgui.TableSetColumnIndex(0)
		a.drawBudgetColumn(n1.Name+"  ->  "+n2.Name, fwd, n2)
		imgui.TableSetColumnIndex(1)
		a.drawBudgetColumn(n2.Name+"  ->  "+n1.Name, rev, n1)
		imgui.EndTable()
	}
}

// marginOf is the budget's bottom line.
func marginOf(terms []budgetTerm, rx scenario.Node) float64 {
	if len(terms) == 0 {
		return math.Inf(-1)
	}
	return terms[len(terms)-1].Running - dsp.NoiseFloorDBm(rxBandwidth(rx), 6) - requiredSNR(rx)
}

func (a *App) drawBudgetColumn(title string, terms []budgetTerm, rx scenario.Node) {
	imgui.SeparatorText(title)
	if len(terms) == 0 {
		textDim("no path - select a node, then ctrl-click a second one")
		return
	}
	received := terms[len(terms)-1].Running
	noise := dsp.NoiseFloorDBm(rxBandwidth(rx), 6)
	required := requiredSNR(rx)
	margin := received - noise - required

	for _, t := range terms {
		textDim(t.Label)
		imgui.SameLineV(150, -1)
		col := imgui.NewVec4(0.9, 0.55, 0.5, 1) // a loss
		if t.Gain {
			col = imgui.NewVec4(0.5, 0.85, 0.6, 1)
		}
		imgui.PushStyleColorVec4(imgui.ColText, col)
		imgui.Text(fmt.Sprintf("%+8.1f", t.DB))
		imgui.PopStyleColor()
		imgui.SameLineV(230, -1)
		textDim(fmt.Sprintf("= %.1f dBm", t.Running))
	}

	imgui.Separator()
	textDim("noise floor")
	imgui.SameLineV(150, -1)
	imgui.Text(fmt.Sprintf("%8.1f dBm", noise))
	textDim(fmt.Sprintf("SNR needed at SF%d", rx.Radio.SpreadFactor))
	imgui.SameLineV(150, -1)
	imgui.Text(fmt.Sprintf("%8.1f dB", required))

	textDim("margin")
	imgui.SameLineV(150, -1)
	col := imgui.NewVec4(0.45, 0.85, 0.5, 1)
	switch {
	case margin < 0:
		col = imgui.NewVec4(0.9, 0.4, 0.4, 1)
	case margin < 6:
		// Six dB is the band where a model this optimistic should not be
		// trusted to have got the sign right, let alone the number.
		col = imgui.NewVec4(0.95, 0.72, 0.25, 1)
	}
	imgui.PushStyleColorVec4(imgui.ColText, col)
	imgui.Text(fmt.Sprintf("%+8.1f dB", margin))
	imgui.PopStyleColor()
	if margin >= 0 && margin < 6 {
		textWrap("Marginal. The model has no multipath and bare-earth terrain, " +
			"so go and measure this one.")
	}
}

// cutFor analyses the path between two nodes.
//
// Recomputed per direction rather than reusing the selection's cut-through: the
// terrain between two points is the same either way, but the antenna heights
// swap, and a budget that quietly used the wrong end's height would be wrong by
// exactly the amount someone is trying to measure.
func (a *App) cutFor(tx, rx scenario.Node) (pathview.CutThrough, error) {
	freq := tx.Radio.CentreHz / 1e6
	if freq <= 0 {
		freq = a.freqMHz
	}
	return pathview.Analyse(a.Terrain,
		tx.Position.Lat, tx.Position.Lon, tx.HeightAGLm,
		rx.Position.Lat, rx.Position.Lon, rx.HeightAGLm, freq, 200)
}

// worstEdgeKm is where the path is most obstructed, in km from the transmitter.
func worstEdgeKm(c pathview.CutThrough) float64 {
	if c.Worst < 0 || c.Worst >= len(c.Samples) {
		return 0
	}
	return c.Samples[c.Worst].DistM / 1000
}

// budgetTerms builds the waterfall for one direction.
func (a *App) budgetTerms(tx, rx scenario.Node) ([]budgetTerm, error) {
	cut, err := a.cutFor(tx, rx)
	if err != nil {
		return nil, err
	}

	bearing := bearingDeg(tx.Position, rx.Position)
	elev := elevationDeg(cut.TxAltM, cut.RxAltM, cut.DistanceKm)

	freq := tx.Radio.CentreHz / 1e6
	if freq <= 0 {
		freq = a.freqMHz
	}
	fspl := terrain.FSPLdB(cut.DistanceKm, freq)

	// Diffraction as its own term, because "120 dB of loss" and "that ridge at
	// 4.2 km costs you 31 dB" are different facts, and only one of them tells
	// somebody what to do about it.
	profile := make([]terrain.Point, len(cut.Samples))
	for i, s := range cut.Samples {
		profile[i] = terrain.Point{DistM: s.DistM, HeightM: s.GroundM}
	}
	diff := terrain.MultiEdgeLossDB(profile, tx.HeightAGLm, rx.HeightAGLm, freq)

	txGain := gainOf(tx, bearing, elev)
	rxGain := gainOf(rx, bearing+180, -elev)

	var out []budgetTerm
	run := tx.TxPowerDBm
	add := func(label string, db float64, gain bool) {
		run += db
		out = append(out, budgetTerm{Label: label, DB: db, Running: run, Gain: gain})
	}
	out = append(out, budgetTerm{Label: "transmit power", DB: tx.TxPowerDBm,
		Running: run, Gain: true})
	add("feedline", -tx.Antenna.FeedlineDB, false)
	add("antenna gain, in that direction", txGain, true)
	add(fmt.Sprintf("free space, %.1f km", cut.DistanceKm), -fspl, false)
	if diff > 0.05 {
		add(fmt.Sprintf("diffraction, worst edge %.1f km", worstEdgeKm(cut)), -diff, false)
	}
	add("receive antenna gain", rxGain, true)
	add("receive feedline", -rx.Antenna.FeedlineDB, false)
	return out, nil
}

func gainOf(n scenario.Node, bearing, elev float64) float64 {
	if n.Antenna.Pattern == nil {
		return 0
	}
	// Feedline is a separate line in the budget, so it must not be deducted
	// here as well — GainTowardsDBi already takes it off.
	return n.Antenna.GainTowardsDBi(bearing, elev) + n.Antenna.FeedlineDB
}

// noiseFloorFor is the receiver's own floor, from its own bandwidth.
func noiseFloorFor(n scenario.Node) float64 {
	return dsp.NoiseFloorDBm(rxBandwidth(n), 6)
}

func rxBandwidth(n scenario.Node) float64 {
	if n.Radio.BandwidthHz > 0 {
		return n.Radio.BandwidthHz
	}
	return 250e3
}

func requiredSNR(n scenario.Node) float64 {
	table := map[int]float64{7: -7.5, 8: -10, 9: -12.5, 10: -15, 11: -17.5, 12: -20}
	if v, ok := table[n.Radio.SpreadFactor]; ok {
		return v
	}
	return -15
}

func bearingDeg(a, b scenario.LatLon) float64 {
	rad := math.Pi / 180
	dLon := (b.Lon - a.Lon) * rad
	y := math.Sin(dLon) * math.Cos(b.Lat*rad)
	x := math.Cos(a.Lat*rad)*math.Sin(b.Lat*rad) -
		math.Sin(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Cos(dLon)
	return math.Mod(math.Atan2(y, x)/rad+360, 360)
}

func elevationDeg(txAlt, rxAlt, distKm float64) float64 {
	if distKm <= 0 {
		return 0
	}
	return math.Atan2(rxAlt-txAlt, distKm*1000) * 180 / math.Pi
}
