#!/usr/bin/env python3
"""A realistic soak: one message at a time, each flood allowed to finish.

The first attempt had five companions keying up on the same instant twelve
times over, which is a storm no mesh experiences, and then read only the last
two thousand events - the most congested five seconds of it. This sends the
way a mesh is actually used: one message, let it flood, let the air clear,
send the next. Every round's events are dumped before the ring can drop them.
"""
import json, os, socket, subprocess, sys, time

PATH = os.path.join(os.environ.get("XDG_RUNTIME_DIR", "/tmp"), "meshcoresim.sock")

def call(verb, params=None, timeout=300):
    s = socket.socket(socket.AF_UNIX)
    s.settimeout(timeout)
    s.connect(PATH)
    req = {"id": 1, "method": verb}
    if params is not None:
        req["params"] = params
    s.sendall((json.dumps(req) + "\n").encode())
    buf = b""
    while not buf.endswith(b"\n"):
        c = s.recv(1 << 20)
        if not c:
            break
        buf += c
    s.close()
    return json.loads(buf.decode())

COMPANIONS = ["Mothy", "Jazzy", "PeterB", "AngusOutlaw1", "CaptBlack-Mysteron1"]

def settle(limit=300):
    deadline = time.time() + limit
    while time.time() < deadline:
        st = call("sim.state")["result"]
        if not st["playing"]:
            return st
        time.sleep(1)
    return call("sim.state")["result"]

def main():
    rounds = int(sys.argv[1]) if len(sys.argv) > 1 else 15
    outdir = sys.argv[2] if len(sys.argv) > 2 else "/tmp/soakdump"
    os.makedirs(outdir, exist_ok=True)
    subprocess.run(f"rm -f {outdir}/*.ndjson", shell=True)

    live = []
    for c in COMPANIONS:
        r = call("companion.connect", {"node": c})
        if "result" in r or "already connected" in str(r.get("error", "")):
            live.append(c)
    print(f"connected: {live}", flush=True)

    for i in range(rounds):
        who = live[i % len(live)]
        if i % 4 == 0:
            call("companion.advert", {"node": who})
        else:
            call("companion.send", {"node": who, "text": f"soak {i}"})
        # One flood at a time: 20 s of air is forty packet-times at
        # SF8/62.5 kHz, long enough for a 58-node mesh to finish relaying.
        call("sim.run", {"for_ms": 20000})
        st = settle()
        d = call("events.dump", {"path": f"{outdir}/r{i:03d}.ndjson"})
        got = d.get("result", {})
        print(f"round {i+1}/{rounds} from {who}: t={st['now_ms']/1000:.0f}s "
              f"dumped={got.get('written')} total={got.get('total')}", flush=True)
    print("FINAL", flush=True)

if __name__ == "__main__":
    main()
