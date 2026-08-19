// Dechirp: multiply a received symbol by the conjugate of the base upchirp.
//
// This is the front half of LoRa demodulation and the hot loop of the whole
// simulator — one dispatch per symbol per receiver. The FFT that follows is the
// other half.
//
// Complex values are packed as vec2<f32>(re, im). Per ADR-0004 this kernel has a
// CPU twin in internal/dsp, and a test asserts they agree: a wrong dechirp does
// not crash, it produces a plausible waterfall and slightly wrong sensitivity.

struct Params {
  n         : u32,   // samples per symbol, 2^SF
  symbols   : u32,   // how many symbols in this batch
  _pad0     : u32,
  _pad1     : u32,
};

@group(0) @binding(0) var<storage, read>       rx     : array<vec2<f32>>;
@group(0) @binding(1) var<storage, read_write> out    : array<vec2<f32>>;
@group(0) @binding(2) var<uniform>             params : Params;

const TAU : f32 = 6.283185307179586;

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let idx = gid.x;
  let total = params.n * params.symbols;
  if (idx >= total) { return; }

  let i  = idx % params.n;          // sample within the symbol
  let nf = f32(params.n);
  let k  = f32(i);

  // Base upchirp phase, matching chirpSample() in internal/dsp/chirp.go with
  // s = 0. The conjugate is applied by negating the imaginary part below.
  let phase = TAU * (k * k / (2.0 * nf) - k / 2.0);
  let base  = vec2<f32>(cos(phase), sin(phase));

  let v = rx[idx];
  // complex multiply by conj(base): (a+bi)(c-di)
  out[idx] = vec2<f32>(
    v.x * base.x + v.y * base.y,
    v.y * base.x - v.x * base.y
  );
}
