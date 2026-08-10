package gpu

import (
	_ "embed"
	"fmt"
	"math"
	"unsafe"

	"github.com/cogentcore/webgpu/wgpu"

	"github.com/A13xB0/meshcoresim/internal/coverage"
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
// coverage.GridLossCPU, and the equivalence test holds the two together.
func (d *Device) CoverageGridLoss(g coverage.HeightGrid, p coverage.GridLossParams) ([]float32, error) {
	if d.coverage == nil {
		return nil, fmt.Errorf("gpu: coverage pipeline not compiled")
	}
	cells := p.RasterW * p.RasterH
	outLen := uint64(cells * 4)

	hb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "heights", Contents: wgpu.ToBytes(g.Heights), Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		return nil, err
	}
	defer hb.Release()

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
