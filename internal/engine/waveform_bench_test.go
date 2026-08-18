package engine_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/engine"
)

// benchMesh is a dense grid of nodes with a burst of overlapping traffic -
// the flood shape that decides whether waveform mode is affordable.
func benchMesh(mode engine.RFMode, nodes, senders int) *engine.Engine {
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 7, RFMode: mode})
	side := 1
	for side*side < nodes {
		side++
	}
	for i := 0; i < nodes; i++ {
		row, col := i/side, i%side
		n := wfNode(fmt.Sprintf("n%03d", i), 0, 22)
		n.Position.Lat = 56.700 + float64(row)*0.004
		n.Position.Lon = -3.900 + float64(col)*0.004
		e.Add(n, nil)
	}
	_ = e.Run(context.Background(), 10)
	frame := make([]byte, 40)
	for i := range frame {
		frame[i] = byte(i * 7)
	}
	for s := 0; s < senders; s++ {
		e.InjectFrame(s*(nodes/senders), frame)
	}
	return e
}

// runBurst drives the mesh until the burst has delivered.
func runBurst(b *testing.B, e *engine.Engine, until uint32) {
	b.Helper()
	if err := e.Run(context.Background(), until); err != nil {
		b.Fatal(err)
	}
}

func benchModes(b *testing.B, nodes, senders int) {
	for _, mode := range []engine.RFMode{engine.RFCalculated, engine.RFWaveform} {
		mode := mode
		b.Run(string(mode), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				e := benchMesh(mode, nodes, senders)
				b.StartTimer()
				runBurst(b, e, 5000)
			}
		})
	}
}

// The W1 budget question: a country-fixture-sized mesh with a flood burst,
// both physics. The waveform numbers are the ceiling everything after W1
// builds under.
func BenchmarkBurst100Nodes10Senders(b *testing.B) { benchModes(b, 100, 10) }
func BenchmarkBurst300Nodes20Senders(b *testing.B) { benchModes(b, 300, 20) }
func BenchmarkBurst300Nodes5Senders(b *testing.B)  { benchModes(b, 300, 5) }
