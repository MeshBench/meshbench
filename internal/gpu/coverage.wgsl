// Coverage loss kernel: one invocation computes one raster cell's path loss
// from the station, FSPL plus Bullington diffraction over a rasterised
// height grid — the same construction as terrain.MultiEdgeLossDB, and the
// exact algorithm of coverage.GridLossCPU, which is its CPU twin (ADR-0004).
// Any change here must change both, and the equivalence test is the proof.

struct Params {
  grid_w: u32,
  grid_h: u32,
  raster_w: u32,
  raster_h: u32,
  g_south: f32,
  g_north: f32,
  g_west: f32,
  g_east: f32,
  r_south: f32,
  r_north: f32,
  r_west: f32,
  r_east: f32,
  st_lat: f32,
  st_lon: f32,
  st_alt: f32,
  remote_h: f32,
  freq_mhz: f32,
  steps: u32,
  pad0: u32,
  pad1: u32,
}

@group(0) @binding(0) var<storage, read> heights: array<f32>;
@group(0) @binding(1) var<storage, read_write> out: array<f32>;
@group(0) @binding(2) var<uniform> p: Params;

const NO_DATA_HEIGHT: f32 = -1e30;
const NO_DATA_LOSS: f32 = 3.4e38;
const PI: f32 = 3.14159265358979;

fn height_at(lat: f32, lon: f32) -> vec2<f32> {
  // Returns (height, ok). Bilinear, clamped at the edges — the same
  // interpolation HeightGrid.At performs.
  var fx = (lon - p.g_west) / (p.g_east - p.g_west) * f32(p.grid_w) - 0.5;
  var fy = (lat - p.g_south) / (p.g_north - p.g_south) * f32(p.grid_h) - 0.5;
  let x0 = i32(floor(fx));
  let y0 = i32(floor(fy));
  let tx = fx - f32(x0);
  let ty = fy - f32(y0);
  let x0c = clamp(x0, 0, i32(p.grid_w) - 1);
  let x1c = clamp(x0 + 1, 0, i32(p.grid_w) - 1);
  let y0c = clamp(y0, 0, i32(p.grid_h) - 1);
  let y1c = clamp(y0 + 1, 0, i32(p.grid_h) - 1);
  let h00 = heights[u32(y0c) * p.grid_w + u32(x0c)];
  let h10 = heights[u32(y0c) * p.grid_w + u32(x1c)];
  let h01 = heights[u32(y1c) * p.grid_w + u32(x0c)];
  let h11 = heights[u32(y1c) * p.grid_w + u32(x1c)];
  let bad = NO_DATA_HEIGHT / 2.0;
  if (h00 <= bad || h10 <= bad || h01 <= bad || h11 <= bad) {
    return vec2<f32>(0.0, 0.0);
  }
  let top = h00 * (1.0 - tx) + h10 * tx;
  let bot = h01 * (1.0 - tx) + h11 * tx;
  return vec2<f32>(top * (1.0 - ty) + bot * ty, 1.0);
}

fn haversine_km(lat1: f32, lon1: f32, lat2: f32, lon2: f32) -> f32 {
  let rad = PI / 180.0;
  let d_lat = (lat2 - lat1) * rad;
  let d_lon = (lon2 - lon1) * rad;
  let a = sin(d_lat / 2.0) * sin(d_lat / 2.0) +
    cos(lat1 * rad) * cos(lat2 * rad) * sin(d_lon / 2.0) * sin(d_lon / 2.0);
  return 2.0 * 6371.0 * asin(min(1.0, sqrt(a)));
}

fn log10f(x: f32) -> f32 {
  return log(x) / log(10.0);
}

@compute @workgroup_size(64)
fn main(@builtin(global_invocation_id) gid: vec3<u32>) {
  let idx = gid.x;
  if (idx >= p.raster_w * p.raster_h) {
    return;
  }
  let x = idx % p.raster_w;
  let y = idx / p.raster_w;
  let lat = p.r_south + (p.r_north - p.r_south) * (f32(y) + 0.5) / f32(p.raster_h);
  let lon = p.r_west + (p.r_east - p.r_west) * (f32(x) + 0.5) / f32(p.raster_w);

  let dist_km = haversine_km(p.st_lat, p.st_lon, lat, lon);
  if (dist_km <= 0.0) {
    out[idx] = 0.0;
    return;
  }
  let rg = height_at(lat, lon);
  if (rg.y == 0.0) {
    out[idx] = NO_DATA_LOSS;
    return;
  }
  let d = dist_km * 1000.0;
  let lambda = 299.792458 / p.freq_mhz;
  let tx_alt = p.st_alt;
  let rx_alt = rg.x + p.remote_h;
  let slope = (rx_alt - tx_alt) / d;

  var max_from_tx = -3.4e38;
  var max_from_rx = -3.4e38;
  var worst_v = -3.4e38;
  var seen = false;
  for (var i: u32 = 1u; i < p.steps; i = i + 1u) {
    let f = f32(i) / f32(p.steps);
    let h = height_at(p.st_lat + (lat - p.st_lat) * f, p.st_lon + (lon - p.st_lon) * f);
    if (h.y == 0.0) {
      out[idx] = NO_DATA_LOSS;
      return;
    }
    let d1 = d * f;
    let d2 = d - d1;
    let hb = h.x + d1 * d2 / (2.0 * 4.0 / 3.0 * 6371000.0);
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
    seen = true;
  }
  let fspl = 32.44 + 20.0 * log10f(dist_km) + 20.0 * log10f(p.freq_mhz);
  if (!seen) {
    out[idx] = fspl;
    return;
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
