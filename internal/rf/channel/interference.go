package channel

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Emitter is a transmitter that is not part of the mesh.
//
// The reason to model these at all is that a repeater site is almost never
// empty. Masts carry paging, telemetry, private mobile radio and other ISM
// users, and a repeater sharing a mast with a 500 W paging transmitter has a
// different noise floor from one on a pole in a field — which is invisible in
// any model that assumes thermal noise and nothing else.
type Emitter struct {
	Name string

	Lat, Lon   float64
	HeightAGLm float64

	// CentreHz and BandwidthHz describe the emission. Bandwidth matters as much
	// as power: a wideband emitter puts only the fraction of its energy that
	// lands in our channel into our receiver, and treating all of it as
	// in-channel overstates the harm by orders of magnitude.
	CentreHz    float64
	BandwidthHz float64

	// ERPdBm is effective radiated power. Site licences and the Ofcom register
	// publish ERP rather than conducted power, so this is the number that will
	// actually be to hand.
	ERPdBm float64

	// DutyCycle in [0,1]. A paging transmitter keys occasionally and a telemetry
	// link is nearly continuous, and the difference decides whether a mesh loses
	// a packet now and then or stops working.
	DutyCycle float64

	// Kind is free text for the UI: "paging", "PMR", "telemetry", "ISM".
	Kind string
}

// InterferenceAt is what one emitter contributes at a receiver.
type InterferenceAt struct {
	Emitter string
	// InChannelDBm is the power landing inside the receiver's own bandwidth,
	// after path loss and the spectral overlap.
	InChannelDBm float64
	// OverlapFraction is how much of the emitter's spectrum falls in ours.
	// Zero means it is out of band entirely, which is the common case and the
	// one worth being able to state plainly.
	OverlapFraction float64
	DutyCycle       float64
}

// Receiver channel description for interference purposes.
type Channel struct {
	CentreHz    float64
	BandwidthHz float64
}

// SpectralOverlap is the fraction of an emitter's power that lands in a channel.
//
// Both are treated as flat across their bandwidth, which is crude and stated
// rather than hidden: real emissions have shaped spectra and a real receiver has
// a shaped filter, so an emitter just outside the channel is treated as
// harmless here when in reality it leaks. Modelling that properly needs an
// emission mask per emitter type, which is MSIM-21.
func SpectralOverlap(e Emitter, c Channel) float64 {
	if e.BandwidthHz <= 0 || c.BandwidthHz <= 0 {
		return 0
	}
	eLo, eHi := e.CentreHz-e.BandwidthHz/2, e.CentreHz+e.BandwidthHz/2
	cLo, cHi := c.CentreHz-c.BandwidthHz/2, c.CentreHz+c.BandwidthHz/2

	lo, hi := math.Max(eLo, cLo), math.Min(eHi, cHi)
	if hi <= lo {
		return 0
	}
	// The fraction of the *emitter's* power that lands in our channel. Dividing
	// by the channel width instead would say a narrow emitter sitting inside a
	// wide channel contributes only part of itself, which is backwards.
	return (hi - lo) / e.BandwidthHz
}

// Interference computes what a set of emitters does to a receiver.
//
// pathLossDB is supplied per emitter by the caller, because computing it needs
// terrain and that belongs above this package. Keeping it out means an emitter
// over a hill and one in line of sight are not silently treated alike.
func Interference(emitters []Emitter, pathLossDB map[string]float64, c Channel) ([]InterferenceAt, error) {
	if c.BandwidthHz <= 0 {
		return nil, fmt.Errorf("rf: receiver channel has no bandwidth")
	}
	out := make([]InterferenceAt, 0, len(emitters))
	for _, e := range emitters {
		loss, ok := pathLossDB[e.Name]
		if !ok {
			return nil, fmt.Errorf("rf: no path loss given for emitter %q", e.Name)
		}
		frac := SpectralOverlap(e, c)
		at := InterferenceAt{
			Emitter:         e.Name,
			OverlapFraction: frac,
			DutyCycle:       clamp01(e.DutyCycle),
		}
		if frac > 0 {
			at.InChannelDBm = e.ERPdBm - loss + 10*math.Log10(frac)
		} else {
			at.InChannelDBm = math.Inf(-1)
		}
		out = append(out, at)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InChannelDBm > out[j].InChannelDBm })
	return out, nil
}

// ElevatedNoiseFloorDBm combines a thermal floor with continuous interference.
//
// Only the continuous part. An emitter with a 5% duty cycle does not raise the
// noise floor — it destroys one transmission in twenty, which is a completely
// different failure and is not something a floor can express. Folding a bursty
// emitter into an average floor makes a mesh look uniformly slightly worse when
// what it actually does is fail intermittently and unpredictably, which is far
// harder to live with.
func ElevatedNoiseFloorDBm(thermalDBm float64, sources []InterferenceAt) float64 {
	total := math.Pow(10, thermalDBm/10)
	for _, s := range sources {
		if math.IsInf(s.InChannelDBm, -1) {
			continue
		}
		// Weighted by duty cycle only in the sense that a continuous source
		// counts fully; anything below "effectively always on" is left for
		// Bursty to report separately.
		if s.DutyCycle >= 0.95 {
			total += math.Pow(10, s.InChannelDBm/10)
		}
	}
	return 10 * math.Log10(total)
}

// Bursty lists the sources that will not show up in a noise floor but will
// still break links, with the fraction of transmissions each puts at risk.
func Bursty(sources []InterferenceAt, aboveDBm float64) []InterferenceAt {
	var out []InterferenceAt
	for _, s := range sources {
		if s.DutyCycle < 0.95 && s.DutyCycle > 0 && s.InChannelDBm > aboveDBm {
			out = append(out, s)
		}
	}
	return out
}

// Describe is the sentence an operator needs about a site.
func Describe(thermalDBm float64, sources []InterferenceAt) string {
	floor := ElevatedNoiseFloorDBm(thermalDBm, sources)
	var b strings.Builder
	if floor > thermalDBm+0.5 {
		fmt.Fprintf(&b, "Noise floor raised from %.1f to %.1f dBm by continuous interference.",
			thermalDBm, floor)
	} else {
		fmt.Fprintf(&b, "Noise floor is thermal at %.1f dBm; no continuous source is significant here.",
			thermalDBm)
	}
	bursts := Bursty(sources, thermalDBm)
	if len(bursts) > 0 {
		fmt.Fprintf(&b, "\n\n%d intermittent source(s) sit above the noise floor. These do not raise it — "+
			"they destroy individual transmissions, which is a different and less forgiving failure:", len(bursts))
		for _, s := range bursts {
			fmt.Fprintf(&b, "\n  %s at %.1f dBm, %.0f%% duty", s.Emitter, s.InChannelDBm, s.DutyCycle*100)
		}
	}
	return b.String()
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
