package gpu

import (
	_ "embed"
	"fmt"
	"math"
	"unsafe"

	"github.com/cogentcore/webgpu/wgpu"

	"github.com/MeshBench/meshbench/internal/coverage"
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

// PairProfileLoss runs the pairs kernel over profiles somebody gathered: one
// loss per packed pair. The CPU twin is coverage.ProfilePairLossCPU, and the
// equivalence test holds the two together.
func (d *Device) PairProfileLoss(p coverage.PairProfiles, freqMHz float64) ([]float32, error) {
	if d.pairs == nil {
		return nil, fmt.Errorf("gpu: pairs pipeline not compiled")
	}
	n := p.Pairs()
	if n == 0 {
		return nil, nil
	}
	outLen := uint64(n * 4)

	mk := func(label string, contents []byte) (*wgpu.Buffer, error) {
		return d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
			Label: label, Contents: contents, Usage: wgpu.BufferUsageStorage,
		})
	}
	hb, err := mk("heights", wgpu.ToBytes(p.Heights))
	if err != nil {
		return nil, err
	}
	defer hb.Release()
	mu, err := mk("meta_u", wgpu.ToBytes(p.MetaU))
	if err != nil {
		return nil, err
	}
	defer mu.Release()
	mf, err := mk("meta_f", wgpu.ToBytes(p.MetaF))
	if err != nil {
		return nil, err
	}
	defer mf.Release()

	out, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "pairloss", Size: outLen,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, err
	}
	defer out.Release()

	params := []uint32{uint32(n), math.Float32bits(float32(freqMHz)), 0, 0}
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
			{Binding: 0, Buffer: hb, Size: uint64(len(p.Heights) * 4)},
			{Binding: 1, Buffer: mu, Size: uint64(len(p.MetaU) * 4)},
			{Binding: 2, Buffer: mf, Size: uint64(len(p.MetaF) * 4)},
			{Binding: 3, Buffer: out, Size: outLen},
			{Binding: 4, Buffer: pb, Size: uint64(len(params) * 4)},
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
	pass.DispatchWorkgroups((uint32(n)+63)/64, 1, 1)
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
	res := make([]float32, n)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&res[0])), outLen), raw)
	_ = staging.Unmap()
	return res, nil
}
