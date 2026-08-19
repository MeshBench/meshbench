package engine_test

import (
	"context"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// The stutter-to-a-stop report: several transmitters up at once, waveform
// mode, and the simulation falls behind the wall. This is that scenario as
// a number - 40 nodes, 8 talkers injecting every 400 ms of simulated time,
// 5 s simulated - so the fixes have something honest to move.
func benchConcurrent(b *testing.B, mode engine.RFMode, talkers int) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 99, RFMode: mode})
		const nodes = 40
		for j := 0; j < nodes; j++ {
			e.Add(wfNode(string(rune('a'+j%26))+string(rune('0'+j/26)), float64(j)*0.004, 22), nil)
		}
		_ = e.Run(context.Background(), 10)
		b.StartTimer()
		for at := 0; at < 5000; at += 400 {
			for t := 0; t < talkers; t++ {
				e.InjectFrame(t*4, []byte("concurrent traffic saturating the air for the benchmark"))
			}
			_ = e.Run(context.Background(), 400)
		}
	}
}

func BenchmarkConcurrentWaveform8(b *testing.B)   { benchConcurrent(b, engine.RFWaveform, 8) }
func BenchmarkConcurrentWaveform3(b *testing.B)   { benchConcurrent(b, engine.RFWaveform, 3) }
func BenchmarkConcurrentCalculated8(b *testing.B) { benchConcurrent(b, engine.RFCalculated, 8) }
