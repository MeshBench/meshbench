package gpu

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/cogentcore/webgpu/wgpu"
)

// The collector is held off across a buffer map, and this is why.
//
// MapAsync registers a cgo handle for the callback, hands wgpu-native the
// address of the local variable holding that handle's index, and returns. The
// callback reads the address back much later, from inside Poll. Nothing in Go
// references those eight bytes in between, so a collection landing in that
// window frees them and the allocator gives them to somebody else; what the
// callback then reads is not a handle, and cgo panics on it. That is a crash
// in the self-check whose entire job is to stop a wrong answer reaching a map,
// and it fails whichever package happens to be running.
//
// The fault is upstream and cannot be repaired from here: the module is
// unmaintained, its tip carries no fix, and it ships four hundred megabytes of
// prebuilt libraries, so there is no fork worth keeping. What can be done is to
// keep the collector out of the window. The window is one buffer map wide and
// every readback in this package is bounded by it.
//
// Counted rather than set and restored, because two readbacks can overlap: a
// coverage fold reads three buffers and a probe can be running beside it. The
// collector goes off when the first enters and back to what it was when the
// last leaves, so an overlap cannot re-enable it under a reader still inside.
var (
	collectorMu    sync.Mutex
	collectorHolds int
	collectorSaved int
)

func holdCollector() {
	collectorMu.Lock()
	defer collectorMu.Unlock()
	if collectorHolds == 0 {
		collectorSaved = debug.SetGCPercent(-1)
	}
	collectorHolds++
}

func releaseCollector() {
	collectorMu.Lock()
	defer collectorMu.Unlock()
	collectorHolds--
	if collectorHolds == 0 {
		debug.SetGCPercent(collectorSaved)
	}
}

// readBuffer copies a device buffer into host memory via a staging buffer.
//
// One copy of the map-and-poll dance rather than one per kernel, because this
// is where a readback goes quietly wrong. A map that does not succeed leaves
// the staging buffer holding whatever was already in that memory, and every
// kernel here returns decibels or symbol bins: the caller cannot tell a failed
// map from a real answer, so it draws the map and the numbers look plausible.
// Four of the five call sites this replaces discarded the status entirely.
//
// The status is therefore checked rather than dropped, and the poll runs until
// the callback has actually landed rather than trusting one blocking call, on
// the same reasoning: a callback that needs a second poll is the kind of thing
// that only shows up on a driver nobody tested against.
func (d *Device) readBuffer(src *wgpu.Buffer, n int) ([]byte, error) {
	staging, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging", Size: uint64(n),
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, err
	}
	defer staging.Release()

	enc, err := d.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, err
	}
	if err := enc.CopyBufferToBuffer(src, 0, staging, 0, uint64(n)); err != nil {
		return nil, fmt.Errorf("gpu: copy result to staging: %w", err)
	}
	cmd, err := enc.Finish(nil)
	if err != nil {
		return nil, err
	}
	d.queue.Submit(cmd)
	cmd.Release()
	enc.Release()

	status := wgpu.BufferMapAsyncStatusUnknown
	done := false
	// From here to the callback landing, wgpu-native holds a pointer into the
	// Go heap that Go itself no longer knows about.
	holdCollector()
	if err := staging.MapAsync(wgpu.MapModeRead, 0, uint64(n),
		func(s wgpu.BufferMapAsyncStatus) { status, done = s, true }); err != nil {
		releaseCollector()
		return nil, fmt.Errorf("gpu: map staging buffer: %w", err)
	}
	for !done {
		d.device.Poll(true, nil)
	}
	releaseCollector()
	if status != wgpu.BufferMapAsyncStatusSuccess {
		return nil, fmt.Errorf("gpu: staging buffer never mapped: %v", status)
	}
	raw := staging.GetMappedRange(0, uint(n))
	out := make([]byte, n)
	copy(out, raw)
	_ = staging.Unmap() // the data is already copied out
	return out, nil
}
