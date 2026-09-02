// Two builds, two nodes, one scenario - the A/B.
//
//   node two-builds-in-one-scenario.mjs <stock version> <local build path>
//
// The most common real use of this API, and the reason the node window grew a
// firmware control: comparing a stock build against one with a single changed
// constant, on the same mesh, at the same seed.
//
// Needs a display: it opens the workbench so you can watch both arms run.

import { pathToFileURL } from "node:url";

import { MeshbenchError, Role, Workbench, describeBuild } from "../meshbench.mjs";

const SEED = 9001;

export async function main(argv = []) {
  const [stockVersion, localPath] = argv;
  if (!stockVersion || !localPath) {
    throw new Error(
      "usage: two-builds-in-one-scenario.mjs <stock version> <local build path>");
  }

  const wb = await Workbench.launch({ fixture: "fife-strict", seed: SEED });
  try {
    const stock = await wb.firmware.find(stockVersion);
    const changed = await wb.firmware.import(localPath, Role.SIMPLE_REPEATER);

    // Two nodes far enough apart to be independently interesting, one on each
    // build. Applied, which restarts each of them.
    const nodes = await wb.nodes.list();
    if (nodes.length < 2) {
      throw new Error("this scenario has fewer than two nodes to compare");
    }
    const a = wb.node(nodes[0].name);
    const b = wb.node(nodes[1].name);
    await a.setFirmware(stock);
    await b.setFirmware(changed);

    await wb.sim.start({ firmwareMs: 15 * 60_000 });
    await wb.sim.run(5 * 60_000, { waitMs: 60 * 60_000 });

    // Per node, because the whole point is which of the two behaved
    // differently - a total would hide it.
    console.log(String(await wb.provenance()));
    const stats = await wb.nodeStats();
    for (const { node, build } of [{ node: a, build: stock }, { node: b, build: changed }]) {
      const s = stats.find((row) => row.name === node.name);
      if (!s) continue;
      console.log(`${s.name.padEnd(24)} ${describeBuild(build).padEnd(32)} ` +
        `sent ${String(s.sent).padStart(4)}  heard ${String(s.heard).padStart(4)}`);
    }

    // One run of one seed is one draw. A difference here is a hypothesis, not a
    // result: vary the seed before believing anything.
    console.log("\none seed, one draw - vary the seed before calling this a difference");
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
