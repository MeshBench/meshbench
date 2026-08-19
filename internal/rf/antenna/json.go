package antenna

import (
	"encoding/json"
	"fmt"
)

// JSON for Mounted, with the pattern type-tagged.
//
// Pattern is an interface, and encoding/json cannot invent a concrete type on
// the way back in — so before this existed, a scenario saved to disk loaded
// with every antenna nil, which turned into zero gain everywhere and looked
// exactly like a propagation problem. The tag makes the round trip explicit,
// and an unknown tag is an error rather than a silently isotropic mast.

type mountedJSON struct {
	Pattern      patternJSON  `json:"pattern"`
	BearingDeg   float64      `json:"bearing_deg,omitempty"`
	DowntiltDeg  float64      `json:"downtilt_deg,omitempty"`
	Polarisation Polarisation `json:"polarisation,omitempty"`
	FeedlineDB   float64      `json:"feedline_db,omitempty"`
}

type patternJSON struct {
	Type string `json:"type"`
	// The union of every pattern's fields; zero values are omitted so each
	// type's JSON carries only what it uses.
	GainDBiPeak   float64 `json:"gain_dbi_peak,omitempty"`
	BeamwidthDeg  float64 `json:"beamwidth_deg,omitempty"`
	FrontToBackDB float64 `json:"front_to_back_db,omitempty"`
}

func (m Mounted) MarshalJSON() ([]byte, error) {
	out := mountedJSON{
		BearingDeg: m.BearingDeg, DowntiltDeg: m.DowntiltDeg,
		Polarisation: m.Polarisation, FeedlineDB: m.FeedlineDB,
	}
	switch p := m.Pattern.(type) {
	case nil:
		// A mount with no pattern is a modelling gap, not a thing to persist.
		out.Pattern = patternJSON{Type: "isotropic"}
	case Isotropic:
		out.Pattern = patternJSON{Type: "isotropic"}
	case Dipole:
		out.Pattern = patternJSON{Type: "dipole"}
	case Collinear:
		out.Pattern = patternJSON{Type: "collinear", GainDBiPeak: p.GainDBiPeak}
	case Yagi:
		out.Pattern = patternJSON{Type: "yagi", GainDBiPeak: p.GainDBiPeak,
			BeamwidthDeg: p.BeamwidthDeg, FrontToBackDB: p.FrontToBackDB}
	default:
		return nil, fmt.Errorf("antenna: no JSON form for pattern %q", m.Pattern.Name())
	}
	return json.Marshal(out)
}

func (m *Mounted) UnmarshalJSON(b []byte) error {
	var in mountedJSON
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	m.BearingDeg, m.DowntiltDeg = in.BearingDeg, in.DowntiltDeg
	m.Polarisation, m.FeedlineDB = in.Polarisation, in.FeedlineDB
	switch in.Pattern.Type {
	case "", "isotropic":
		// Absent means unstated, and isotropic is the one honest reading of
		// "an antenna nobody described": no direction is favoured.
		m.Pattern = Isotropic{}
	case "dipole":
		m.Pattern = Dipole{}
	case "collinear":
		m.Pattern = Collinear{GainDBiPeak: in.Pattern.GainDBiPeak}
	case "yagi":
		m.Pattern = Yagi{GainDBiPeak: in.Pattern.GainDBiPeak,
			BeamwidthDeg: in.Pattern.BeamwidthDeg, FrontToBackDB: in.Pattern.FrontToBackDB}
	default:
		return fmt.Errorf("antenna: unknown pattern type %q", in.Pattern.Type)
	}
	return nil
}
