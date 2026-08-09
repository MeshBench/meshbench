# Wireshark dissector

```
cp meshcoresim.lua ~/.local/lib/wireshark/plugins/
```

Then open any `.pcapng` written by `internal/capture`.

Useful filters:

```
msim.to == 9 && msim.crc_ok == 0     what node 9 heard but could not decode
msim.outcome == 3                    frames dropped by dedup — working as designed
msim.from == 1 && msim.outcome != 5  node 1's frames that nobody relayed
msim.snr < -100                      marginal receptions (SNR is dB x10)
```

The last two are the point of a **merged** capture: one transmission appears
once per receiver, so you can see that node A decoded a frame while node B did
not. Per-receiver files cannot show that.

## Known drift risk

This file duplicates knowledge of MeshCore's packet format that also lives in
the firmware we link, so it **will** go stale. Kept deliberately minimal —
pseudo-header in full, MeshCore body only as far as the header byte. The honest
fixes are to generate it from one source of truth, or to test it against
captures in CI. Expanding it by hand until it looks complete is how it silently
becomes wrong.

The pseudo-header is versioned. Bump `PseudoHeaderVersion` in
`internal/capture/pcapng.go` on any layout change — captures outlive the code
that wrote them.
