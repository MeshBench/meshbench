package engine_test

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// The workbench reads a run while it is running: the scoreboard redraws, a
// sensitivity card polls, an attached SDR client pulls IQ, and the operator
// drags an observer across the map - all off the goroutine that is stepping.
// Under -race that used to report the engine writing a node's transmit power
// while another goroutine read it out of the same struct, because the
// lock-copy-unlock snapshot copies pointers and the readers dereference them
// after the lock is gone.
func TestReadersDoNotRaceAStep(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417})
	defer func() { _ = e.Close() }()
	e.Add(wfNode("a", 0, 22), nil)
	e.Add(wfNode("b", 0.010, 22), nil)
	e.Add(wfNode("c", 0.020, 22), nil)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	spin := func(f func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				f(i)
			}
		}()
	}

	spin(func(int) { _ = e.Scoreboard() })
	spin(func(int) { _ = e.Sensitivity() })
	spin(func(int) { _ = e.ObserveSpan(1, e.NowMs(), 256) })
	spin(func(int) { _ = e.LinkMargins() })
	// The write the readers are racing: MeshCore reports its radio every tick,
	// and the engine turns that into a new transmit power and noise figure.
	spin(func(i int) {
		gain := firmware.RxGainBoosted
		if i%2 == 0 {
			gain = firmware.RxGainPowerSaving
		}
		e.ApplyRadioState(i%3, firmware.RadioStats{
			Configured: true, RxGainReg: gain, TxPowerDBm: int8(14 + i%8),
		})
	})
	// And the other one: an operator walking a node around the map.
	spin(func(i int) { e.SetNodePosition(2, 56.700, -3.880+float64(i%16)*0.001) })

	for step := 0; step < 60; step++ {
		if step%20 == 0 {
			e.InjectFrame(0, []byte("something for the readers to see"))
		}
		if err := e.Step(context.Background()); err != nil {
			close(stop)
			wg.Wait()
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

// A capture onto a named pipe owns a goroutine draining onto it. Close shuts
// every node down, and until it also closed the recorders that goroutine and
// its open file outlived the engine: a workbench opening one scenario after
// another leaked one of each per run.
func TestCloseStopsTheCaptureWriter(t *testing.T) {
	e := engine.New(flat{100}, engine.Config{StepMs: 10})
	e.Add(wfNode("a", 0, 22), nil)
	e.Add(wfNode("b", 0.010, 22), nil)

	before := runtime.NumGoroutine()
	fifo := filepath.Join(t.TempDir(), "live.pcapng")
	if err := e.StartCaptureFIFO(fifo); err != nil {
		t.Skipf("no named pipes here: %v", err)
	}
	if err := e.StartEventLog(filepath.Join(t.TempDir(), "events.ndjson")); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if p := e.CapturePath(); p != "" {
		t.Errorf("the capture is still open on %s after Close", p)
	}
	if p := e.EventLogPath(); p != "" {
		t.Errorf("the event log is still open on %s after Close", p)
	}
	// The writer exits when its channel closes, which Close causes but does
	// not wait for, so give the scheduler a moment before believing a count.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("%d goroutine(s) outlived the engine", after-before)
	}
}
