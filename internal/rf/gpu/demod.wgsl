// LoRa demodulation: dechirp, FFT, argmax - one workgroup per symbol.
//
// The CPU twin is dsp.Demodulator.DemodulateSymbolInto. This kernel exists
// for the waveform verdicts' batch shape: hundreds of receivers times
// hundreds of symbols, all independent. f32 against the CPU's f64 - the
// equivalence test measures the bin agreement rather than assuming it.
//
// The FFT is radix-2 in workgroup shared memory, so a symbol must fit:
// 2^SF complex values. At SF11 that is 16 KiB of shared f32 pairs, the
// WebGPU default limit; SF12 does not fit and stays on the CPU.

struct Params {
  n: u32,        // samples per symbol = 2^sf
  symbols: u32,  // how many symbols in the batch
}

@group(0) @binding(0) var<storage, read> rx: array<vec2f>;    // interleaved IQ
@group(0) @binding(1) var<storage, read> chirp: array<vec2f>; // conj(base upchirp)
@group(0) @binding(2) var<storage, read_write> out: array<vec2u>; // bin, confidence*1e3
@group(0) @binding(3) var<uniform> p: Params;

var<workgroup> buf: array<vec2f, 2048>;
var<workgroup> mag: array<f32, 2048>;

fn cmul(a: vec2f, b: vec2f) -> vec2f {
  return vec2f(a.x * b.x - a.y * b.y, a.x * b.y + a.y * b.x);
}

const PI: f32 = 3.14159265358979;

@compute @workgroup_size(256)
fn main(@builtin(workgroup_id) wid: vec3u, @builtin(local_invocation_id) lid: vec3u) {
  let sym = wid.x;
  if (sym >= p.symbols) { return; }
  let n = p.n;
  let base = sym * n;
  let tid = lid.x;

  // Dechirp straight into shared memory, bit-reversed for the in-place FFT.
  var bits = 0u;
  var t = n;
  while (t > 1u) { bits += 1u; t >>= 1u; }
  for (var i = tid; i < n; i += 256u) {
    let r = reverseBits(i) >> (32u - bits);
    buf[r] = cmul(rx[base + i], chirp[i]);
  }
  workgroupBarrier();

  // Iterative radix-2, tid-strided butterflies.
  var half = 1u;
  while (half < n) {
    let step = PI / f32(half);
    for (var k = tid; k < n / 2u; k += 256u) {
      let blk = k / half;
      let pos = k % half;
      let i0 = blk * half * 2u + pos;
      let i1 = i0 + half;
      let ang = -step * f32(pos);
      let w = vec2f(cos(ang), sin(ang));
      let a = buf[i0];
      let b = cmul(buf[i1], w);
      buf[i0] = a + b;
      buf[i1] = a - b;
    }
    workgroupBarrier();
    half <<= 1u;
  }

  for (var i = tid; i < n; i += 256u) {
    let v = buf[i];
    mag[i] = v.x * v.x + v.y * v.y;
  }
  workgroupBarrier();

  // Argmax on one thread: n is at most 2048 and the FFT above dominates.
  if (tid == 0u) {
    var peak = -1.0;
    var second = 0.0;
    var at = 0u;
    for (var i = 0u; i < n; i += 1u) {
      let m = mag[i];
      if (m > peak) { second = peak; peak = m; at = i; }
      else if (m > second) { second = m; }
    }
    var conf = 0.0;
    if (second > 0.0) { conf = sqrt(peak / second); }
    out[sym] = vec2u(at, u32(clamp(conf, 0.0, 4.0e6) * 1000.0));
  }
}
