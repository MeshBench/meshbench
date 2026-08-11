# Watching the traffic

A capture here carries something no real one can: **every receiver's view of
every frame**. The same transmission appears once per node that could have heard
it, with what that node actually made of it. A packet node A heard and node B
did not is the most informative event in a mesh, and no single radio can record
it.

## Live

**Simulation → capture live to Wireshark**, then *open Wireshark* on the status
line. That starts the capture, opens Wireshark on loopback filtered to our port,
and loads both dissectors.

It streams as UDP datagrams to `127.0.0.1:5555`. Datagrams have no history, so
Wireshark can be started, stopped and restarted mid-run and simply picks up from
the next packet — unlike a pcapng stream, which carries its header once and
shows a later arrival precisely nothing.

Nothing binds that port. Wireshark *sniffs* the interface; the simulator fires
and forgets.

### If Wireshark cannot capture

`dumpcap` usually ships `root:wireshark` with no execute bit for anyone else, so
Wireshark reports "Permission denied" against a helper you never asked for. The
proper fix, once:

    sudo usermod -aG wireshark $USER      # then log out and back in

## Saved

**Simulation → capture to pcapng...** writes the same thing to a file, which
needs no permissions at all.

## Reading it

Two layers:

* **MeshBench capture** — what only a simulator knows: the receiving node, the
  outcome, the true RSSI and SNR, the radio settings, and node *names*.
* **MeshCore Protocol** — the wire format, dissected by
  [aaronb/wireshark-meshcore](https://github.com/aaronb/wireshark-meshcore).

The packet list is set up to show the first: from, received by, outcome, SNR,
hops, hash size. Nothing in it is about IP.

### Filters worth knowing

    msim.to_name == "West Lomond"          what one repeater heard
    msim.to_name == "West Lomond" && msim.outcome == 1
                                           ... and could not decode
    msim.outcome == 3                      dropped as a duplicate
    meshcore.payload_type == 5             group text - a channel message
    meshcore.payload_type == 4             adverts
    meshcore.path_hash_size == 3           3-byte path hashes
    meshcore.hops > 5                      far from home
    msim.snr < 0                           only just arrived

`msim.outcome` is the interesting one. **out of range** and **not demodulated**
are physics; **dropped by firmware** is the duplicate table, and a flood that
dies of that is a configuration problem wearing an RF disguise.

### The path length byte is not a byte count

Its top two bits are the hash size minus one; only the low six are the hop
count. A 20-hop path with 3-byte hashes is 60 bytes, and reading the byte as a
length mis-parses every packet whose hashes are wider than one. Both dissectors
get this right; anything else reading MeshCore frames should be checked.
