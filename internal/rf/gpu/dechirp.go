// Package gpu runs the DSP kernels on the GPU.
//
// Every kernel here has a CPU twin in internal/dsp and a test asserting they
// agree (ADR-0004). The CPU path is the oracle, not a fallback: a wrong kernel
// does not crash, it produces a plausible waterfall and slightly wrong
// sensitivity, and CI runs the twin rather than the kernel, because a runner
// has no GPU for the kernel to run on.
package gpu

import (
	_ "embed"
	"fmt"
	"sync"
	"unsafe"

	"github.com/cogentcore/webgpu/wgpu"
)

// acquireMu serialises acquiring and releasing a device. The wgpu adapter and
// device requests register a cgo.Handle for their callback and delete it when
// the callback fires; two Opens - the GPU probe, a warm, and a coverage map can
// each reach one on a fresh session - racing that path, or a Close releasing a
// device while another Open is mid-request, is the "misuse of an invalid
// Handle" panic that made a green CI tick mean less. Only acquisition and
// release are guarded; work on an already-open device is not, so nothing here
// serialises the actual GPU passes.
var acquireMu sync.Mutex

//go:embed dechirp.wgsl
var dechirpWGSL string

// Device is an acquired GPU with the pipelines compiled.
type Device struct {
	instance     *wgpu.Instance
	adapter      *wgpu.Adapter
	device       *wgpu.Device
	queue        *wgpu.Queue
	dechirp      *wgpu.ComputePipeline
	demod        *wgpu.ComputePipeline
	coverage     *wgpu.ComputePipeline
	coverageFold *wgpu.ComputePipeline
	pairs        *wgpu.ComputePipeline
	Name         string
	Backend      string
	// MaxStorageMB is the largest storage buffer this device will bind, so a
	// caller sizing a grid can ask first rather than fail after.
	MaxStorageMB uint64
}

// usable refuses an adapter that is not a GPU at all.
//
// Wherever there is no hardware to drive, Mesa still answers: llvmpipe presents
// itself as an OpenGL adapter, and lavapipe as a Vulkan one. Both open cleanly,
// compile the shaders and run them, so nothing downstream can tell that "on the
// GPU" now means the processor wearing a shader compiler, running one kernel
// through a software rasteriser instead of the reference path that computes the
// same answer directly. That is slower than the CPU twin it would be standing
// in for, and it leaves the interface claiming an acceleration the machine
// never had.
//
// So an adapter that reports itself as CPU is declined and the reason is
// returned, which is the promise the project makes about every GPU path: a
// machine without a usable one loses time, not features.
func usable(info wgpu.AdapterInfo) error {
	if info.AdapterType != wgpu.AdapterTypeCPU {
		return nil
	}
	name := info.Name
	if name == "" {
		name = "the adapter"
	}
	return fmt.Errorf(
		"gpu: %s is a software rasteriser on the %s backend, not a GPU: the "+
			"processor reference path computes the same answer faster",
		name, info.BackendType)
}

// Open acquires a GPU and compiles every shader up front.
//
// All shaders are compiled at startup deliberately: a shader error is a runtime
// error, and finding it at the first frame that happens to use one is much worse
// than finding it now.
func Open() (*Device, error) {
	acquireMu.Lock()
	defer acquireMu.Unlock()
	inst := wgpu.CreateInstance(nil)
	if inst == nil {
		return nil, fmt.Errorf("gpu: no WebGPU instance (no Vulkan/Metal/DX12 backend?)")
	}
	ad, err := inst.RequestAdapter(nil)
	if err != nil {
		return nil, fmt.Errorf("gpu: no adapter: %w", err)
	}
	info := ad.GetInfo()
	if err := usable(info); err != nil {
		ad.Release()
		inst.Release()
		return nil, err
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
	if err := d.compileCoverageFold(); err != nil {
		return nil, err
	}
	if err := d.compilePairs(); err != nil {
		return nil, err
	}
	if err := d.compileDemod(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Device) Close() {
	acquireMu.Lock()
	defer acquireMu.Unlock()
	if d.coverage != nil {
		d.coverage.Release()
	}
	if d.coverageFold != nil {
		d.coverageFold.Release()
	}
	if d.pairs != nil {
		d.pairs.Release()
	}
	if d.dechirp != nil {
		d.dechirp.Release()
	}
	if d.demod != nil {
		d.demod.Release()
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
	cmd, err := enc.Finish(nil)
	if err != nil {
		return nil, err
	}
	d.queue.Submit(cmd)
	cmd.Release()
	enc.Release()

	raw, err := d.readBuffer(out, int(byteLen))
	if err != nil {
		return nil, err
	}
	res := make([]complex64, len(rx))
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&res[0])), byteLen), raw)
	return res, nil
}
