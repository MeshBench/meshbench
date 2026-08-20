> **Working note, last true on 9 August 2026.** Kept for the thinking in it, not maintained as a description of the code. **2 of the 3 package paths it names no longer exist**, the seven-layer restructure of 19 August having moved them. Where this disagrees with the tree, the tree is right; the authority is the code in `internal/rf/`, and its tests.

# The RF chain

Implementation-level detail for `internal/dsp` and `internal/rf`. Every constant
here is checkable; if an implementation disagrees with this document, one of them
is wrong and it is worth finding out which.

## 1. LoRa modulation

A symbol is a chirp sweeping bandwidth *BW*, cyclically shifted by symbol value
*s* ∈ [0, 2^SF).

| Quantity | Formula | SF7/125k | SF12/125k |
|---|---|---|---|
| Samples/symbol `N` | `2^SF` | 128 | 4096 |
| Symbol time `Ts` | `2^SF / BW` | 1.024 ms | 32.768 ms |
| Chirp rate `μ` | `BW² / 2^SF` | 122 kHz/ms | 3.8 kHz/ms |

Baseband upchirp, sample *n*, at 1 sample/Hz:

```
k    = (s + n) mod N
x[n] = exp( j2π · ( k²/(2N) − k/2 ) )
```

Downchirp is the complex conjugate. The SF12/SF7 duration ratio is exactly 32,
which is why SF12 is expensive in airtime *and* in collision exposure.

### Frame structure

```
[ preamble 8 sym ][ sync 2 sym ][ SFD 2.25 downchirp ][ header ][ payload ][ CRC ]
```

Model all of it. A collision landing in the preamble prevents synchronisation
entirely; one landing in the payload may still decode. **Collapsing that
distinction is the commonest simulator error** and it changes collision
statistics materially.

### Time on air

```
n_payload = 8 + max( ceil( (8·PL − 4·SF + 28 + 16·CRC − 20·IH)
                           / (4·(SF − 2·DE)) ) · (CR + 4), 0 )
T_packet  = (n_preamble + 4.25 + n_payload) · Ts
```

`DE` = low-data-rate optimisation (1 when `Ts > 16 ms`, i.e. SF11/12 at 125 kHz),
`IH` = implicit header, `CR` ∈ [1,4].

**Cross-check required:** this must agree with MeshCore's own
`getEstAirtimeFor()`. Write the test before the modulator.

## 2. Noise

```
N_dBm = −174 + 10·log10(BW_Hz) + NF_dB
```

| BW | NF 6 dB | NF 0 dB (ideal) |
|---|---|---|
| 125 kHz | **−117.0 dBm** | −123.0 dBm |
| 250 kHz | **−114.0 dBm** | −120.0 dBm |
| 500 kHz | **−111.0 dBm** | −117.0 dBm |

These are pinned by test in `internal/dsp/lora_test.go`. Every sensitivity claim
in the project is measured against them, so a drift here moves every reported
margin invisibly.

Complex AWGN, variance `N/2` per component, from counter-based RNG so a seed
reproduces the exact noise realisation on any thread or GPU lane.

## 3. Processing gain and sensitivity

```
G_p = 10·log10(2^SF)
```

| SF | 7 | 8 | 9 | 10 | 11 | 12 |
|---|---|---|---|---|---|---|
| `G_p` dB | 21.07 | 24.08 | 27.09 | 30.10 | 33.11 | **36.12** |

This is why LoRa decodes below the noise floor: required SNR at SF12 is about
−20 dB.

### The acceptance test

Sweep TX power down, measure packet error rate over ≥1,000 frames, find the 1%
PER crossing. **Simulated sensitivity must land within ~2 dB of Semtech's
published SX1262 figures** across SF7–SF12 at BW 125/250 kHz (SF12/125k ≈
−137 dBm).

If this fails, the chain is wrong — however convincing the waterfall looks. This
is the only evidence that any of the rest is trustworthy.

## 4. Demodulation

```
1.  y_dechirped[n] = y[n] · conj(base_upchirp[n])
2.  Y = FFT_N(y_dechirped)
3.  symbol     = argmax |Y[k]|
    confidence = |Y[peak]| / |Y[second_peak]|      → shown in the UI
```

Capture effect is **emergent**: with two overlapping frames the stronger produces
the dominant bin. Never write a rule for it — that is precisely why it will be
right in cases nobody anticipated.

## 5. The channel

```
y_j[n] = Σ_i  a_ij · x_i[n − d_ij] · exp(j·φ_ij)  +  w[n]
```

- `a_ij` — linear amplitude from the full link budget (§6).
- `d_ij` — propagation delay in samples, `distance / c · BW`. At 125 kHz one
  sample is **2.4 km**, so typical mesh delays are a fraction of a sample; they
  still matter through `φ`, which is what makes summation coherent rather than a
  fudge.
- `w[n]` — AWGN per §2.

## 6. Link budget

```
P_rx(dBm) = P_tx
          − L_feedline_tx
          + G_tx(θ_tx, φ_tx)        ← evaluated in the true direction
          − L_path                  ← FSPL + diffraction (§7)
          + G_rx(θ_rx, φ_rx)        ← the *other* look angle, not the same one
          − L_feedline_rx
          − L_polarisation          ← 20–30 dB on mismatch
```

Both directions are computed separately. The look angle from A to B is not the
look angle from B to A once elevation is involved.

## 7. Path loss

```
FSPL(dB) = 32.44 + 20·log10(d_km) + 20·log10(f_MHz)
```

Diffraction: **multi-edge Deygout**, never single knife-edge. In hamreach a
Glen Coe link read 36.8 dB under single-edge (verdict: *works*, through a
1,084 m massif) and 120.1 dB once corrected.

```
Fresnel r  = sqrt( λ·d1·d2 / (d1+d2) )        60% clearance is the usable rule
Earth bulge = d1·d2 / (2·k·R_e),  k = 4/3, R_e = 6,371,000 m
```

**Check the bulge against a known figure, not against its own plausibility.** It
was once wrong by a factor of 1,000 in hamreach and survived review.

## 8. Cost

| Quantity | Value |
|---|---|
| Samples/s/receiver @125 kHz | 125,000 complex |
| Bytes/s/receiver (cf32) | 1.0 MB |
| FFTs per symbol | 1 of size 2^SF |
| Symbols/s @ SF7 | ~977 |
| 50 receivers, SF7, real time | ~49k FFTs/s of size 128 |

Scales with receiver count *N*, not *N²*, because each receiver sums its own
arrivals once. This is why the GPU exists (ADR-0004).

## 9. Known-wrong-on-purpose

Not modelled: multipath/fading, Doppler, oscillator ppm error, non-LoRa
interference beyond a flat floor (until ADR-0012), PA non-linearity, phase noise,
body loss, near-field effects, antenna mutual coupling.

**Every one of these makes reality worse than simulation.** The bias is
one-directional and must be surfaced in the UI. ADR-0015's validation against
observer data is how we find out its size.
