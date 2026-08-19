package gpu

import (
	_ "embed"
	"fmt"
	"math"
	"unsafe"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
	"github.com/cogentcore/webgpu/wgpu"
)

//go:embed coverage.wgsl
var coverageWGSL string

// compileCoverage builds the coverage pipeline, called from Open.
func (d *Device) compileCoverage() error {
	mod, err := d.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label:          "coverage",
		WGSLDescriptor: &wgpu.ShaderModuleWGSLDescriptor{Code: coverageWGSL},
	})
	if err != nil {
		return fmt.Errorf("gpu: coverage shader: %w", err)
	}
	defer mod.Release()
	d.coverage, err = d.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:   "coverage",
		Compute: wgpu.ProgrammableStageDescriptor{Module: mod, EntryPoint: "main"},
	})
	if err != nil {
		return fmt.Errorf("gpu: coverage pipeline: %w", err)
	}
	return nil
}

// CoverageGridLoss runs the coverage kernel: one station's path loss to every
// raster cell, over a rasterised height grid. The CPU twin is
// propagation.GridLossCPU, and the equivalence test holds the two together.
// CoverageGrid is a height grid resident on the device, uploaded once and
// shared by every station's dispatch - re-uploading fifteen megabytes per
// station was most of a national job's transfer.
type CoverageGrid struct {
	d   *Device
	g   propagation.HeightGrid
	buf *wgpu.Buffer
}

// UploadGrid puts the heights on the device.
func (d *Device) UploadGrid(g propagation.HeightGrid) (*CoverageGrid, error) {
	if d.coverage == nil {
		return nil, fmt.Errorf("gpu: coverage pipeline not compiled")
	}
	hb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "heights", Contents: wgpu.ToBytes(g.Heights), Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		return nil, err
	}
	return &CoverageGrid{d: d, g: g, buf: hb}, nil
}

// Release frees the device copy.
func (cg *CoverageGrid) Release() { cg.buf.Release() }

func (d *Device) CoverageGridLoss(g propagation.HeightGrid, p propagation.GridLossParams) ([]float32, error) {
	cg, err := d.UploadGrid(g)
	if err != nil {
		return nil, err
	}
	defer cg.Release()
	return cg.Loss(p)
}

// Loss prices one station over the resident grid.
func (cg *CoverageGrid) Loss(p propagation.GridLossParams) ([]float32, error) {
	d, g, hb := cg.d, cg.g, cg.buf
	cells := p.RasterW * p.RasterH
	outLen := uint64(cells * 4)

	out, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "loss", Size: outLen,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, err
	}
	defer out.Release()

	f := math.Float32bits
	params := []uint32{
		uint32(g.W), uint32(g.H), uint32(p.RasterW), uint32(p.RasterH),
		f(float32(g.South)), f(float32(g.North)), f(float32(g.West)), f(float32(g.East)),
		f(float32(p.South)), f(float32(p.North)), f(float32(p.West)), f(float32(p.East)),
		f(float32(p.StLat)), f(float32(p.StLon)), f(float32(p.StAltM)), f(float32(p.RemoteHeightM)),
		f(float32(p.FreqMHz)), uint32(p.Steps), 0, 0,
	}
	pb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "params", Contents: wgpu.ToBytes(params), Usage: wgpu.BufferUsageUniform,
	})
	if err != nil {
		return nil, err
	}
	defer pb.Release()

	staging, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging", Size: outLen,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, err
	}
	defer staging.Release()

	bg, err := d.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: d.coverage.GetBindGroupLayout(0),
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: hb, Size: uint64(len(g.Heights) * 4)},
			{Binding: 1, Buffer: out, Size: outLen},
			{Binding: 2, Buffer: pb, Size: uint64(len(params) * 4)},
		},
	})
	if err != nil {
		return nil, err
	}
	defer bg.Release()

	enc, err := d.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, err
	}
	pass := enc.BeginComputePass(nil)
	pass.SetPipeline(d.coverage)
	pass.SetBindGroup(0, bg, nil)
	pass.DispatchWorkgroups((uint32(cells)+63)/64, 1, 1)
	_ = pass.End()
	pass.Release()
	if err := enc.CopyBufferToBuffer(out, 0, staging, 0, outLen); err != nil {
		return nil, fmt.Errorf("gpu: copy result to staging: %w", err)
	}
	cmd, err := enc.Finish(nil)
	if err != nil {
		return nil, err
	}
	d.queue.Submit(cmd)
	cmd.Release()
	enc.Release()

	done := false
	if err := staging.MapAsync(wgpu.MapModeRead, 0, outLen, func(wgpu.BufferMapAsyncStatus) { done = true }); err != nil {
		return nil, fmt.Errorf("gpu: map staging buffer: %w", err)
	}
	for !done {
		d.device.Poll(true, nil)
	}
	raw := staging.GetMappedRange(0, uint(outLen))
	res := make([]float32, cells)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&res[0])), outLen), raw)
	_ = staging.Unmap()
	return res, nil
}
