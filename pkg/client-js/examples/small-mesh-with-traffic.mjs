// Example 3 from #209, in Node: two repeaters, two companions, one of them a
// T-Deck, and a message to the public channel every twenty seconds.
//
//   node small-mesh-with-traffic.mjs
//
// Needs a display - it opens the workbench so you can watch the traffic move -
// and `meshbench` on PATH, or MESHBENCH_BINARY naming one, with a workbench
// already running (start one, or open the app). The Go and Python clients ship
// the same example; this is the peer, done idiomatically in Node.

import { Workbench } from "../meshbench.mjs";

const MESH = [
  { name: "R1", kind: "simple-repeater", lat: 56.2, lon: -3.2 },
  { name: "R2", kind: "simple-repeater", lat: 56.12, lon: -3.02 },
  { name: "C1", kind: "companion", lat: 56.19, lon: -3.17 },
  { name: "C2", kind: "companion", lat: 56.09, lon: -3.1 },
];

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// sim.play only starts the clock; it does not bring firmware up on its own,
// and it starts firmware only when *no* node is running at all. A script
// that called sim.play straight after placing nodes would run a channel with
// nothing behind it: ten minutes would pass, nothing would transmit, and the
// summary at the end would print zero receptions and look like a clean exit.
// So this does the three things the workbench's own play button relies on,
// in order, and checks each one - the same sequence pkg/client-go's
// Sim().Start() and pkg/client-python's Workbench.headless() wait for.
async function waitIdle(wb, timeoutMs = 5 * 60_000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const snap = await wb.call("session.snapshot");
    const running = (snap.jobs || []).filter((j) => !j.finished);
    if (running.length === 0) return;
    if (Date.now() > deadline) {
      throw new Error(
        `the workbench did not go idle within ${timeoutMs} ms; still running: ` +
        running.map((j) => j.what).join(", "));
    }
    await sleep(200);
  }
}

async function waitFirmwareStarted(wb, timeoutMs = 10 * 60_000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const st = await wb.call("firmware.state");
    if (!st.starting && st.nodes > 0 && st.running >= st.nodes) return;
    if (Date.now() > deadline) {
      throw new Error(
        `firmware did not come up within ${timeoutMs} ms; ` +
        `${st.running} of ${st.nodes} running`);
    }
    await sleep(200);
  }
}

async function waitRunFinished(wb, timeoutMs = 5 * 60_000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const st = await wb.call("sim.state");
    if (!st.playing) return;
    if (Date.now() > deadline) {
      throw new Error(
        `the run did not finish within ${timeoutMs} ms; ` +
        `${st.now_ms} ms of simulated time`);
    }
    await sleep(200);
  }
}

const wb = await Workbench.attach(); // the handshake happens inside attach()
try {
  await wb.call("project.new", { place: "Fife" });
  for (const node of MESH) await wb.call("nodes.place", node);

  // C1 is the T-Deck. The board goes on before the firmware is pinned: a host
  // image is not a board image, and setting the board clears a pin made for
  // different hardware.
  await wb.call("node.set_board", { node: "C1", board: "LilyGo_TDeck" });

  // Pin every role that needs one to the newest build this machine holds,
  // rather than a version typed here that goes stale - the same thing the
  // Python client's use_what_is_here() does, written out.
  const { builds = [] } = await wb.call("firmware.library");
  const onDisk = builds.filter((b) => b.on_disk && !b.board);
  const { roles = [] } = await wb.call("firmware.needed");
  for (const row of roles) {
    const build = onDisk.find((b) => b.role === row.role);
    if (!build) throw new Error(`no ${row.role} build on this machine: meshbench firmware download ${row.role}`);
    await wb.call("firmware.set", { role: row.role, version: build.version });
  }

  // Every twenty seconds, from the plain companion to the public channel.
  // Simulated seconds - the mesh's own clock, not yours.
  await wb.call("schedule.add", {
    node: "C2",
    command: "public hello",
    at_ms: 5_000,
    every_ms: 20_000,
  });

  // Bring the mesh up before running the clock: wait out the link warm, start
  // whatever firmware is not already up, then play by its own name.
  await waitIdle(wb);
  const before = await wb.call("firmware.state");
  if (before.running < before.nodes) {
    await wb.call("firmware.start");
    await waitFirmwareStarted(wb);
  }
  const state = await wb.call("sim.state");
  if (!state.playing) await wb.call("sim.play");

  await wb.call("sim.run", { for_ms: 10 * 60_000 }); // ten minutes of mesh time
  await waitRunFinished(wb);

  const { total = 0, events = [] } = await wb.call("events.recent", { limit: 1000 });
  const received = events.filter((e) => e.class === "received");
  console.log(`${total} events, ${received.length} receptions in the tail`);

  // A script that reports success on a run that heard nothing is worse than
  // one that fails: it looks like a clean pass on a tree with something
  // wrong. Ten minutes of mesh time from a companion posting every twenty
  // seconds should produce receptions on the repeaters; zero means the run
  // is broken, not that the mesh was quiet.
  if (received.length === 0) {
    throw new Error("no receptions in ten minutes of simulated time - the mesh produced no traffic");
  }
} finally {
  await wb.close();
}
