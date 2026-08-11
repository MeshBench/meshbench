package scenario

// RadioPreset is one entry of the community preset table: a named, agreed set
// of LoRa parameters for a territory.
type RadioPreset struct {
	Label   string
	FreqMHz float64
	BwKHz   float64
	SF      int
	// CR is the denominator minus four, as MeshCore's CLI takes it: 5..8 on the
	// wire means 4/5..4/8.
	CR int
}

// RadioPresets is the MeshCore community's own preset list, from
// https://forum.letsmesh.net/t/meshcore-radio-setting-presets/67 — the same
// source HopReach bakes in.
//
// Baked in rather than fetched, for the same two reasons: the workbench works
// offline, and a preset is an *agreement between operators* — silently tracking
// upstream edits would mean the table in a saved scenario no longer matched the
// table the scenario was built with. Re-transcribe deliberately when the forum
// list changes.
var RadioPresets = []RadioPreset{
	{"Australia", 915.8, 250.0, 10, 5},
	{"Australia (Narrow)", 916.575, 62.5, 7, 8},
	{"Australia (Mid)", 915.075, 125.0, 9, 5},
	{"Australia: SA, WA", 923.125, 62.5, 8, 8},
	{"Australia: QLD", 923.125, 62.5, 8, 5},
	{"Brazil", 923.125, 62.5, 8, 8},
	{"EU/UK (Narrow)", 869.618, 62.5, 8, 8},
	{"EU/UK (Deprecated)", 869.525, 250.0, 11, 5},
	{"Czech Republic (Narrow)", 869.432, 62.5, 7, 5},
	{"EU 433MHz (Long Range)", 433.65, 250.0, 11, 5},
	{"EU 433MHz (Narrow)", 433.65, 62.5, 8, 8},
	{"Netherlands", 869.618, 62.5, 7, 5},
	{"New Zealand", 917.375, 250.0, 11, 5},
	{"New Zealand (Narrow)", 917.375, 62.5, 7, 5},
	{"Portugal 433", 433.375, 62.5, 9, 6},
	{"Portugal 868", 869.618, 62.5, 7, 6},
	{"Switzerland", 869.618, 62.5, 8, 8},
	{"USA/Canada (Recommended)", 910.525, 62.5, 7, 5},
	{"Vietnam (Narrow)", 920.25, 62.5, 8, 5},
	{"Vietnam (Deprecated)", 920.25, 250.0, 11, 5},
}

// DefaultPreset is EU/UK (Narrow) — the workbench is built in Scotland, and it
// is also HopReach's default, so the two tools agree out of the box.
const DefaultPreset = "EU/UK (Narrow)"

// PresetByLabel finds a preset, and reports whether it exists.
func PresetByLabel(label string) (RadioPreset, bool) {
	for _, p := range RadioPresets {
		if p.Label == label {
			return p, true
		}
	}
	return RadioPreset{}, false
}

// Config converts a preset to the RadioConfig a node carries.
func (p RadioPreset) Config() RadioConfig {
	return RadioConfig{
		CentreHz:     p.FreqMHz * 1e6,
		BandwidthHz:  p.BwKHz * 1000,
		SpreadFactor: p.SF,
		// RadioConfig's CodingRate is 1..4; the preset table uses MeshCore's
		// 5..8. One representation per package, converted at the boundary.
		CodingRate: p.CR - 4,
	}
}

// Matches reports whether a node's radio is exactly this preset. Used to show
// which preset a node is on, if any — a node can always be off-table.
func (p RadioPreset) Matches(r RadioConfig) bool {
	return r.CentreHz == p.FreqMHz*1e6 &&
		r.BandwidthHz == p.BwKHz*1000 &&
		r.SpreadFactor == p.SF &&
		r.CodingRate == p.CR-4
}

// DefaultRadio is the modem configuration a new or imported node gets.
//
// The named preset rather than numbers copied into four call sites: every one
// of them said 869.525 MHz at 250 kHz - the *wide* preset - while the
// constant above declared the narrow one, so nothing in the workbench
// actually started on the default it documented.
func DefaultRadio() RadioConfig {
	for _, p := range RadioPresets {
		if p.Label == DefaultPreset {
			return p.Config()
		}
	}
	return RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500, SpreadFactor: 8, CodingRate: 4}
}

// DefaultFreqMHz is that preset's frequency, for the chrome that shows one.
func DefaultFreqMHz() float64 {
	for _, p := range RadioPresets {
		if p.Label == DefaultPreset {
			return p.FreqMHz
		}
	}
	return 869.618
}
