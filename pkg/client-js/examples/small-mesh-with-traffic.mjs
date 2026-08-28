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

const wb = await Workbench.attach();
try {
  await wb.hello(); // refuse a protocol this client does not speak

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

  await wb.call("sim.play");
  await wb.call("sim.run", { for_ms: 10 * 60_000 }); // ten minutes of mesh time

  const { total = 0, events = [] } = await wb.call("events.recent", { limit: 1000 });
  const received = events.filter((e) => e.class === "received");
  console.log(`${total} events, ${received.length} receptions in the tail`);
} finally {
  await wb.close();
}
