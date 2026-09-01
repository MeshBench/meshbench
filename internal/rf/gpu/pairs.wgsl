// Pair loss kernel: one invocation computes one pair of nodes' path loss over
// a profile somebody already gathered — FSPL plus Bullington diffraction, the
// exact algorithm of terrain.MultiEdgeLossDB. The CPU twin is
// coverage.ProfilePairLossCPU, and the equivalence test holds the two together
// (ADR-0004).
//
// Profiles rather than a rasterised grid, and the difference is the lesson of
// the first attempt: a bounding-box grid reads ground no path crosses, which
// downloads tiles the old workbench never needed and grows past what a card
// will bind on exactly the networks that need help. A profile is only the
// ground under the path, gathered from the same hot tile cache the processor
// uses, so the heights here are byte-identical to the CPU's and the buffer is
// small at any span.

struct Params {
  n_pairs: u32,
  freq_mhz: f32,
  pad0: u32,
  pad1: u32,
}

// heights holds every profile end to end. meta_u is offset and point count
// per pair; meta_f is distance in metres and the two antenna heights above
// ground. Parallel arrays rather than a struct, because a storage struct's
// stride is a negotiation and three flat arrays are not.
@group(0) @binding(0) var<storage, read> heights: array<f32>;
@group(0) @binding(1) var<storage, read> meta_u: array<u32>;
@group(0) @binding(2) var<storage, read> meta_f: array<f32>;
@group(0) @binding(3) var<storage, read_write> out: array<f32>;
@group(0) @binding(4) var<uniform> p: Params;

const PI: f32 = 3.14159265358979;

fn log10f(x: f32) -> f32 {
  return log(x) / log(10.0);
}

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
  let idx = gid.x;
  if (idx >= p.n_pairs) {
    return;
  }
  let off = meta_u[idx * 2u];
  let cnt = meta_u[idx * 2u + 1u];
  let d = meta_f[idx * 3u];
  let agl_a = meta_f[idx * 3u + 1u];
  let agl_b = meta_f[idx * 3u + 2u];

  let dist_km = d / 1000.0;
  if (dist_km <= 0.0) {
    // Co-located, or a profile packed with no length: log10(0) is -Inf, and an
    // infinite negative loss reads downstream as an infinite margin. Zero, as
    // the coverage kernel returns for the station's own cell, and as the CPU
    // twin returns here.
    out[idx] = 0.0;
    return;
  }
  let fspl = 32.44 + 20.0 * log10f(dist_km) + 20.0 * log10f(p.freq_mhz);
  if (cnt < 3u) {
    out[idx] = fspl;
    return;
  }

  let lambda = 299.792458 / p.freq_mhz;
  let tx_alt = heights[off] + agl_a;
  let rx_alt = heights[off + cnt - 1u] + agl_b;
  let slope = (rx_alt - tx_alt) / d;
  let n = f32(cnt - 1u);

  var max_from_tx = -3.4e38;
  var max_from_rx = -3.4e38;
  var worst_v = -3.4e38;
  for (var i: u32 = 1u; i < cnt - 1u; i = i + 1u) {
    let f = f32(i) / n;
    let d1 = d * f;
    let d2 = d - d1;
    // Four-thirds earth, as the CPU adds: leaving the bulge out is a horizon
    // several kilometres closer than the real one.
    let hb = heights[off + i] + d1 * d2 / (2.0 * 4.0 / 3.0 * 6371000.0);
    let s_tx = (hb - tx_alt) / d1;
    if (s_tx > max_from_tx) {
      max_from_tx = s_tx;
    }
    let s_rx = (hb - rx_alt) / d2;
    if (s_rx > max_from_rx) {
      max_from_rx = s_rx;
    }
    let h_los = hb - (tx_alt + slope * d1);
    let vi = h_los * sqrt(2.0 * d / (lambda * d1 * d2));
    if (vi > worst_v) {
      worst_v = vi;
    }
  }

  var v: f32;
  if (max_from_tx < slope) {
    v = worst_v;
  } else {
    let db = (rx_alt - tx_alt + max_from_rx * d) / (max_from_tx + max_from_rx);
    if (db <= 0.0 || db >= d) {
      out[idx] = fspl;
      return;
    }
    let h = tx_alt + max_from_tx * db - (tx_alt + slope * db);
    v = h * sqrt(2.0 * d / (lambda * db * (d - db)));
  }
  var loss = 0.0;
  if (v > -0.78) {
    loss = 6.9 + 20.0 * log10f(sqrt((v - 0.1) * (v - 0.1) + 1.0) + v - 0.1);
  }
  if (loss > 0.0) {
    loss = loss + (1.0 - exp(-loss / 6.0)) * (10.0 + 0.02 * d / 1000.0);
  }
  out[idx] = fspl + loss;
}
