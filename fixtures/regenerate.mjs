// Rebuild the shipped ScotMesh fixtures from live CoreScope.
//
// The fixtures under this directory are a snapshot of a real network, and a
// snapshot goes stale: nodes come and go, and the boundary that was chosen once
// can turn out to miss a place - which is what happened to Northern Ireland,
// left out because the study area was Scotland + the Republic of Ireland and NI
// is neither (MSIM #244). Rather than rebuild them by hand in the workbench,
// which is how they drifted from anything reproducible, this drives a headless
// workbench through the same steps.
//
// It is the peer of what a person does in the GUI, in the Node client:
//   1. choose the boundaries - here Scotland, Ireland AND Northern Ireland
//   2. import the nodes CoreScope has seen inside them
//   3. infer each node's regions from a week of traffic, and apply them
//   4. place one of every other node kind the fixtures carry
//   5. pin firmware, save the strict variant, then the permissive one
//
// Usage - with a headless workbench already listening (so its address is
// yours to name), and MESHBENCH pointing at a built binary:
//
//   meshbench headless -control-socket /tmp/mb.sock -quiet &
//   MB_SOCK=/tmp/mb.sock node fixtures/regenerate.mjs
//
// It writes the two fixtures to the workbench's projects directory; this script
// only prints where. The top-level study metadata a fixture also carries (the
// map view, the assertions) is added when they are copied in - see
// docs/fixtures.md.

import { Workbench } from "../pkg/client-js/meshbench.mjs";

const URL = "https://scotmesh-corescope.mm7roq.compute.oarc.uk";
const SOCK = process.env.MB_SOCK || "/tmp/mb.sock";
const AREAS = ["Scotland", "Ireland", "Northern Ireland"];
const PLACED = [
  { name: "Edinburgh Room Server", kind: "room-server", lat: 55.953, lon: -3.188 },
  { name: "Wicklow Advanced", kind: "advanced-repeater", lat: 53.006, lon: -6.366 },
  { name: "Glasgow SDR Observer", kind: "sdr-observer", lat: 55.864, lon: -4.252 },
  { name: "Dublin Interferer", kind: "emitter", lat: 53.35, lon: -6.26 },
];
const FIRMWARE = [
  ["simple_repeater", "repeater-v1.17.0"],
  ["companion_radio", "companion-v1.17.0"],
  ["simple_room_server", "room-server-v1.17.0"],
];

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const log = (...a) => console.log(...a);
const wb = await Workbench.attach({ socket: SOCK, timeoutMs: 600000 });
try {
  await wb.call("project.new", {});
  await wb.call("study.margin", { km: 30 });
  await wb.call("radio.preset", { preset: "EU/UK (Narrow)" });
  for (const q of AREAS) {
    const set = await wb.call("boundary.set", { query: q });
    if (!set.names || !set.names.length) throw new Error(`no boundary named ${q}`);
    await wb.call("boundary.accept", { name: set.names[0] });
  }
  await wb.call("import.set_source", { url: URL });
  await wb.call("import.fetch");
  log("imported", (await wb.call("import.commit", { strategy: "replace-all" })).nodes, "nodes");
  try { await wb.call("sim.seed", { seed: 9001 }); } catch { /* set once an engine exists */ }

  // Regions: the read runs on a goroutine, so poll infer.apply, which refuses
  // until the read is in and then applies.
  await wb.call("infer.run", { hours: 168 });
  let applied = 0;
  const deadline = Date.now() + 5 * 60 * 1000;
  while (Date.now() < deadline) {
    await sleep(3000);
    try { applied = (await wb.call("infer.apply")).applied; break; }
    catch (e) { if (!/nothing inferred/.test(e.message)) throw e; }
  }
  log("applied regions to", applied, "nodes");
  if (applied <= 0) throw new Error("no regions applied - is CoreScope reachable?");

  for (const n of PLACED) await wb.call("nodes.place", n);
  for (const [role, version] of FIRMWARE) await wb.call("firmware.set", { role, version });

  const strict = await wb.call("project.save", { name: "fixture-scotland-ireland-strict" });
  await wb.call("nodes.allow_flood", { on: true });
  const perm = await wb.call("project.save", { name: "fixture-scotland-ireland-permissive" });
  log("wrote", strict.path, "and", perm.path);
} finally { await wb.close(); }
