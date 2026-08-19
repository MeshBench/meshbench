package engine_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// outcomeSet runs one traffic pattern under a mode and returns every
// from>to:kind pair - the shape of the run, without timing noise.
func outcomeSet(t *testing.T, mode engine.RFMode) map[string]string {
	t.Helper()
	e := benchMeshT(t, mode, 60, 6)
	if err := e.Run(context.Background(), 8000); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, ev := range e.Events() {
		key := ev.From + ">" + ev.To
		// Last event per pair wins; the pairs are one packet each here.
		out[key] = ev.Kind
	}
	return out
}

func benchMeshT(t *testing.T, mode engine.RFMode, nodes, senders int) *engine.Engine {
	t.Helper()
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

// TestModeDivergence is the measurement tool the plan asks for, kept as a
// test: run one scenario under both physics and report where they disagree.
// It never fails on divergence - divergence is the finding, not a fault -
// only on the modes producing no traffic at all.
//
//	go test ./internal/engine/ -run TestModeDivergence -v
func TestModeDivergence(t *testing.T) {
	calc := outcomeSet(t, engine.RFCalculated)
	wave := outcomeSet(t, engine.RFWaveform)

	if len(calc) == 0 || len(wave) == 0 {
		t.Fatalf("a mode produced no events: calculated=%d waveform=%d", len(calc), len(wave))
	}

	agree, calcOnly, waveOnly, differ := 0, 0, 0, 0
	for k, cv := range calc {
		wv, ok := wave[k]
		switch {
		case !ok:
			calcOnly++
		case cv == wv:
			agree++
		default:
			differ++
			if differ <= 12 {
				t.Logf("differs %-14s calculated=%-5s waveform=%s", k, cv, wv)
			}
		}
	}
	for k := range wave {
		if _, ok := calc[k]; !ok {
			waveOnly++
		}
	}
	t.Logf("divergence: %d agree, %d differ, %d calculated-only, %d waveform-only",
		agree, differ, calcOnly, waveOnly)
}
