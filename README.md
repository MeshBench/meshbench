# MeshcoreSim documentation

Decisions live in Plane project **MSIM** as ADR-0001…ADR-0016. These documents
carry the implementation detail those decisions imply.

| Document | Read it when |
|---|---|
| [`architecture.md`](architecture.md) | Before writing any code. Module boundaries, data flow, time, determinism, concurrency. |
| [`rf-chain.md`](rf-chain.md) | Implementing `internal/dsp` or `internal/rf`. Every formula and constant, with the acceptance test. |
| [`firmware-integration.md`](firmware-integration.md) | Implementing `internal/firmware`. The shim contract, verified facts about upstream, and what is still unknown. |
| [`ux/`](ux/README.md) | Building UI. Seven rendered designs; regenerate with `go run ./tools/mockup`. |

## Start here

The spine is **MSIM-1 → MSIM-2 → MSIM-4**:

1. **MSIM-1** — prove MeshCore's mesh stack builds for the host. Everything
   assumes it. Timeboxed; a clear "no" is a finding, not a failure.
2. **MSIM-2** — the CPU LoRa modem, accepted only when simulated sensitivity
   lands within ~2 dB of Semtech's published SX1262 figures. This is the
   honesty test for the whole project.
3. **MSIM-4** — the channel: coherent summation, delay, AWGN. Capture effect
   must *emerge*.

If those three do not land, nothing else matters.

## The rules that are easy to violate

- **The channel decides nothing.** No code path may say "if two transmissions
  overlap, both fail".
- **Every GPU kernel has a CPU twin**, and a test asserts they agree. A wrong
  FFT does not crash — it produces a plausible waterfall and slightly wrong
  sensitivity that nobody notices for months.
- **Airtime must match the firmware's `getEstAirtimeFor()`**, or the simulation
  desynchronises from the firmware's own CSMA silently.
- **Reachability is asymmetric.** Both directions, always.
- **Position uncertainty propagates.** Too-uncertain nodes get no verdict.
- **The simulator is kinder than the air**, and must say so.
- [`shortcomings.md`](shortcomings.md) — what the model does not do, what it
  gets measurably wrong, and in which direction. Read before trusting a result.
