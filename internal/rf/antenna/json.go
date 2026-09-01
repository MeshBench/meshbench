package antenna

import "encoding/json"

// JSON for Mounted, with the pattern type-tagged.
//
// Pattern is an interface, and encoding/json cannot invent a concrete type on
// the way back in — so before this existed, a scenario saved to disk loaded
// with every antenna nil, which turned into zero gain everywhere and looked
// exactly like a propagation problem. The tag makes the round trip explicit,
// and an unknown tag is an error rather than a silently isotropic mast.
//
// The tag and its parameters are Shape, which is also what the verbs and the
// form speak, so what a person can choose and what can be saved cannot drift.

type mountedJSON struct {
	Pattern      Shape        `json:"pattern"`
	BearingDeg   float64      `json:"bearing_deg,omitempty"`
	DowntiltDeg  float64      `json:"downtilt_deg,omitempty"`
	Polarisation Polarisation `json:"polarisation,omitempty"`
	FeedlineDB   float64      `json:"feedline_db,omitempty"`
}

func (m Mounted) MarshalJSON() ([]byte, error) {
	shape, err := ShapeOf(m.Pattern)
	if err != nil {
		return nil, err
	}
	return json.Marshal(mountedJSON{
		Pattern:    shape,
		BearingDeg: m.BearingDeg, DowntiltDeg: m.DowntiltDeg,
		Polarisation: m.Polarisation, FeedlineDB: m.FeedlineDB,
	})
}

func (m *Mounted) UnmarshalJSON(b []byte) error {
	var in mountedJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	p, err := in.Pattern.Pattern()
	if err != nil {
		return err
	}
	m.BearingDeg, m.DowntiltDeg = in.BearingDeg, in.DowntiltDeg
	m.Polarisation, m.FeedlineDB = in.Polarisation, in.FeedlineDB
	m.Pattern = p
	return nil
}
