// Two repeaters, two companions, one of them a T-Deck, and a message to the
// public channel every twenty seconds.
//
//   node small-mesh-with-traffic.mjs
//
// Needs a display: it opens the workbench so you can watch the traffic move.
// Costs about ten minutes of simulated time, and a few of yours.
//
// The repeating traffic needed no new verb: schedule.add has taken every_ms all
// along and nothing said so, which to somebody writing a script is the same as
// it not existing.

import { pathToFileURL } from "node:url";

import { Board, Class, Kind, MeshbenchError, Workbench } from "../meshbench.mjs";

const MESH = [
  { name: "R1", kind: Kind.SIMPLE_REPEATER, lat: 56.20, lon: -3.20 },
  { name: "R2", kind: Kind.SIMPLE_REPEATER, lat: 56.12, lon: -3.02 },
  { name: "C1", kind: Kind.COMPANION, lat: 56.19, lon: -3.17 },
  { name: "C2", kind: Kind.COMPANION, lat: 56.09, lon: -3.10 },
];

export async function main() {
  const wb = await Workbench.launch();
  try {
    await wb.project.new("Fife");
    await wb.nodes.placeMany(MESH);
    await wb.waitIdle(10 * 60_000);

    // C1 is the T-Deck. The board goes on before the firmware is pinned,
    // because a host image is not a board image and setting the board clears a
    // pin made for different hardware.
    await wb.node("C1").setBoard(Board.LILYGO_TDECK);

    // Whatever this machine holds for each role that needs one, rather than a
    // version typed here that goes stale.
    await wb.firmware.useWhatIsHere();

    // Every twenty seconds, from the plain companion to the public channel.
    // Simulated seconds - the mesh's own clock, not yours.
    await wb.schedule.add({
      node: "C2", command: "public hello", atMs: 5_000, everyMs: 20_000,
    });

    await wb.sim.start();
    await wb.sim.run(10 * 60_000, { waitMs: 60 * 60_000 });

    const events = await wb.events.recent(1000);
    const received = events.filter((e) => e.class === Class.RECEIVED).length;
    console.log(String(await wb.provenance()));
    console.log(`${await wb.events.total()} events, ${received} receptions in the tail`);

    // A script that reports success on a run that heard nothing is worse than
    // one that fails: it looks like a clean pass on a tree with something
    // wrong. Ten minutes of mesh time from a companion posting every twenty
    // seconds should produce receptions; zero means the run is broken, not that
    // the mesh was quiet.
    if (received === 0) {
      throw new Error(
        "no receptions in ten minutes of simulated time - the mesh produced no traffic");
    }
  } finally {
    await wb.close();
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  // A refusal is a sentence, not a stack: an operator needs the workbench's own
  // words, which is what log.Fatal gives the Go examples and a traceback does not.
  process.exitCode = await main().catch((e) => {
    console.error(e instanceof MeshbenchError ? e.message : e);
    return 1;
  });
}
