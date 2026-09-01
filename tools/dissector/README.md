# Wireshark dissector

Two files, both needed, and the order they load in matters. To open a `.pcapng`
written by `internal/sim/capture`:

```
wireshark -X lua_script:tools/dissector/meshcore_dissector.lua \
          -X lua_script:tools/dissector/meshbench.lua \
          your-capture.pcapng
```

Both scripts claim DLT_USER0, and whichever loads second wins, so ours goes
last. Copying them into `~/.local/lib/wireshark/plugins/` instead does not do
that: Wireshark loads a plugins directory alphabetically, `meshbench.lua` sorts
before `meshcore_dissector.lua`, and the frame is then read with the vendored
radio header rather than ours. It does not look like a failure, which is why it
is worth saying: the frames still decode, into the wrong fields.

Copying `meshbench.lua` alone is the other half of the same trap. The msim
columns all populate (protocol, from, to, RSSI, SNR) and the MeshCore body is
simply absent, with nothing on screen saying a dissector is missing.

The live view has no such clash: `msim` is the only thing registered on the
capture port, so a plugins directory is fine there.

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
`internal/sim/capture/pcapng.go` on any layout change: captures outlive the
code that wrote them.

## Two files, two jobs

`meshbench.lua` is the metadata layer: which node received a frame, what it
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
loads at runtime, not linked into the binary, so it sits alongside MeshBench's
own GPL-3.0-or-later (`docs/licence.md`) rather than combining with it.

## Loading them

The workbench does it for you: *Simulation → capture live to Wireshark* starts
the capture and opens Wireshark with both scripts, in this order, and a filter
for our port. By hand, live:

    wireshark -k -i lo -f "udp port 5555" \
      -X lua_script:tools/dissector/meshcore_dissector.lua \
      -X lua_script:tools/dissector/meshbench.lua

Order matters, and it is worth being precise about why, because the comment in
`meshbench.lua` had it backwards for a while. Both scripts register on
DLT_USER0; the second registration is the one that stands. Naming ours last on
the command line is therefore the only arrangement that reads a saved MeshBench
capture with the MeshBench pseudo-header.

## Filters worth knowing

    msim.to_name == "West Lomond"              what one repeater heard
    msim.to_name == "West Lomond" && msim.outcome == 1
                                               ... and could not decode
    meshcore.path_hash_size == 3               3-byte path hashes
    meshcore.payload_type == 5                 group text - a channel message
    msim.snr < 0                               only just arrived
