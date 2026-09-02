// Import a real mesh from its live feed, find one node, and make it advert.
//
//   node live-import-and-advert.mjs [area] [node]
//   node live-import-and-advert.mjs Fife
//   node live-import-and-advert.mjs bounds/tay-catchment.geojson
//
// The area is a place name or a path to GeoJSON, and it is set before the
// import because the import filters at fetch time - the whole feed is around
// 676 nodes and this is how you study a corner of it.
//
// The node is searched for rather than typed: the names on a real mesh carry
// emoji, so "West Lomond" is really "🏔️ West Lomond 📡".
//
// Needs the network.

import { pathToFileURL } from "node:url";

import { Class, MeshbenchError, Workbench } from "../meshbench.mjs";

const FEED = "https://scotmesh-corescope.mm7roq.compute.oarc.uk";

export async function main(argv = []) {
  const area = argv[0] || "Fife";
  const wanted = argv[1] || "West Lomond";

  const wb = await Workbench.launch();
  try {
    const studying = await wb.boundary.use(area);
    console.log("studying", studying.join(", "));

    // Fetch the nodes, commit them, read a week of traffic, and apply the
    // regions it implies. Skip that last step and nothing ever relays.
    const found = await wb.live.pull(FEED);
    console.log(String(found));
    await wb.waitIdle(60 * 60_000);

    const node = await wb.nodes.find(wanted);
    const all = await wb.nodes.list();
    console.log(`${wanted} is "${node.name}", one of ${all.length}`);

    await wb.firmware.useWhatIsHere();
    await wb.sim.start();

    // ask, rather than send and then read: reading straight after sending reads
    // the moment before the command went out.
    console.log(await node.console.ask("advert", 100));
    await wb.sim.run(2 * 60_000, { waitMs: 60 * 60_000 });

    const heard = new Set();
    for (const e of await wb.events.recent(2000)) {
      if (e.class === Class.RECEIVED && e.from === node.name) heard.add(e.to);
    }
    console.log(`${heard.size} of ${all.length - 1} others heard it directly`);
    console.log(String(await wb.provenance()));
  } finally {
    await wb.close();
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  // A refusal is a sentence, not a stack: an operator needs the workbench's own
  // words, which is what log.Fatal gives the Go examples and a traceback does not.
  process.exitCode = await main(process.argv.slice(2)).catch((e) => {
    console.error(e instanceof MeshbenchError ? e.message : e);
    return 1;
  });
}
