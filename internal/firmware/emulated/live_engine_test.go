package emulated_test

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// Stand in for the engine: accept the node, tick it, and see whether anything
// comes back. If a frame arrives, the emulated node is on the channel.
func TestLiveEmulatedNodeJoinsTheEngine(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	bc := &emulated.BoardCatalogue{CacheDir: dir}
	all, err := bc.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var want emulated.BoardImage
	for _, img := range emulated.Runnable(all, func(b string) bool { return b == "Generic_E22_sx1262" }) {
		if img.Version == "v1.17.0" && img.Role == "simple_repeater" {
			want = img
		}
	}
	path, err := bc.Ensure(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	padded := filepath.Join(dir, "p.bin")
	if _, err := emulated.PadImage(path, padded); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	n := &emulated.EmulatedNode{
		Image: padded, NodeName: "eng", Dir: filepath.Join(dir, "node"),
		Machine: "esp32", SPI: 2, NSS: 18, Busy: 32,
	}
	if err := n.Start(ctx, ln.Addr().String()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = n.Stop() }()

	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("the node joined the engine")

	frames, acks := 0, 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		hdr := make([]byte, 3)
		for {
			if _, err := io.ReadFull(conn, hdr); err != nil {
				return
			}
			n := int(hdr[1])<<8 | int(hdr[2])
			buf := make([]byte, n)
			if n > 0 {
				if _, err := io.ReadFull(conn, buf); err != nil {
					return
				}
			}
			switch hdr[0] {
			case 0x01:
				frames++
				t.Logf("frame from the node: %d bytes", len(buf))
			case 0x03:
				acks++
			}
		}
	}()

	// Paced to wall time, unlike a native node.
	//
	// The engine can run a native node far faster than real time because it
	// owns its execution. An emulator does not work that way: the firmware runs
	// on wall time, so racing the clock ahead only means the chip believes it is
	// a minute later than the code that is still booting.
	for ms := uint32(100); ms <= 45_000; ms += 100 {
		var p [4]byte
		binary.BigEndian.PutUint32(p[:], ms)
		if _, err := conn.Write(append([]byte{0x02, 0, 4}, p[:]...)); err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = conn.Close()
	<-done

	t.Logf("acks %d, frames from the node %d", acks, frames)
	if acks == 0 {
		t.Error("the node never acknowledged a tick")
	}
	if frames == 0 {
		t.Error("the node never put a frame on the channel")
	}
}
