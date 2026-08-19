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

// A companion the workbench connects to interactively must actually be able
// to hear another one - the thing the review comment called "the radio is no
// longer receiving for companions in client".
//
// Real companion_radio builds have no command line, so the clock and the
// modem never reach one through the text provisioning every other node gets:
// left alone a companion boots on its firmware's own factory defaults, the
// deprecated wide EU/UK preset and a clock nothing else in the run agrees
// with, rather than the scenario's narrow one. This pins the scenario to a
// different band than the firmware's own default, so a build that forgets to
// send the modem params fails on a frequency/bandwidth mismatch rather than
// passing by accident.
func TestLiveCompanionClientReceivesAcrossConnect(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4}
	mk := func(name string, lon float64) scenario.Node {
		return scenario.Node{
			Name: name, Kind: scenario.Companion,
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
	s.build([]scenario.Node{mk("Sender", -3.900), mk("Hearer", -3.901)}, 869.618)
	if err := s.eng.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := st.Do(ctx, "companion.connect", map[string]any{"node": "Sender"}); err != nil {
		t.Fatalf("connect Sender: %v", err)
	}
	if _, err := st.Do(ctx, "companion.connect", map[string]any{"node": "Hearer"}); err != nil {
		t.Fatalf("connect Hearer: %v", err)
	}
	// Time for AppStart, the boot frames and the device query to round-trip
	// before anything is sent.
	if err := s.eng.Run(ctx, 2000); err != nil {
		t.Fatal(err)
	}

	if _, err := st.Do(ctx, "companion.send",
		map[string]any{"node": "Sender", "text": "narrowband hello", "channel": float64(0)}); err != nil {
		t.Fatalf("companion.send: %v", err)
	}
	// One contiguous run for the send itself: CSMA backoff and airtime need a
	// run long enough to complete a transmission in. Broken into many small
	// steps instead, none of them ever finished it.
	if err := s.eng.Run(ctx, 6000); err != nil {
		t.Fatal(err)
	}

	// PUSH_CODE_MSG_WAITING only sets a flag; something has to notice it and
	// send CMD_SYNC_NEXT_MESSAGE to actually collect the message - in the
	// running workbench that is refreshCompanions, on every tick, for as long
	// as the window stays open. Contiguous runs here for the same reason the
	// send needed one above.
	s.collectWaiting()
	if err := s.eng.Run(ctx, 15000); err != nil {
		t.Fatal(err)
	}
	s.collectWaiting()
	if err := s.eng.Run(ctx, 5000); err != nil {
		t.Fatal(err)
	}

	hearer := s.comps["Hearer"]
	hearer.mu.Lock()
	var texts []string
	for _, m := range hearer.messages {
		texts = append(texts, m.Text)
	}
	hearer.mu.Unlock()

	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "narrowband hello") {
			found = true
		}
	}
	if !found {
		hen, _ := s.eng.NodeByName("Hearer")
		sen, _ := s.eng.NodeByName("Sender")
		t.Fatalf("Hearer never decoded the message (sender.Sent=%d hearer.Heard=%d); messages seen: %v",
			sen.Sent, hen.Heard, texts)
	}
}
