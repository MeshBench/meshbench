package engine_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// Does the console answer a person, in words they can use?
//
// A feature for a person rather than for a network, so the test is what a
// person would do: type a command and read what comes back. It asks the
// things someone actually types at a repeater they have just put on a mast -
// what firmware is this, what is it tuned to, what is it called, send an
// advert - and insists each answer carries the fact that was asked for.
//
// It deliberately does not ask for `help`. Published MeshCore repeater builds
// have no such command and answer "Unknown command", so a newcomer's first
// guess reads as a broken node. That is a real finding about the firmware and
// it is recorded in the console study, but asserting it here would only pin a
// gap this project cannot close: the scheduled live job runs against published
// builds, and a test that can never pass is a job everybody learns to ignore.
//
//	MESHBENCH_LIVE=1 go test ./internal/sim/engine/ -run TestConsole -v -timeout 200s
func TestConsoleAnswersAPerson(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120e9)
	defer cancel()

	e := engine.New(flat{}, engine.Config{
		FreqMHz: 869.618, SF: 8, BandwidthHz: 62_500, CodingRate: 4,
		NoiseFigDB: 6, StepMs: 10, Seed: 7,
	})
	defer func() { _ = e.Close() }()

	e.Add(scenario.Node{
		Name: "help-rptr", Kind: scenario.SimpleRepeater,
		Position: scenario.LatLon{Lat: 56.70, Lon: -3.90}, HeightAGLm: 10,
		Antenna: antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6},
			Polarisation: "vertical"},
		TxPowerDBm: 20, NoiseFigureDB: 6,
		Radio: scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500,
			SpreadFactor: 8, CodingRate: 4},
		Firmware: scenario.FirmwareRef{Version: "repeater-v1.17.0"},
	}, nil)

	if err := e.AttachNative(ctx, 7); err != nil {
		t.Fatal(err)
	}
	node, ok := e.NodeByName("help-rptr")
	if !ok || node.Firmware == nil {
		t.Fatal("no firmware")
	}

	// The console is a serial port with one reader, so the sink is set once and
	// drained between questions.
	sink := &lockedBuffer{}
	node.Firmware.Bridge.Console(sink)

	ask := func(line string) string {
		t.Helper()
		sink.Reset()
		if err := node.Firmware.Bridge.Type([]byte(line + "\r\n")); err != nil {
			t.Fatal(err)
		}
		// Give the firmware time to produce a reply: it is a program reading a
		// UART, not a function call.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if err := e.Run(ctx, e.NowMs()+200); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(sink.String(), "->") {
				break
			}
		}
		return sink.String()
	}

	for _, c := range []struct {
		typed string
		want  []string
	}{
		// The build identifies itself, which is the first thing anyone asks of
		// a node they did not flash themselves.
		{"ver", []string{"v1.17.0"}},
		// The tuning comes back as freq,bw,sf,cr - all four, because a repeater
		// on the wrong one of them is silent in a way nothing else explains.
		{"get radio", []string{"869.6", "62.5", "8"}},
		{"get name", []string{"repeater"}},
		// Not a question but the one action a person takes from this prompt,
		// and it has to say it did something.
		{"advert", []string{"OK"}},
	} {
		got := ask(c.typed)
		t.Logf("%q ->\n%s", c.typed, strings.TrimSpace(got))
		// The two ways this firmware says it understood nothing. Checked as the
		// whole answer rather than anywhere in it, since a reply quotes back
		// what was typed.
		for _, bad := range []string{"-> Err - ??", "-> Unknown command"} {
			if strings.Contains(got, bad) {
				t.Errorf("%q was not understood, and it is a command this build has", c.typed)
			}
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%q did not mention %q, which is most of why someone types it",
					c.typed, w)
			}
		}
	}
}

// lockedBuffer is a console sink safe to read while the node writes to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *lockedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}
