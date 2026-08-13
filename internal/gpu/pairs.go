package gpu

import (
	_ "embed"
	"fmt"
	"math"
	"unsafe"

	"github.com/cogentcore/webgpu/wgpu"

	"github.com/A13xB0/meshcoresim/internal/coverage"
)

//go:embed pairs.wgsl
var pairsWGSL string

// compilePairs builds the pair-loss pipeline, called from Open.
func (d *Device) compilePairs() error {
	mod, err := d.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label:          "pairs",
		WGSLDescriptor: &wgpu.ShaderModuleWGSLDescriptor{Code: pairsWGSL},
	})
	if err != nil {
		return fmt.Errorf("gpu: pairs shader: %w", err)
	}
	defer mod.Release()
	d.pairs, err = d.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:   "pairs",
		Compute: wgpu.ProgrammableStageDescriptor{Module: mod, EntryPoint: "main"},
	})
	if err != nil {
		return fmt.Errorf("gpu: pairs pipeline: %w", err)
	}
	return nil
}

// PairLoss runs the pair kernel: every node's path loss to every other, over a
// rasterised height grid. Only the upper triangle of the n by n result is
// written. The CPU twin is coverage.PairLossCPU, and the equivalence test
// holds the two together.
func (d *Device) PairLoss(g coverage.HeightGrid, nodes []coverage.PairNode,
	p coverage.PairParams) ([]float32, error) {

	if d.pairs == nil {
		return nil, fmt.Errorf("gpu: pairs pipeline not compiled")
	}
	n := len(nodes)
	if n < 2 {
		return make([]float32, n*n), nil
	}
	outLen := uint64(n * n * 4)

	hb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "heights", Contents: wgpu.ToBytes(g.Heights), Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		return nil, err
	}
	defer hb.Release()

	// Four floats per node, which is what the shader's struct is: three used
	// and one of padding, because a storage array of three-float structs is
	// laid out with a stride the host has to guess at.
	flat := make([]float32, 0, n*4)
	for _, nd := range nodes {
		flat = append(flat, float32(nd.Lat), float32(nd.Lon), float32(nd.AGLm), 0)
	}
	nb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "nodes", Contents: wgpu.ToBytes(flat), Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		return nil, err
	}
	defer nb.Release()

	out, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "pairloss", Size: outLen,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, err
	}
	defer out.Release()

	f := math.Float32bits
	params := []uint32{
		uint32(g.W), uint32(g.H), uint32(n), uint32(p.StepsCap),
		f(float32(g.South)), f(float32(g.North)), f(float32(g.West)), f(float32(g.East)),
		f(float32(p.FreqMHz)), f(float32(p.StepM)), 0, 0,
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
		Layout: d.pairs.GetBindGroupLayout(0),
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: hb, Size: uint64(len(g.Heights) * 4)},
			{Binding: 1, Buffer: nb, Size: uint64(len(flat) * 4)},
			{Binding: 2, Buffer: out, Size: outLen},
			{Binding: 3, Buffer: pb, Size: uint64(len(params) * 4)},
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
	pass.SetPipeline(d.pairs)
	pass.SetBindGroup(0, bg, nil)
	pass.DispatchWorkgroups((uint32(n*n)+63)/64, 1, 1)
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
	res := make([]float32, n*n)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&res[0])), outLen), raw)
	_ = staging.Unmap()
	return res, nil
}
