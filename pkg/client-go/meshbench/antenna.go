package meshbench

import "context"

// Antenna is what a node stands under: which sort, and where it points.
//
// Directional in azimuth, so the bearing is not decoration. A beam is twenty
// decibels or more down off its boresight, and the difference between a node
// aimed down a valley and the same node aimed at a hillside is the difference
// between a link and no link.
type Antenna struct {
	Node string `json:"node"`
	// Pattern is "isotropic", "dipole", "collinear" or "yagi", and empty for a
	// node that has no antenna at all - which is not the same as an omni at 0
	// dBi, and is reported rather than filled in.
	Pattern       string  `json:"pattern"`
	GainDBiPeak   float64 `json:"gain_dbi_peak"`
	BeamwidthDeg  float64 `json:"beamwidth_deg"`
	FrontToBackDB float64 `json:"front_to_back_db"`
	// BearingDeg is the compass bearing of the boresight, 0 at north.
	BearingDeg float64 `json:"bearing_deg"`
	// DowntiltDeg tilts the beam below the horizon, which is what a mast on a
	// hill does to reach the town underneath it.
	DowntiltDeg float64 `json:"downtilt_deg"`
	// Polarisation is "vertical", "horizontal" or "circular", and empty for a
	// node that has not said. Unstated costs nothing; a mismatch costs 3 dB
	// circular to linear and 20 dB vertical to horizontal.
	Polarisation string `json:"polarisation"`
	// FeedlineDB is cable and connector loss, as a positive number.
	FeedlineDB float64 `json:"feedline_db"`
	// PeakDBi is what the pattern manages on its own boresight, before the
	// feedline.
	PeakDBi float64 `json:"peak_dbi"`
}

// AntennaChange is what to change about an antenna. Nil leaves a field alone,
// because "leave this" and "set it to zero" are different answers and a float
// cannot say both - the same reason CardChange takes pointers.
type AntennaChange struct {
	Pattern       *string
	GainDBiPeak   *float64
	BeamwidthDeg  *float64
	FrontToBackDB *float64
	BearingDeg    *float64
	DowntiltDeg   *float64
	Polarisation  *string
	FeedlineDB    *float64
}

func (c AntennaChange) params() map[string]any {
	p := map[string]any{}
	if c.Pattern != nil {
		p["pattern"] = *c.Pattern
	}
	if c.GainDBiPeak != nil {
		p["gain_dbi_peak"] = *c.GainDBiPeak
	}
	if c.BeamwidthDeg != nil {
		p["beamwidth_deg"] = *c.BeamwidthDeg
	}
	if c.FrontToBackDB != nil {
		p["front_to_back_db"] = *c.FrontToBackDB
	}
	if c.BearingDeg != nil {
		p["bearing_deg"] = *c.BearingDeg
	}
	if c.DowntiltDeg != nil {
		p["downtilt_deg"] = *c.DowntiltDeg
	}
	if c.Polarisation != nil {
		p["polarisation"] = *c.Polarisation
	}
	if c.FeedlineDB != nil {
		p["feedline_db"] = *c.FeedlineDB
	}
	return p
}

// Aimed is where an antenna ended up pointing, and what that won.
//
// GainDBi is the point of it: on an omni the answer is the same as before, and
// a call that reported success while changing nothing would be one to distrust.
type Aimed struct {
	Node       string  `json:"node"`
	At         string  `json:"at"`
	BearingDeg float64 `json:"bearing_deg"`
	DistanceKm float64 `json:"distance_km"`
	GainDBi    float64 `json:"gain_dbi"`
}

// Antenna reports what this node stands under and which way it points.
func (n Node) Antenna(ctx context.Context) (Antenna, error) {
	var out Antenna
	return out, n.w.CallInto(ctx, "node.antenna", map[string]any{"node": n.name}, &out)
}

// SetAntenna changes this node's antenna. What is left nil is left alone, so
// turning a beam does not restate the beam.
func (n Node) SetAntenna(ctx context.Context, c AntennaChange) error {
	p := c.params()
	p["node"] = n.name
	return n.w.Do(ctx, "nodes.antenna", p)
}

// Aim turns this node's antenna towards another node.
//
// The bearing between two placed nodes is exact, so this is a better answer
// than reading one off a map and typing it back.
func (n Node) Aim(ctx context.Context, at string) (Aimed, error) {
	var out Aimed
	return out, n.w.CallInto(ctx, "node.aim",
		map[string]any{"node": n.name, "at": at}, &out)
}

// SetAntenna gives every node the same antenna, or every node of one kind.
//
// The fleet-level default, and the only way a large scenario gets one: setting
// fifty-eight nodes by hand is not a workflow anybody will use. Pass an empty
// kind for every node.
func (n Nodes) SetAntenna(ctx context.Context, kind Kind, c AntennaChange) error {
	p := c.params()
	if kind != "" {
		p["kind"] = string(kind)
	}
	return n.w.Do(ctx, "nodes.antenna", p)
}
