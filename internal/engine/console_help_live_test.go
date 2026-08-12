package engine_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/A13xB0/meshcoresim/internal/antenna"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// Does `help` answer, and is the answer any use?
//
// A feature for a person rather than for a network, so the test is what a
// person would do: type the word and read what comes back. Stock firmware
// answers "Err - ??", which is what a repeater says when it understands
// nothing at all, so the first thing a newcomer tries reads as a broken node.
//
//	MESHCORESIM_LIVE=1 MESHCORESIM_NATIVE=~/msim/study/11-console-help \
//	go test ./internal/engine/ -run TestConsoleHelp -v -timeout 200s
func TestConsoleHelpAnswersAPerson(t *testing.T) {
	if os.Getenv("MESHCORESIM_LIVE") == "" {
		t.Skip("set MESHCORESIM_LIVE=1")
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
		{"help", []string{"region", "get", "advert"}},
		{"help region", []string{"allowf", "#sco"}},
		{"help radio", []string{"rxdelay"}},
	} {
		got := ask(c.typed)
		t.Logf("%q ->\n%s", c.typed, strings.TrimSpace(got))
		// The reply itself quotes the error string, so look for it as the whole
		// answer rather than anywhere in it. The first version of this check
		// failed on its own help text.
		if strings.Contains(got, "-> Err - ??") {
			t.Errorf("%q was not understood", c.typed)
			continue
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
