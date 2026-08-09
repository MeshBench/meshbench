package scenario

import (
	"fmt"
	"sort"
	"strings"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/provider"
)

// ImportOptions control how provider records become scenario nodes.
type ImportOptions struct {
	// DefaultBoard is used where the record does not say what hardware a node
	// is. Required: there is no neutral choice, and picking one silently means
	// every imported node gets someone's guess at a transmit power.
	DefaultBoard string

	// Radio is the network's modem configuration. Not a property of a node
	// record — no source publishes it — so the operator supplies it once.
	Radio RadioConfig

	// MaxUncertaintyKm is how loosely a node may be placed and still be
	// imported as a real node. Above it the node is still returned, but flagged:
	// dropping it loses the fact that it exists, and importing it silently
	// pretends to know where it is.
	MaxUncertaintyKm float64

	// Region, if set, limits the import. Nodes outside the boundary but within
	// its RF margin are kept as participants — a repeater just outside still
	// relays to and interferes with nodes inside, and dropping it produces a
	// mesh that behaves better than reality.
	Region *Region
}

// Imported is one record's outcome.
type Imported struct {
	Node Node

	// Uncertain marks a node placed too loosely to trust. It is in the scenario
	// and it will be simulated, but any result involving it carries this.
	Uncertain bool

	// Participant marks a node outside the region that is kept for its RF only.
	// Results are reported for what is inside; the RF is computed over both.
	Participant bool

	// Warnings are what the operator should know before believing anything
	// about this node.
	Warnings []string
}

// ImportResult is the whole import.
type ImportResult struct {
	Nodes []Imported

	SkippedNoPosition int
	SkippedOutside    int
	Uncertain         int
	Participants      int
}

// Import turns provider records into scenario nodes.
//
// The rule throughout: a record that cannot be placed is reported, not invented.
// Every source gives partial data — a node with no position, a position with no
// accuracy, a name with no hardware — and the temptation at each gap is to fill
// it with something reasonable. That produces a scenario full of confident
// fiction, and nothing downstream can tell which parts were real.
func Import(records []provider.NodeRecord, o ImportOptions) (ImportResult, error) {
	if o.DefaultBoard == "" {
		return ImportResult{}, fmt.Errorf(
			"scenario: import needs a default board; there is no neutral choice, and " +
				"guessing one gives every node someone else's transmit power")
	}
	board, err := BoardByName(o.DefaultBoard)
	if err != nil {
		return ImportResult{}, err
	}
	if o.Radio.CentreHz <= 0 || o.Radio.BandwidthHz <= 0 {
		return ImportResult{}, fmt.Errorf(
			"scenario: import needs a radio configuration; no source publishes one")
	}
	if o.MaxUncertaintyKm <= 0 {
		o.MaxUncertaintyKm = 1.0
	}

	var res ImportResult
	seen := map[string]bool{}

	for _, r := range records {
		if !r.HasPosition {
			// Kept out of the scenario but counted. A node with no position
			// cannot be simulated; pretending it is at (0,0) puts it in the
			// Atlantic and it will quietly fail to reach anything.
			res.SkippedNoPosition++
			continue
		}

		name := r.Name
		if name == "" {
			name = shortKey(r.PublicKey)
		}
		if name == "" || seen[strings.ToLower(name)] {
			// A duplicate name would make two nodes indistinguishable in every
			// ledger entry and every export.
			name = uniqueName(name, r.PublicKey, seen)
		}
		seen[strings.ToLower(name)] = true

		pos := LatLon{Lat: r.Lat, Lon: r.Lon}
		imp := Imported{}

		if o.Region != nil {
			switch {
			case o.Region.Contains(pos):
			case o.Region.Participates(pos):
				imp.Participant = true
				res.Participants++
				imp.Warnings = append(imp.Warnings,
					"outside the study area but within RF range of it; simulated, but not reported on")
			default:
				res.SkippedOutside++
				continue
			}
		}

		n := Node{
			Name:          name,
			Kind:          kindFor(r.Kind),
			Position:      pos,
			UncertaintyKm: r.UncertaintyKm,
			HeightAGLm:    r.HeightAGLm,
			Radio:         o.Radio,
			NoiseFigureDB: board.NoiseFigureDB,
			Antenna: antenna.Mounted{
				Pattern:      antenna.Collinear{GainDBiPeak: board.AntennaDBi + 4},
				Polarisation: "vertical",
				FeedlineDB:   board.FeedlineDB,
			},
		}
		if n.Kind.Transmits() {
			n.TxPowerDBm = board.MaxTxDBm
		}
		if n.HeightAGLm <= 0 {
			// A height nobody recorded. Ten metres is a plausible repeater mast
			// and an implausible handheld, which is precisely why it is flagged
			// rather than quietly applied — after terrain, height is the largest
			// factor in whether a VHF/UHF path works.
			n.HeightAGLm = 10
			imp.Warnings = append(imp.Warnings,
				"no antenna height recorded; assumed 10 m, which after terrain is the "+
					"largest single factor in whether this node reaches anything")
		}
		if r.UncertaintyKm > o.MaxUncertaintyKm {
			imp.Uncertain = true
			res.Uncertain++
			imp.Warnings = append(imp.Warnings, fmt.Sprintf(
				"position is good to about %.1f km; at 869 MHz that is several dB of path "+
					"loss, so results involving this node are indicative rather than answers",
				r.UncertaintyKm))
		}
		if r.Kind == "" {
			imp.Warnings = append(imp.Warnings,
				fmt.Sprintf("no role recorded; imported as %s", n.Kind))
		}

		if err := n.Validate(); err != nil {
			return ImportResult{}, fmt.Errorf("scenario: importing %s: %w", name, err)
		}
		imp.Node = n
		res.Nodes = append(res.Nodes, imp)
	}

	sort.Slice(res.Nodes, func(i, j int) bool { return res.Nodes[i].Node.Name < res.Nodes[j].Node.Name })
	return res, nil
}

// Describe states what was imported and what was not.
func (r ImportResult) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d nodes imported.", len(r.Nodes))
	if r.Uncertain > 0 {
		fmt.Fprintf(&b, " %d are placed too loosely to trust to a decibel and are marked.", r.Uncertain)
	}
	if r.Participants > 0 {
		fmt.Fprintf(&b, " %d sit outside the study area but within RF range, so they are "+
			"simulated without being reported on.", r.Participants)
	}
	if r.SkippedNoPosition > 0 {
		fmt.Fprintf(&b, "\n%d records had no position at all and were left out — they exist, "+
			"but nothing can be computed about where.", r.SkippedNoPosition)
	}
	if r.SkippedOutside > 0 {
		fmt.Fprintf(&b, "\n%d were beyond the study area and its RF margin.", r.SkippedOutside)
	}
	return b.String()
}

// kindFor maps a source's role string onto a node kind.
//
// Unknown roles become simple repeaters rather than being rejected: a source
// that invents a new role name should not stop an import, and a repeater is the
// conservative assumption — it transmits, so it is accounted for in everyone
// else's interference rather than silently absent from it.
func kindFor(role string) Kind {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "companion", "client", "phone":
		return Companion
	case "room-server", "room_server", "advanced", "advanced-repeater":
		return AdvancedRepeater
	case "observer", "sdr", "sdr-observer":
		return SDRObserver
	default:
		return SimpleRepeater
	}
}

func shortKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:8]
}

func uniqueName(base, key string, seen map[string]bool) string {
	if base == "" {
		base = "node"
	}
	if k := shortKey(key); k != "" {
		candidate := base + "-" + k
		if !seen[strings.ToLower(candidate)] {
			return candidate
		}
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !seen[strings.ToLower(candidate)] {
			return candidate
		}
	}
}
