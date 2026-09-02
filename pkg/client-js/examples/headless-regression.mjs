// The one CI runs.
//
//   node headless-regression.mjs [fixture] [junit.xml]
//
// No display, no GPU, no toolkit. Opens a fixture, runs it, checks its
// assertions, writes JUnit, and exits non-zero if the mesh stopped delivering.
// This is the shape a MeshCore pull request would use.
//
// Costs as long as the fixture asks for. fife-strict at five simulated minutes
// is a couple of minutes of wall clock with no firmware, and considerably more
// with it.

import { pathToFileURL } from "node:url";

import { MeshbenchError, Workbench } from "../meshbench.mjs";

const SEED = 9001;

export async function main(argv = []) {
  const fixture = argv[0] || "fife-strict";
  const junit = argv[1] || "";

  const wb = await Workbench.headless({ fixture, seed: SEED });
  try {
    // Bring the mesh up before running the clock. sim.run only advances time;
    // without this the firmware never starts, nothing transmits, and the run
    // reports every assertion failed on a tree with nothing wrong with it -
    // which is the worst thing a regression check can do.
    await wb.sim.start();
    await wb.firmware.waitStarted();

    await wb.sim.run(5 * 60_000, { waitMs: 60 * 60_000 });

    const report = await wb.assertions.check();
    // The report prints the caveats above the numbers itself, because this is
    // the output somebody pastes into a pull request and the caveats are the
    // half that gets dropped.
    console.log(String(report));
    console.log(`${await wb.events.total()} events`);

    if (junit) report.writeJUnit(junit);

    // A fixture with no assertions can report but cannot pass or fail, and a
    // green tick that checked nothing is the worst outcome available here.
    if (report.total === 0) return 2;
    return report.ok ? 0 : 1;
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
