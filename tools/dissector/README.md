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

## Two files, two jobs

`meshcoresim.lua` is the metadata layer: which node received a frame, what it
then did with it, the true RSSI and SNR, and the node names — everything only a
simulator can know. It registers on loopback UDP (live) and on DLT_USER0 (a
saved pcapng).

`meshcore_dissector.lua` is the MeshCore protocol itself, and is **not ours**:

> https://github.com/aaronb/wireshark-meshcore — Copyright (C) 2025 Aaron Brown,
> GPL-2.0-only. Vendored verbatim, with its licence in
> `LICENSE.meshcore_dissector`.

It knows the wire format in far more detail than anything we would write —
advert app-data, acks, traces, group data — and two dissectors for one format
only guarantees they drift. Ours calls it for the frame body.

**Licensing:** that file is GPL-2.0-only. It is a standalone script Wireshark
loads at runtime, not linked into the binary, but it is still GPL and this
project has not chosen a licence (ADR-0001). Worth settling deliberately rather
than by accident.

## Loading them

The workbench does it for you — *Simulation → capture live to Wireshark* starts
the capture and opens Wireshark with both scripts and a filter for our port.
By hand:

    wireshark -k -i lo -f "udp port 5555" \
      -X lua_script:tools/dissector/meshcore_dissector.lua \
      -X lua_script:tools/dissector/meshcoresim.lua

Order matters: ours registers DLT_USER0 last so our captures get our header.

## Filters worth knowing

    msim.to_name == "West Lomond"              what one repeater heard
    msim.to_name == "West Lomond" && msim.outcome == 1
                                               ... and could not decode
    meshcore.path_hash_size == 3               3-byte path hashes
    meshcore.payload_type == 5                 group text - a channel message
    msim.snr < 0                               only just arrived
