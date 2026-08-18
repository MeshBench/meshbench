// The demodulation kernel's host side.
package gpu

import (
	_ "embed"
	"fmt"
	"math/cmplx"
	"unsafe"

	"github.com/cogentcore/webgpu/wgpu"

	"github.com/MeshBench/meshbench/internal/dsp"
)

//go:embed demod.wgsl
var demodWGSL string

// compileDemod builds the demodulation pipeline; called from Open.
func (d *Device) compileDemod() error {
	mod, err := d.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label:          "demod",
		WGSLDescriptor: &wgpu.ShaderModuleWGSLDescriptor{Code: demodWGSL},
	})
	if err != nil {
		return fmt.Errorf("gpu: demod shader: %w", err)
	}
	defer mod.Release()
	d.demod, err = d.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:   "demod",
		Compute: wgpu.ProgrammableStageDescriptor{Module: mod, EntryPoint: "main"},
	})
	if err != nil {
		return fmt.Errorf("gpu: demod pipeline: %w", err)
	}
	return nil
}

// DemodBatch demodulates a run of consecutive symbols: rx holds symbols*2^sf
// samples, and the result is one FFT-argmax bin per symbol, with the
// demodulator's peak-to-second confidence scaled by a thousand.
//
// SF12's 4096-sample symbol does not fit the 16 KiB workgroup memory the
// WebGPU default guarantees; callers keep SF12 on the CPU oracle.
func (d *Device) DemodBatch(rx []complex64, sf int) (bins []int, conf []float64, err error) {
	if sf > 11 {
		return nil, nil, fmt.Errorf("gpu: SF%d exceeds workgroup memory; use the CPU path", sf)
	}
	n := 1 << sf
	if len(rx)%n != 0 {
		return nil, nil, fmt.Errorf("gpu: %d samples is not whole SF%d symbols", len(rx), sf)
	}
	symbols := len(rx) / n
	if symbols == 0 {
		return nil, nil, nil
	}

	// conj of the base upchirp, the same reference the CPU dechirps with.
	base := dsp.Modulator{SF: sf}.BaseUpchirp()
	ch := make([]complex64, n)
	for i, v := range base {
		ch[i] = complex64(cmplx.Conj(v))
	}

	in, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "demod-rx", Contents: wgpu.ToBytes(rx), Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		return nil, nil, err
	}
	defer in.Release()
	chirp, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label: "demod-chirp", Contents: wgpu.ToBytes(ch), Usage: wgpu.BufferUsageStorage,
	})
	if err != nil {
		return nil, nil, err
	}
	defer chirp.Release()

	outLen := uint64(symbols * 8) // vec2<u32>
	out, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "demod-out", Size: outLen,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, nil, err
	}
	defer out.Release()
	read, err := d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "demod-read", Size: outLen,
		Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead,
	})
	if err != nil {
		return nil, nil, err
	}
	defer read.Release()

	params := struct{ N, Symbols uint32 }{uint32(n), uint32(symbols)}
	pbuf, err := d.device.CreateBufferInit(&wgpu.BufferInitDescriptor{
		Label:    "demod-params",
		Contents: unsafe.Slice((*byte)(unsafe.Pointer(&params)), 8),
		Usage:    wgpu.BufferUsageUniform,
	})
	if err != nil {
		return nil, nil, err
	}
	defer pbuf.Release()

	layout := d.demod.GetBindGroupLayout(0)
	defer layout.Release()
	bind, err := d.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: layout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: in, Size: wgpu.WholeSize},
			{Binding: 1, Buffer: chirp, Size: wgpu.WholeSize},
			{Binding: 2, Buffer: out, Size: wgpu.WholeSize},
			{Binding: 3, Buffer: pbuf, Size: wgpu.WholeSize},
		},
	})
	if err != nil {
		return nil, nil, err
	}
	defer bind.Release()

	enc, err := d.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, nil, err
	}
	pass := enc.BeginComputePass(nil)
	pass.SetPipeline(d.demod)
	pass.SetBindGroup(0, bind, nil)
	pass.DispatchWorkgroups(uint32(symbols), 1, 1)
	if err := pass.End(); err != nil {
		return nil, nil, err
	}
	if err := enc.CopyBufferToBuffer(out, 0, read, 0, outLen); err != nil {
		return nil, nil, err
	}
	cmd, err := enc.Finish(nil)
	if err != nil {
		return nil, nil, err
	}
	d.queue.Submit(cmd)

	done := make(chan wgpu.BufferMapAsyncStatus, 1)
	err = read.MapAsync(wgpu.MapModeRead, 0, outLen,
		func(s wgpu.BufferMapAsyncStatus) { done <- s })
	if err != nil {
		return nil, nil, err
	}
	d.device.Poll(true, nil)
	if s := <-done; s != wgpu.BufferMapAsyncStatusSuccess {
		return nil, nil, fmt.Errorf("gpu: demod readback: %v", s)
	}
	raw := read.GetMappedRange(0, uint(outLen))
	pairs := unsafe.Slice((*uint32)(unsafe.Pointer(&raw[0])), symbols*2)
	bins = make([]int, symbols)
	conf = make([]float64, symbols)
	for i := 0; i < symbols; i++ {
		bins[i] = int(pairs[i*2])
		conf[i] = float64(pairs[i*2+1]) / 1000
	}
	if err := read.Unmap(); err != nil {
		return nil, nil, err
	}
	return bins, conf, nil
}
