package nodeantenna

import (
	"fmt"
	"math"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// polarisations are the ones the model prices. A fourth spelling is refused
// rather than stored, because CrossPolLossDB reads an unrecognised value as
// orthogonal to everything and would take a link off the air over a typo.
var polarisations = []antenna.Polarisation{
	antenna.Vertical, antenna.Horizontal, antenna.Circular,
}

// overlay is the antenna a node ends up with when only some of it was said.
//
// Partial on purpose: the useful call is "turn this one 40 degrees", not "state
// every parameter of an antenna again". So the node's own antenna is the
// starting point and each named field replaces one part of it, which also means
// switching a collinear to a yagi keeps the gain figure somebody already chose.
func overlay(cur antenna.Mounted, p any) (antenna.Mounted, error) {
	shape, err := antenna.ShapeOf(cur.Pattern)
	if err != nil {
		return cur, err
	}
	if v, ok := session.NamedField(p, "pattern"); ok && strings.TrimSpace(v) != "" {
		shape.Type = strings.ToLower(strings.TrimSpace(v))
	}
	if v, ok := session.NamedNum(p, "gain_dbi_peak"); ok {
		shape.GainDBiPeak = v
	}
	if v, ok := session.NamedNum(p, "beamwidth_deg"); ok {
		shape.BeamwidthDeg = v
	}
	if v, ok := session.NamedNum(p, "front_to_back_db"); ok {
		shape.FrontToBackDB = v
	}
	pattern, err := shape.Pattern()
	if err != nil {
		return cur, fmt.Errorf("%w; the sorts are %s", err,
			strings.Join(antenna.Types(), ", "))
	}
	out := cur
	out.Pattern = pattern
	if v, ok := session.NamedNum(p, "bearing_deg"); ok {
		out.BearingDeg = compass(v)
	}
	if v, ok := session.NamedNum(p, "downtilt_deg"); ok {
		out.DowntiltDeg = v
	}
	if v, ok := session.NamedNum(p, "feedline_db"); ok {
		out.FeedlineDB = v
	}
	if v, ok := session.NamedField(p, "polarisation"); ok && strings.TrimSpace(v) != "" {
		pol, err := polarisation(v)
		if err != nil {
			return cur, err
		}
		out.Polarisation = pol
	}
	return out, nil
}

// polarisation reads one of the three, and refuses anything else.
func polarisation(s string) (antenna.Polarisation, error) {
	want := antenna.Polarisation(strings.ToLower(strings.TrimSpace(s)))
	for _, p := range polarisations {
		if p == want {
			return p, nil
		}
	}
	var names []string
	for _, p := range polarisations {
		names = append(names, string(p))
	}
	return "", fmt.Errorf("polarisation %q is not one of %s", s, strings.Join(names, ", "))
}

// compass folds a bearing into [0, 360). A sweep counts upwards past north and
// a person types -90 for west, and both should mean what they obviously mean.
func compass(deg float64) float64 {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	return d
}

// describe is one node's antenna as a verb answers with it: the same words the
// verb that sets it takes, so what comes back can be handed straight back in.
func describe(n scenario.Node) map[string]any {
	shape, err := antenna.ShapeOf(n.Antenna.Pattern)
	out := map[string]any{
		"node":             n.Name,
		"pattern":          shape.Type,
		"gain_dbi_peak":    shape.GainDBiPeak,
		"beamwidth_deg":    shape.BeamwidthDeg,
		"front_to_back_db": shape.FrontToBackDB,
		"bearing_deg":      n.Antenna.BearingDeg,
		"downtilt_deg":     n.Antenna.DowntiltDeg,
		"polarisation":     string(n.Antenna.Polarisation),
		"feedline_db":      n.Antenna.FeedlineDB,
	}
	if err != nil || n.Antenna.Pattern == nil {
		// Said, not filled in. A node with no antenna and a node with an omni
		// at 0 dBi read identically in a table of numbers, and only one of them
		// is a scenario somebody meant.
		out["pattern"] = ""
		out["peak_dbi"] = 0.0
		return out
	}
	out["peak_dbi"] = n.Antenna.Pattern.PeakDBi()
	return out
}
