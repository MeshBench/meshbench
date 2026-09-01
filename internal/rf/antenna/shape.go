package antenna

import "fmt"

// Shape is a pattern in the vocabulary somebody chooses an antenna in: which
// sort it is, and the two or three numbers that sort is quoted by.
//
// It exists because that pairing is needed in three places which have to agree
// - the JSON a scenario is saved as, the verb a script aims an antenna with,
// and the form a person fills in - and a fourth copy of the same switch is how
// a yagi comes back off disk as an omni. One switch, in the package that owns
// the patterns.
//
// The json tags are the on-disk names, so this is also the pattern half of a
// saved Mounted rather than a separate shape that must be kept in step with it.
type Shape struct {
	Type string `json:"type"`
	// The union of every pattern's parameters. Zero values are omitted, so each
	// sort carries only what it uses.
	GainDBiPeak   float64 `json:"gain_dbi_peak,omitempty"`
	BeamwidthDeg  float64 `json:"beamwidth_deg,omitempty"`
	FrontToBackDB float64 `json:"front_to_back_db,omitempty"`
}

// Types are the sorts of antenna that can be built, in the order they are worth
// meeting: the reference, then the two omnis, then the beam.
func Types() []string { return []string{"isotropic", "dipole", "collinear", "yagi"} }

// ShapeOf describes a pattern. A pattern this package did not make has no
// description, which is an error rather than a plausible-looking omni.
func ShapeOf(p Pattern) (Shape, error) {
	switch v := p.(type) {
	case nil:
		// A mount with no pattern is a modelling gap, not a thing to describe.
		return Shape{Type: "isotropic"}, nil
	case Isotropic:
		return Shape{Type: "isotropic"}, nil
	case Dipole:
		return Shape{Type: "dipole"}, nil
	case Collinear:
		return Shape{Type: "collinear", GainDBiPeak: v.GainDBiPeak}, nil
	case Yagi:
		return Shape{Type: "yagi", GainDBiPeak: v.GainDBiPeak,
			BeamwidthDeg: v.BeamwidthDeg, FrontToBackDB: v.FrontToBackDB}, nil
	default:
		return Shape{}, fmt.Errorf("antenna: no description for pattern %q", p.Name())
	}
}

// Pattern builds the pattern a Shape describes.
//
// An unnamed sort means unstated, and isotropic is the one honest reading of
// "an antenna nobody described": no direction is favoured. An unknown one is an
// error, because silently substituting an omni for a beam somebody asked for
// would change the answer and say nothing.
func (s Shape) Pattern() (Pattern, error) {
	switch s.Type {
	case "", "isotropic":
		return Isotropic{}, nil
	case "dipole":
		return Dipole{}, nil
	case "collinear":
		return Collinear{GainDBiPeak: s.GainDBiPeak}, nil
	case "yagi":
		return Yagi{GainDBiPeak: s.GainDBiPeak,
			BeamwidthDeg: s.BeamwidthDeg, FrontToBackDB: s.FrontToBackDB}, nil
	}
	return nil, fmt.Errorf("antenna: unknown pattern type %q", s.Type)
}
