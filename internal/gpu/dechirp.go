// Package gpu runs the DSP kernels on the GPU.
//
// Every kernel here has a CPU twin in internal/dsp and a test asserting they
// agree (ADR-0004). The CPU path is the oracle, not a fallback — CI has no GPU,
// and a wrong kernel does not crash, it produces a plausible waterfall and
// slightly wrong sensitivity.
package gpu

import (
	_ "embed"
	"fmt"
	"unsafe"

	"github.com/cogentcore/webgpu/wgpu"
)

//go:embed dechirp.wgsl
var dechirpWGSL string

// Device is an acquired GPU with the pipelines compiled.
type Device struct {
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue
	dechirp  *wgpu.ComputePipeline
	coverage *wgpu.ComputePipeline
	pairs    *wgpu.ComputePipeline
	Name     string
	Backend  string
	// MaxStorageMB is the largest storage buffer this device will bind, so a
	// caller sizing a grid can ask first rather than fail after.
	MaxStorageMB uint64
}

// Open acquires a GPU and compiles every shader up front.
//
// All shaders are compiled at startup deliberately: a shader error is a runtime
// error, and finding it at the first frame that happens to use one is much worse
// than finding it now.
func Open() (*Device, error) {
	inst := wgpu.CreateInstance(nil)
	if inst == nil {
		return nil, fmt.Errorf("gpu: no WebGPU instance (no Vulkan/Metal/DX12 backend?)")
	}
	ad, err := inst.RequestAdapter(nil)
	if err != nil {
		return nil, fmt.Errorf("gpu: no adapter: %w", err)
	}
	// The adapter's own limits rather than WebGPU's defaults. The default
	// storage-buffer binding is 128 MiB, and a country-sized height grid is
	// twice that: with the default, the pairs kernel fails to bind on exactly
	// the network that needs it most, and the failure reads as a silent fall
	// back to the processor.
	supported := ad.GetLimits()
	dev, err := ad.RequestDevice(&wgpu.DeviceDescriptor{
		RequiredLimits: &wgpu.RequiredLimits{Limits: supported.Limits},
	})
	if err != nil {
		// A card that refuses its own reported limits still works at the
		// defaults, only smaller.
		dev, err = ad.RequestDevice(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("gpu: no device: %w", err)
	}
	info := ad.GetInfo()
	// The device's own limits, not the adapter's. The request for the
	// adapter's limits can be refused, and the fallback device then sits at
	// WebGPU's defaults - so sizing work from what the adapter advertised
	// binds a 268 MB grid to a 128 MiB limit and fails after the grid was
	// already built.
	actual := dev.GetLimits()
	d := &Device{
		instance: inst, adapter: ad, device: dev, queue: dev.GetQueue(),
		Name: info.Name, Backend: info.BackendType.String(),
		MaxStorageMB: actual.Limits.MaxStorageBufferBindingSize / (1 << 20),
	}
	mod, err := dev.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label:          "dechirp",
		WGSLDescriptor: &wgpu.ShaderModuleWGSLDescriptor{Code: dechirpWGSL},
	})
	if err != nil {
		return nil, fmt.Errorf("gpu: dechirp shader: %w", err)
	}
	defer mod.Release()
	d.dechirp, err = dev.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:   "dechirp",
		Compute: wgpu.ProgrammableStageDescriptor{Module: mod, EntryPoint: "main"},
	})
	if err != nil {
		return nil, fmt.Errorf("gpu: dechirp pipeline: %w", err)
	}
	if err := d.compileCoverage(); err != nil {
		return nil, err
	}
	if err := d.compilePairs(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Device) Close() {
	if d.coverage != nil {
		d.coverage.Release()
	}
	if d.dechirp != nil {
		d.dechirp.Release()
	}
	if d.device != nil {
		d.device.Release()
	}
	if d.adapter != nil {
		d.adapter.Release()
	}
	if d.instance != nil {
		d.instance.Release()
	}
}

// Dechirp runs the kernel over a batch of symbols. Input and output are
// interleaved float32 pairs (re, im) — complex64 in memory layout, chosen
// because f32 is what the GPU has and pretending otherwise would hide the
// precision difference the equivalence test exists to measure.
func (d *Device) Dechirp(rx []complex64, sf int) ([]complex64, error) {
	n := 1 << sf
	if len(rx)%n != 0 {
		return nil, fmt.Errorf("gpu: %d samples is not a whole number of SF%d symbols", len(rx), sf)
	}
	symbols := len(rx) / n
	byteLen := uint64(len(rx) * 8) // 2 × f32

	in, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label:    "rx",
		Contents: wgpu.ToBytes(rx),
		Usage:    wgpu.BufferUsageStorage,
	})
	if err != nil {
		return nil, err
	}
	defer in.Release()

	out, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "out", Size: byteLen,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, err
	}
	defer out.Release()

	params := []uint32{uint32(n), uint32(symbols), 0, 0}
	pb, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "params", Contents: wgpu.ToBytes(params), Usage: wgpu.BufferUsageUniform,
	})
	if err != nil {
		return nil, err
	}
	defer pb.Release()

	staging, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging", Size: byteLen,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, err
	}
	defer staging.Release()

	bg, err := d.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: d.dechirp.GetBindGroupLayout(0),
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: in, Size: byteLen},
			{Binding: 1, Buffer: out, Size: byteLen},
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
	pass.SetPipeline(d.dechirp)
	pass.SetBindGroup(0, bg, nil)
	groups := (uint32(len(rx)) + 63) / 64
	pass.DispatchWorkgroups(groups, 1, 1)
	_ = pass.End()
	pass.Release()
	if err := enc.CopyBufferToBuffer(out, 0, staging, 0, byteLen); err != nil {
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
	// A failed map does not crash — it leaves the staging buffer unreadable and
	// the result would be whatever was already in memory, which looks like a
	// plausible waterfall. Exactly the failure CLAUDE.md warns about.
	if err := staging.MapAsync(wgpu.MapModeRead, 0, byteLen, func(wgpu.BufferMapAsyncStatus) { done = true }); err != nil {
		return nil, fmt.Errorf("gpu: map staging buffer: %w", err)
	}
	for !done {
		d.device.Poll(true, nil)
	}
	raw := staging.GetMappedRange(0, uint(byteLen))
	res := make([]complex64, len(rx))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&res[0])), byteLen), raw)
	_ = staging.Unmap() // the data is already copied out
	return res, nil
}
