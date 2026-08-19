// The on-device best-server fold. Stations are dispatched one after
// another against persistent best/second/served buffers, so a whole
// raster costs one readback however many stations price it - the
// per-station readback was most of the whole-map job's wall clock.
package gpu

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/cogentcore/webgpu/wgpu"
)

// compileCoverageFold builds the fold pipeline from the coverage module's
// second entry point, called from Open alongside compileCoverage.
func (d *Device) compileCoverageFold() error {
	mod, err := d.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label:          "coverage-fold",
		WGSLDescriptor: &wgpu.ShaderModuleWGSLDescriptor{Code: coverageWGSL},
	})
	if err != nil {
		return fmt.Errorf("gpu: coverage fold shader: %w", err)
	}
	defer mod.Release()
	d.coverageFold, err = d.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:   "coverage-fold",
		Compute: wgpu.ProgrammableStageDescriptor{Module: mod, EntryPoint: "fold"},
	})
	if err != nil {
		return fmt.Errorf("gpu: coverage fold pipeline: %w", err)
	}
	return nil
}

// CoverageFold is the persistent per-cell state for one raster (or one
// band of one): best and second-best slots plus the serving count.
type CoverageFold struct {
	cg    *CoverageGrid
	cells int
	best  *wgpu.Buffer
	sec   *wgpu.Buffer
	srv   *wgpu.Buffer
}

// foldSlotBytes mirrors the kernel's Slot: min, out, in, station.
const foldSlotBytes = 16

// NewFold allocates and empties the fold state for w x h cells.
func (cg *CoverageGrid) NewFold(w, h int) (*CoverageFold, error) {
	cells := w * h
	empty := make([]uint32, cells*4)
	minBits := math.Float32bits(-math.MaxFloat32)
	for i := 0; i < cells; i++ {
		empty[i*4+0] = minBits
		empty[i*4+1] = minBits
		empty[i*4+2] = minBits
		empty[i*4+3] = 0xffffffff
	}
	f := &CoverageFold{cg: cg, cells: cells}
	var err error
	mk := func(label string, contents []uint32) *wgpu.Buffer {
		if err != nil {
			return nil
		}
		var b *wgpu.Buffer
		b, err = cg.d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
			Label: label, Contents: wgpu.ToBytes(contents),
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
		})
		return b
	}
	f.best = mk("fold-best", empty)
	f.sec = mk("fold-second", empty)
	f.srv = mk("fold-served", make([]uint32, cells))
	if err != nil {
		f.Release()
		return nil, err
	}
	return f, nil
}

// Release frees the fold state.
func (f *CoverageFold) Release() {
	for _, b := range []*wgpu.Buffer{f.best, f.sec, f.srv} {
		if b != nil {
			b.Release()
		}
	}
}

// Station folds one station in: a dispatch and nothing back.
func (f *CoverageFold) Station(p propagation.GridLossParams, b propagation.StationBudget,
	gt propagation.GainTable) error {
	d, cg := f.cg.d, f.cg
	if p.RasterW*p.RasterH != f.cells {
		return fmt.Errorf("gpu: fold sized for %d cells, station priced over %d",
			f.cells, p.RasterW*p.RasterH)
	}
	fb := math.Float32bits
	params := []uint32{
		uint32(cg.g.W), uint32(cg.g.H), uint32(p.RasterW), uint32(p.RasterH),
		fb(float32(cg.g.South)), fb(float32(cg.g.North)), fb(float32(cg.g.West)), fb(float32(cg.g.East)),
		fb(float32(p.South)), fb(float32(p.North)), fb(float32(p.West)), fb(float32(p.East)),
		fb(float32(p.StLat)), fb(float32(p.StLon)), fb(float32(p.StAltM)), fb(float32(p.RemoteHeightM)),
		fb(float32(p.FreqMHz)), uint32(p.Steps), 0, 0,
	}
	budget := []uint32{
		fb(float32(b.TxPowerDBm)), fb(float32(b.SensitivityDBm)),
		fb(float32(b.RemoteTxDBm)), fb(float32(b.RemoteGainDBi)),
		fb(float32(b.RemoteSensitivityDBm)), uint32(b.Station),
		uint32(gt.AzN), uint32(gt.ElN),
		fb(float32(gt.ElMinDeg)), fb(float32(gt.ElStepDeg)), 0, 0,
	}
	pb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "fold-params", Contents: wgpu.ToBytes(params), Usage: wgpu.BufferUsageUniform,
	})
	if err != nil {
		return err
	}
	defer pb.Release()
	bb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "fold-budget", Contents: wgpu.ToBytes(budget), Usage: wgpu.BufferUsageUniform,
	})
	if err != nil {
		return err
	}
	defer bb.Release()
	gb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "fold-gains", Contents: wgpu.ToBytes(gt.DB), Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		return err
	}
	defer gb.Release()

	bg, err := d.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: d.coverageFold.GetBindGroupLayout(0),
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: cg.buf, Size: uint64(len(cg.g.Heights) * 4)},
			{Binding: 2, Buffer: pb, Size: uint64(len(params) * 4)},
			{Binding: 3, Buffer: bb, Size: uint64(len(budget) * 4)},
			{Binding: 4, Buffer: gb, Size: uint64(len(gt.DB) * 4)},
			{Binding: 5, Buffer: f.best, Size: uint64(f.cells * foldSlotBytes)},
			{Binding: 6, Buffer: f.sec, Size: uint64(f.cells * foldSlotBytes)},
			{Binding: 7, Buffer: f.srv, Size: uint64(f.cells * 4)},
		},
	})
	if err != nil {
		return err
	}
	defer bg.Release()

	enc, err := d.device.CreateCommandEncoder(nil)
	if err != nil {
		return err
	}
	pass := enc.BeginComputePass(nil)
	pass.SetPipeline(d.coverageFold)
	pass.SetBindGroup(0, bg, nil)
	pass.DispatchWorkgroups((uint32(f.cells)+63)/64, 1, 1)
	_ = pass.End()
	pass.Release()
	cmd, err := enc.Finish(nil)
	if err != nil {
		return err
	}
	d.queue.Submit(cmd)
	cmd.Release()
	enc.Release()
	return nil
}

// Read brings the fold home: best and second slots plus serving counts.
func (f *CoverageFold) Read() (best, second []propagation.FoldSlot, served []uint32, err error) {
	bestRaw, err := f.cg.d.readBuffer(f.best, f.cells*foldSlotBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	secRaw, err := f.cg.d.readBuffer(f.sec, f.cells*foldSlotBytes)
	if err != nil {
		return nil, nil, nil, err
	}
	srvRaw, err := f.cg.d.readBuffer(f.srv, f.cells*4)
	if err != nil {
		return nil, nil, nil, err
	}
	best = make([]propagation.FoldSlot, f.cells)
	second = make([]propagation.FoldSlot, f.cells)
	served = make([]uint32, f.cells)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&best[0])), f.cells*foldSlotBytes), bestRaw)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&second[0])), f.cells*foldSlotBytes), secRaw)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&served[0])), f.cells*4), srvRaw)
	return best, second, served, nil
}

// readBuffer copies a device buffer into host memory via a staging buffer.
func (d *Device) readBuffer(src *wgpu.Buffer, n int) ([]byte, error) {
	staging, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "fold-staging", Size: uint64(n),
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
		return nil, err
	}
	cmd, err := enc.Finish(nil)
	if err != nil {
		return nil, err
	}
	d.queue.Submit(cmd)
	cmd.Release()
	enc.Release()
	done := false
	if err := staging.MapAsync(wgpu.MapModeRead, 0, uint64(n),
		func(wgpu.BufferMapAsyncStatus) { done = true }); err != nil {
		return nil, err
	}
	for !done {
		d.device.Poll(true, nil)
	}
	raw := staging.GetMappedRange(0, uint(n))
	out := make([]byte, n)
	copy(out, raw)
	_ = staging.Unmap()
	return out, nil
}
