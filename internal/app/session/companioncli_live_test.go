package session

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A companion logged in to a repeater runs a command on it over the air and
// reads the answer back (#253). Nothing here is modelled: the repeater's own
// firmware takes TXT_TYPE_CLI_DATA from an admin and answers in kind, and the
// verb only originates it.
func TestLiveACompanionRunsACommandOnARepeater(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4}
	mk := func(name string, kind scenario.Kind, lon float64) scenario.Node {
		return scenario.Node{
			Name: name, Kind: kind,
			Position: scenario.LatLon{Lat: 56.7, Lon: lon}, HeightAGLm: 2,
			Antenna:       antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
			TxPowerDBm:    14,
			NoiseFigureDB: 6,
			Radio:         radio,
		}
	}

	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	go st.Run(ctx)
	s.build([]scenario.Node{mk("Phone", scenario.Companion, -3.900), mk("Rep", scenario.SimpleRepeater, -3.901)}, 869.618)
	if err := s.eng.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Run takes an instant, not a duration; this runs for one, collecting
	// whatever the companion says is waiting as it goes.
	run := func(ms uint32) {
		t.Helper()
		end := s.eng.NowMs() + ms
		for s.eng.NowMs() < end {
			s.collectWaiting()
			if err := s.eng.Run(ctx, min(end, s.eng.NowMs()+2000)); err != nil {
				t.Fatal(err)
			}
		}
	}
	do := func(verb string, p map[string]any) any {
		t.Helper()
		v, err := st.Do(ctx, verb, p)
		if err != nil {
			t.Fatalf("%s: %v", verb, err)
		}
		return v
	}

	do("companion.connect", map[string]any{"node": "Phone"})
	run(2000)
	// The repeater has to be a contact before anything can be addressed to it.
	do("console.type", map[string]any{"node": "Rep", "command": "advert"})
	run(8000)
	do("console.cli", map[string]any{"node": "Phone", "command": "contacts"})
	run(2000)
	// Unprovisioned, the repeater adverts under its firmware's own name, so
	// it is addressed as the contact the phone actually holds.
	phone := s.comps["Phone"]
	phone.mu.Lock()
	if len(phone.contacts) == 0 {
		phone.mu.Unlock()
		t.Fatal("the phone never heard the repeater's advert")
	}
	rep := phone.contacts[0].Name
	phone.mu.Unlock()
	do("console.cli", map[string]any{"node": "Phone", "command": "login " + rep + " password"})
	run(10000)
	got := do("companion.cli", map[string]any{"node": "Phone", "to": rep, "command": "get name"}).(map[string]any)
	if got["to"] != rep {
		t.Fatalf("companion.cli answered %v", got)
	}
	run(14000)

	state := do("companion.state", map[string]any{"node": "Phone"}).(map[string]any)
	replies, _ := state["cli_replies"].([]map[string]any)
	for _, r := range replies {
		if strings.Contains(r["text"].(string), rep) {
			t.Logf("%s answered: %q", rep, r["text"])
			return
		}
	}
	t.Fatalf("no answer to get name came back: replies %v; recent %v", replies, state["recent"])
}
