package native_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/firmware/native"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// The native node is a compiled artefact from another repository, so its
// absence is a skip and not a failure — but only when it is genuinely absent.
// A binary that exists and cannot run is a real failure and must not be
// skipped past.
func nativeNode(t *testing.T, seed uint64) (*firmware.Node, *bytes.Buffer) {
	t.Helper()
	if _, err := firmware.FindNative("", "simple_repeater"); err != nil {
		if errors.Is(err, firmware.ErrNativeMissing) {
			t.Skipf("no native node binary: %v (build with tools/native/build.sh)", err)
		}
		t.Fatal(err)
	}
	log := &bytes.Buffer{}
	// A working directory of its own. Without one the node inherits the test's,
	// which is the package's own source directory, and the repeater persists its
	// identity to "flash" on first boot - so running these left a _main.id in
	// the tree, and every one of them shared the identity the first had written.
	n, err := firmware.Start(context.Background(), "native-1",
		&native.Native{Seed: seed, Log: log, WorkDir: t.TempDir()})
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

	saidItReached(t, log, 2000)
}

// saidItReached checks the node's own account of its run: that the firmware
// came up, and that it left on a closed socket having reached the last tick it
// was given.
//
// The simulated time in the closing line is the evidence that matters. A node
// that connected and then stopped processing ticks closes just the same, and
// the number is the only thing that tells the two apart.
//
// These are the words the published builds print, checked against
// repeater-v1.17.1 and main. The pair this used to look for - "MeshCore up"
// and "bridge closed" - are not among them and had not been for some time,
// which nothing noticed because no pipeline ever ran a native node.
func saidItReached(t *testing.T, log *bytes.Buffer, atMs uint32) {
	t.Helper()
	if !bytes.Contains(log.Bytes(), []byte("radio_init")) {
		t.Fatalf("node never reported its radio coming up; stderr:\n%s", log)
	}
	want := fmt.Sprintf("bridge: closed after %d ms", atMs)
	if !bytes.Contains(log.Bytes(), []byte(want)) {
		t.Fatalf("node did not report closing at %d ms; stderr:\n%s", atMs, log)
	}
}

func TestFindNativeReportsWhereItLooked(t *testing.T) {
	t.Setenv(firmware.EnvNativeBinary, "")
	// PATH emptied so the lookup cannot succeed by accident on a machine that
	// happens to have a node installed.
	t.Setenv("PATH", t.TempDir())
	_, err := firmware.FindNative("", "simple_repeater")
	if !errors.Is(err, firmware.ErrNativeMissing) {
		t.Fatalf("want firmware.ErrNativeMissing, got %v", err)
	}
	for _, want := range []string{firmware.NativeBinaryName("simple_repeater"), firmware.EnvNativeBinary} {
		if !bytes.Contains([]byte(err.Error()), []byte(want)) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

func TestNativeStopIsSafeWithoutStart(t *testing.T) {
	if err := (&native.Native{}).Stop(); err != nil {
		t.Fatal(err)
	}
}

// The airtime formula exists twice — once in Go for the channel, once in C++
// inside the node — because the firmware needs it at runtime and the engine
// needs it to occupy the air for the right length of time. Two copies that
// nothing compares are two formulas, and CLAUDE.md's rule is that they must
// agree: the firmware's CSMA timing is built on its own number, so a drift
// desynchronises the mesh from the channel silently.
//
// Asked at the settings the build was compiled with, because those are the only
// ones it will answer about. See the sweep below.
func TestNativeAirtimeAgreesWithTheChannel(t *testing.T) {
	bin := nativeBinary(t)
	for _, n := range []int{1, 16, 40, 64, 200} {
		want := dsp.AirtimeMillis(builtInSF, builtInBWkHz*1000, builtInCR, n, true, true)
		got := nodeAirtime(t, bin, n, 0, 0, 0)
		// One millisecond, because the firmware truncates in float and the
		// channel truncates in float64.
		if math.Abs(float64(got)-want) > 1 {
			t.Errorf("%d bytes: node says %d ms, channel says %.0f ms", n, got, want)
		}
	}
}

// The same agreement across the modem settings a scenario can actually choose,
// which is where a drift would do the most damage: the same bytes at SF12/62.5
// occupy the air some forty times longer than at SF7/250.
//
// Skipped, loudly, on a build whose --print-airtime ignores --sf, --bw-khz and
// --cr. Both published repeater builds do - repeater-v1.17.1 and main answer
// the same number for SF7 and SF12 - so this can only be asked of the compiled
// default until MeshBench/meshcore-native applies those flags before printing.
// Skipped rather than dropped: the day it does, this starts checking again
// without anyone remembering to write it.
func TestNativeAirtimeAgreesAcrossTheModemSettings(t *testing.T) {
	bin := nativeBinary(t)
	if slow, fast := nodeAirtime(t, bin, 64, 12, 125, 1), nodeAirtime(t, bin, 64, 7, 125, 1); slow == fast {
		t.Skipf("this build answers %d ms for SF12 and for SF7, so it is not applying "+
			"--sf/--bw-khz/--cr to --print-airtime and only its compiled default can be "+
			"compared; that is MeshBench/meshcore-native's to fix", slow)
	}
	for _, sf := range []int{7, 8, 9, 10, 11, 12} {
		for _, bwKHz := range []float64{125, 250} {
			for _, n := range []int{1, 16, 64, 200} {
				want := dsp.AirtimeMillis(sf, bwKHz*1000, 1, n, true, true)
				got := nodeAirtime(t, bin, n, sf, bwKHz, 1)
				if math.Abs(float64(got)-want) > 1 {
					t.Errorf("SF%d BW%.0fkHz %d bytes: node says %d ms, channel says %.0f ms",
						sf, bwKHz, n, got, want)
				}
			}
		}
	}
}

// What the published repeater build is compiled for, and therefore what
// --print-airtime is about when it is given nothing else: 300 ms for a
// 40-byte packet, which is the figure the catalogue's own live check uses to
// tell a good download from a truncated one.
const (
	builtInSF    = 10
	builtInBWkHz = 250.0
	builtInCR    = 1
)

func nativeBinary(t *testing.T) string {
	t.Helper()
	bin, err := firmware.FindNative("", "simple_repeater")
	if err != nil {
		if errors.Is(err, firmware.ErrNativeMissing) {
			t.Skipf("no native node binary: %v", err)
		}
		t.Fatal(err)
	}
	return bin
}

// nodeAirtime asks the firmware for its own estimate. A zero spreading factor
// means ask about whatever the build was compiled with.
func nodeAirtime(t *testing.T, bin string, n, sf int, bwKHz float64, cr int) int {
	t.Helper()
	args := []string{"--print-airtime", strconv.Itoa(n)}
	if sf != 0 {
		args = append(args, "--sf", strconv.Itoa(sf),
			"--bw-khz", strconv.FormatFloat(bwKHz, 'f', -1, 64),
			"--cr", strconv.Itoa(cr))
	}
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		t.Fatalf("asking the node for the airtime of %d bytes: %v", n, err)
	}
	ms, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("unparseable airtime %q: %v", out, err)
	}
	return ms
}

// The node must not decide for itself when its transmission ended. It waits to
// be told, because how long the signal occupied the channel is a property of
// the samples the engine generated — not of any formula the node could apply.
func TestNodeWaitsForTheEngineToEndTransmission(t *testing.T) {
	n, log := nativeNode(t, 4417)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Long enough that any airtime estimate the node might have used would have
	// expired several times over. If the node were timing itself, it would have
	// declared the send complete and moved on.
	for at := uint32(10); at <= 5000; at += 10 {
		if err := n.Bridge.Advance(ctx, at); err != nil {
			t.Fatalf("advance: %v", err)
		}
	}
	if err := n.Bridge.TransmitFinished(); err != nil {
		t.Fatalf("signal end of transmission: %v", err)
	}
	if err := n.Bridge.Advance(ctx, 5010); err != nil {
		t.Fatalf("advance after tx done: %v", err)
	}
	_ = n.Close()
	saidItReached(t, log, 5010)
}
