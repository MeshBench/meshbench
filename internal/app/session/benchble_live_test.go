package session

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// A companion served over BLE is advertised on this machine's adapter, shows
// as a BLE endpoint, and stops being advertised when it is dropped (#256).
// Needs an adapter and BlueZ, which the CI runners have not got.
func TestLiveAServedNodeIsAdvertisedOverBLE(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	if _, err := os.Stat("/sys/class/bluetooth/hci0"); err != nil {
		t.Skip("no Bluetooth adapter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	st := state.New(10)
	s := &Sim{gpuAsked: true}
	Register(st, s)
	go st.Run(ctx)
	phone := scenario.Node{
		Name: "Phone", Kind: scenario.Companion,
		Position: scenario.LatLon{Lat: 56.7, Lon: -3.9}, HeightAGLm: 2,
		Antenna:    antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
		TxPowerDBm: 14, NoiseFigureDB: 6,
		Radio: scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4},
	}
	s.build([]scenario.Node{phone}, 869.618)
	if err := s.eng.AttachNative(ctx, 4417); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	got, err := st.Do(ctx, "bench.serve", map[string]any{"node": "Phone", "kind": "ble"})
	if err != nil {
		t.Fatal(err)
	}
	if addr := got.(map[string]any)["addr"]; addr != "MSIM-Phone" {
		t.Fatalf("advertised as %v", addr)
	}
	eps := s.endpoints()
	if len(eps) != 1 || eps[0].Kind != "ble" || !eps[0].Attached {
		t.Fatalf("endpoints %+v, want one BLE endpoint with the peripheral attached", eps)
	}

	if _, err := st.Do(ctx, "bench.drop", map[string]any{"node": "Phone"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.bleName("Phone"); !ok {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the peripheral outlived its endpoint")
}
