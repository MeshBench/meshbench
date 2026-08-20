#!/usr/bin/env python3
"""Judge a dumped soak against the real ScotMesh receptions."""
import collections, glob, json, sys

def q(xs, p):
    xs = sorted(xs)
    return xs[min(int(p * (len(xs) - 1)), len(xs) - 1)]

def main():
    outdir = sys.argv[1] if len(sys.argv) > 1 else "/tmp/soakdump"
    seen, rows = set(), []
    for path in sorted(glob.glob(f"{outdir}/*.ndjson")):
        for line in open(path):
            line = line.strip()
            if not line:
                continue
            r = json.loads(line)
            # Rounds overlap in the ring; a packet-receiver-instant is one
            # event however many dumps caught it.
            k = (r.get("at_ms"), r.get("kind"), r.get("from"), r.get("to"),
                 r.get("packet_id"))
            if k in seen:
                continue
            seen.add(k)
            rows.append(r)
    if not rows:
        print("no events dumped")
        return 1

    kinds = collections.Counter(r.get("kind") for r in rows)
    rx = [r for r in rows if r.get("kind") == "rx"]
    tx = [r for r in rows if r.get("kind") == "tx"]
    snrs = [r["snr_db"] for r in rx if r.get("snr_db") is not None]
    span = (max(r["at_ms"] for r in rows) - min(r["at_ms"] for r in rows)) / 1000 or 1

    print(f"=== SOAK: {len(rows)} distinct events over {span:.0f} s of simulated air")
    print(f"    {dict(kinds)}")
    print(f"\nMODEL, {len(snrs)} receptions")
    print(f"  SNR   min {min(snrs):+.1f}  p10 {q(snrs,.10):+.1f}  p50 {q(snrs,.50):+.1f}"
          f"  p90 {q(snrs,.90):+.1f}  max {max(snrs):+.1f}")
    over15 = sum(1 for s in snrs if s > 15.0001)
    print(f"  above +15 dB: {over15}   above +20 dB: {sum(1 for s in snrs if s > 20)}")
    print(f"  transmissions {len(tx)}  =  {len(tx)/span:.2f}/s across the mesh")
    print(f"  receptions per transmission: {len(rx)/max(len(tx),1):.1f}")

    print("\nREAL ScotMesh, 1,992 receptions")
    print("  SNR   min -13.5  p10  -5.5  p50  +5.0  p90 +13.0  max +15.0")
    print("  above +15 dB: 0     above +20 dB: 0")

    byto = collections.defaultdict(list)
    for r in rx:
        byto[r["to"]].append(r)
    doubles = sum(1 for evs in byto.values()
                  for c in collections.Counter(e["at_ms"] for e in evs).values() if c > 1)

    print("\n=== VERDICTS ===")
    ok = True
    def check(name, passed, detail):
        nonlocal ok
        ok = ok and passed
        print(f"  [{'PASS' if passed else 'FAIL'}] {name}: {detail}")

    check("one demodulator", doubles == 0,
          f"{doubles} receivers decoded two packets in the same instant")
    check("reporting ceiling", over15 == 0,
          f"{over15} receptions above the +15 dB a modem can report")
    check("the mesh is alive", len(snrs) > 500, f"{len(snrs)} receptions")
    check("median plausible", -5 <= q(snrs, .50) <= 15,
          f"p50 {q(snrs,.50):+.1f} dB against the real +5.0")
    check("floods thin out", len(rx)/max(len(tx),1) < 20,
          f"{len(rx)/max(len(tx),1):.1f} receptions per transmission")

    miss = collections.Counter((r.get("detail") or "")[:52]
                               for r in rows if r.get("kind") == "miss")
    print("\ntop miss causes:")
    for d, c in miss.most_common(7):
        print(f"  {c:6d}  {d}")
    return 0 if ok else 1

if __name__ == "__main__":
    sys.exit(main())
