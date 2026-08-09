package firmware_test

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/A13xB0/meshcoresim/internal/dsp"
	"github.com/A13xB0/meshcoresim/internal/firmware"
)

// The native node is a compiled artefact from another repository, so its
// absence is a skip and not a failure — but only when it is genuinely absent.
// A binary that exists and cannot run is a real failure and must not be
// skipped past.
func nativeNode(t *testing.T, seed uint64) (*firmware.Node, *bytes.Buffer) {
	t.Helper()
	if _, err := firmware.FindNative(""); err != nil {
		if errors.Is(err, firmware.ErrNativeMissing) {
			t.Skipf("no native node binary: %v (build with tools/native/build.sh)", err)
		}
		t.Fatal(err)
	}
	log := &bytes.Buffer{}
	n, err := firmware.Start(context.Background(), "native-1", &firmware.Native{Seed: seed, Log: log})
	if err != nil {
		t.Fatalf("start native node: %v", err)
	}
	t.Cleanup(func() {
		_ = n.Close()
		if t.Failed() && log.Len() > 0 {
			t.Logf("node stderr:\n%s", log)
		}
	})
	waitAttached(t, n)
	return n, log
}

func waitAttached(t *testing.T, n *firmware.Node) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !n.Bridge.Attached() {
		if time.Now().After(deadline) {
			t.Fatal("native node never connected to the bridge")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNativeNodeRunsInLockstep(t *testing.T) {
	n, _ := nativeNode(t, 4417)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ticking to a time the node has already passed would be a protocol error;
	// ticking forward repeatedly is the normal case and must be exact.
	for at := uint32(10); at <= 200; at += 10 {
		if err := n.Bridge.Advance(ctx, at); err != nil {
			t.Fatalf("advance to %d ms: %v", at, err)
		}
	}
}

// The whole reason native exists: it must run faster than the wall clock it is
// simulating. This asserts a very loose ratio — the point is to catch a node
// that has silently started sleeping, not to benchmark one.
func TestNativeNodeBeatsRealTime(t *testing.T) {
	n, _ := nativeNode(t, 4417)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const simMs = 10_000
	start := time.Now()
	for at := uint32(100); at <= simMs; at += 100 {
		if err := n.Bridge.Advance(ctx, at); err != nil {
			t.Fatalf("advance to %d ms: %v", at, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > simMs*time.Millisecond {
		t.Fatalf("%d ms of simulated time took %v — slower than real time", simMs, elapsed)
	}
	t.Logf("%d ms simulated in %v (%.0f× real time)", simMs, elapsed,
		float64(simMs*time.Millisecond)/float64(elapsed))
}

// A frame handed to the node must reach the firmware, and the firmware's own
// transmissions must come back out — otherwise the bridge is a socket that
// happens to stay open.
func TestNativeNodeDeliversAndTransmits(t *testing.T) {
	n, log := nativeNode(t, 4417)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A MeshCore advert-shaped frame. It does not have to be routable; it has
	// to be something the firmware reads out of the radio and acts on.
	frame := []byte{0x12, 0x00, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7}
	if err := n.Bridge.Deliver(frame); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	for at := uint32(10); at <= 2000; at += 10 {
		if err := n.Bridge.Advance(ctx, at); err != nil {
			t.Fatalf("advance: %v", err)
		}
	}
	_ = n.Close()

	// The node reports its own counters on the way out, which is the only
	// evidence available that the frame got past the socket and into Mesh.
	if !bytes.Contains(log.Bytes(), []byte("MeshCore up")) {
		t.Fatalf("node never reported MeshCore starting; stderr:\n%s", log)
	}
	if !bytes.Contains(log.Bytes(), []byte("bridge closed")) {
		t.Fatalf("node did not shut down cleanly; stderr:\n%s", log)
	}
}

func TestFindNativeReportsWhereItLooked(t *testing.T) {
	t.Setenv(firmware.EnvNativeBinary, "")
	// PATH emptied so the lookup cannot succeed by accident on a machine that
	// happens to have a node installed.
	t.Setenv("PATH", t.TempDir())
	_, err := firmware.FindNative("")
	if !errors.Is(err, firmware.ErrNativeMissing) {
		t.Fatalf("want ErrNativeMissing, got %v", err)
	}
	for _, want := range []string{firmware.NativeBinaryName(), firmware.EnvNativeBinary} {
		if !bytes.Contains([]byte(err.Error()), []byte(want)) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

func TestNativeStopIsSafeWithoutStart(t *testing.T) {
	if err := (&firmware.Native{}).Stop(); err != nil {
		t.Fatal(err)
	}
	if err := (&firmware.Emulated{}).Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestEmulatedNeedsFirmware(t *testing.T) {
	if err := (&firmware.Emulated{}).Start(context.Background(), "127.0.0.1:1"); err == nil {
		t.Fatal("an emulated node with no image should not start")
	}
}

// The airtime formula exists twice — once in Go for the channel, once in C++
// inside the node — because the firmware needs it at runtime and the engine
// needs it to occupy the air for the right length of time. Two copies that
// nothing compares are two formulas, and CLAUDE.md's rule is that they must
// agree: the firmware's CSMA timing is built on its own number, so a drift
// desynchronises the mesh from the channel silently.
func TestNativeAirtimeAgreesWithTheChannel(t *testing.T) {
	bin, err := firmware.FindNative("")
	if err != nil {
		if errors.Is(err, firmware.ErrNativeMissing) {
			t.Skipf("no native node binary: %v", err)
		}
		t.Fatal(err)
	}
	for _, sf := range []int{7, 8, 9, 10, 11, 12} {
		for _, bwKHz := range []float64{125, 250} {
			for _, n := range []int{1, 16, 64, 200} {
				want := dsp.AirtimeMillis(sf, bwKHz*1000, 1, n, true, true)
				out, err := exec.Command(bin,
					"--print-airtime", strconv.Itoa(n),
					"--sf", strconv.Itoa(sf),
					"--bw-khz", strconv.FormatFloat(bwKHz, 'f', -1, 64),
					"--cr", "1").Output()
				if err != nil {
					t.Fatalf("SF%d BW%.0f n=%d: %v", sf, bwKHz, n, err)
				}
				got, err := strconv.Atoi(strings.TrimSpace(string(out)))
				if err != nil {
					t.Fatalf("unparseable airtime %q: %v", out, err)
				}
				// One millisecond, because the firmware truncates in float and
				// the channel truncates in float64.
				if math.Abs(float64(got)-want) > 1 {
					t.Errorf("SF%d BW%.0fkHz %d bytes: node says %d ms, channel says %.0f ms",
						sf, bwKHz, n, got, want)
				}
			}
		}
	}
}
