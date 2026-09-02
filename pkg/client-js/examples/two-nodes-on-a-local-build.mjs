// A fixture trimmed to two, both on a build from a MeshCore checkout - and
// re-runnable without clearing anything down.
//
//   node two-nodes-on-a-local-build.mjs ~/src/MeshCore
//
// The interesting half is the second run. It attaches to the workbench the
// first one left, stops the clock, rebuilds, repoints the nodes and starts
// again, rather than opening a fresh session and paying for the fixture twice.

import { pathToFileURL } from "node:url";

import { Kind, MeshbenchError, Role, Workbench } from "../meshbench.mjs";

// Outskirts of Glasgow, and Glenrothes.
const KEEP = {
  "Glasgow-Outskirts": [55.8720, -4.3300],
  "Glenrothes": [56.1980, -3.1780],
};

export async function main(argv = []) {
  const checkout = argv[0];
  if (!checkout) {
    throw new Error("usage: two-nodes-on-a-local-build.mjs <path to MeshCore>");
  }

  // The one the last run left, or a new one.
  const wb = await Workbench.attachOrLaunch();
  try {
    // Stop the clock before anything else: a no-op on a fresh session, and the
    // thing that makes the second run safe on a live one.
    await wb.sim.pause();

    const want = Object.keys(KEEP).sort();
    let nodes = await wb.nodes.list();
    // Whether the mesh is already the one this example is about, not whether
    // the session is empty. A launched workbench is never empty: it opens its
    // own default fixture, which is 311 nodes - so "is it empty" was always
    // false, the trim never ran, and this put a local build on a national
    // network and reported it as though that had been the plan.
    const already = nodes.length === want.length &&
      nodes.every((n) => n.name in KEEP);
    if (!already) {
      await wb.project.open("fife-strict");
      // Put them where they belong first, then delete the rest. keep is
      // all-or-none by design, so naming a node that is not there yet refuses
      // and removes nothing - and one of these two is never in the fixture.
      const have = await wb.nodes.list();
      for (const name of want) {
        const [lat, lon] = KEEP[name];
        if (have.some((n) => n.name === name)) {
          await wb.node(name).move(lat, lon);
          continue;
        }
        await wb.nodes.place({ name, kind: Kind.COMPANION, lat, lon });
      }
      await wb.nodes.keep(...want);
      await wb.waitIdle(10 * 60_000);
    }

    // Both roles from one build, deliberately. A locally built repeater
    // compiled against a stale shim once answered console output with 0x06
    // where the host expects 0x07: it connected, misbehaved and exited. Two
    // arms built at different moments measure the build process, not the
    // firmware.
    const built = await wb.firmware.build(checkout, { waitMs: 30 * 60_000 });
    if (built.length === 0) {
      throw new Error("the build produced nothing the library can see");
    }

    nodes = await wb.nodes.list();
    for (const n of nodes) {
      const role = n.kind === Kind.COMPANION
        ? Role.COMPANION_RADIO : Role.SIMPLE_REPEATER;
      const build = built.find((b) => b.role === role);
      if (build) await wb.node(n.name).setFirmware(build);
    }

    await wb.sim.start({ firmwareMs: 10 * 60_000 });
    console.log(`${nodes.length} nodes on a build from ${checkout}`);
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
