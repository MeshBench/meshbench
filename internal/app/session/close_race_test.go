package session

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Close used to run on a goroutine that was not the store's while a verb
// handler on the store's own goroutine wrote the same served/servedAddrs
// maps - a concurrent map write, which the Go runtime kills the process for
// rather than letting a caller recover from. It also meant Close could die
// partway through, before runTeardowns and eng.Close ever ran, leaving
// firmware child processes behind it.
//
// This races the served-map helpers directly rather than a live engine's
// ServeCompanionTCP, so what it exercises is the race this fix is actually
// about - two goroutines touching served/servedAddrs at once - and not a
// real listener's own socket lifecycle. Run with -race.
func TestCloseDoesNotRaceServedCompanions(t *testing.T) {
	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62500, SpreadFactor: 8, CodingRate: 4}
	node := scenario.Node{
		Name: "Comp", Kind: scenario.Companion,
		Position: scenario.LatLon{Lat: 56.7, Lon: -3.9}, HeightAGLm: 2,
		Antenna:       antenna.Mounted{Pattern: antenna.Dipole{}, Polarisation: "vertical"},
		TxPowerDBm:    14,
		NoiseFigureDB: 6,
		Radio:         radio,
	}
	s := &Sim{gpuAsked: true}
	s.build([]scenario.Node{node}, 869.618)

	// Saved and restored so this test's own hook does not leak into whatever
	// runs after it in the same process.
	saved := teardowns
	t.Cleanup(func() { teardowns = saved })
	var torndown atomic.Bool
	RegisterTeardown(func(*Sim) { torndown.Store(true) })

	const rounds = 500
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		close(started)
		for i := 0; i < rounds; i++ {
			s.setServedLink("Comp", &engine.CompanionLink{Node: "Comp", Kind: "tcp", Addr: "127.0.0.1:0"}, nil)
			s.stopServing("Comp")
		}
	}()

	<-started
	s.Close()
	wg.Wait()

	if !torndown.Load() {
		t.Error("Close returned without running teardown - firmware was never stopped")
	}
}

// Close on a running store takes the engine away on the store's goroutine,
// and on a store that never ran it does not wait for one.
func TestCloseDetachesThroughTheStoreOnlyWhenItRuns(t *testing.T) {
	for _, run := range []bool{true, false} {
		st := state.New(10)
		s := &Sim{gpuAsked: true}
		Register(st, s)
		if run {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			go st.Run(ctx)
			for !st.Running() {
				runtime.Gosched()
			}
		}
		s.build(nil, 869.618)
		if s.eng == nil {
			t.Fatal("no engine to close")
		}
		start := time.Now()
		s.Close()
		if took := time.Since(start); took > 2*time.Second {
			t.Errorf("run=%v: Close took %v", run, took)
		}
		if s.eng != nil {
			t.Errorf("run=%v: the engine is still on the Sim after Close", run)
		}
	}
}
