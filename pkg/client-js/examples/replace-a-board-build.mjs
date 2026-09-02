// Build a board image from its own repository and put the new one on a node.
//
//   WADAMESH=~/src/wadamesh node replace-a-board-build.mjs
//
// Run it again after a change and it reuses the session already open: pause,
// swap the firmware, delete the build it replaced, carry on. Needs a display,
// and leaves the window up on the node's Hardware tab so you can watch it boot.

import { spawnSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";

import {
  Board, Kind, MeshbenchError, NotFound, Role, Tab, Workbench,
} from "../meshbench.mjs";

// Every environment wadamesh defines ends in _touch. WADAMESH_ENV picks a
// different board.
const DEFAULT_PIO_ENV = "LilyGo_TDeck_companion_radio_touch";
const NODE = "Bench";

export async function main() {
  const repo = process.env.WADAMESH || path.join(os.homedir(), "src", "wadamesh");
  const pioEnv = process.env.WADAMESH_ENV || DEFAULT_PIO_ENV;
  const image = path.join(repo, ".pio", "build", pioEnv, "firmware.bin");

  const built = spawnSync("pio", ["run", "-e", pioEnv],
    { cwd: repo, stdio: "inherit" });
  if (built.status !== 0) throw new Error(`pio run -e ${pioEnv} failed in ${repo}`);

  // Not closed on the way out: close owns the process it started, and the point
  // here is to leave the window up.
  const wb = await Workbench.attachOrLaunch();

  // Pause first. Swapping firmware under a running clock stops the node while
  // its neighbours carry on transmitting, and nothing accounts for the gap.
  await wb.sim.pause();

  try {
    await wb.nodes.info(NODE);
  } catch (e) {
    if (!(e instanceof NotFound)) throw e;
    await wb.project.new("Fife");
    await wb.nodes.place({
      name: NODE, kind: Kind.COMPANION, lat: 56.20, lon: -3.20,
      board: Board.LILYGO_TDECK,
    });
  }
  const node = wb.node(NODE);
  const old = await node.build();

  // No label, so it is stamped with the time: two runs are two builds rather
  // than one quietly overwriting the other.
  const fresh = await wb.firmware.import(image, Role.COMPANION_RADIO,
    { board: Board.LILYGO_TDECK });
  await node.setFirmware(fresh); // stops it, provisions it, starts it
  await node.waitRunning();

  // Only now it is on the new one. A pin nothing can honour does not fail until
  // the node next starts.
  if (old && old.version !== fresh.version) await wb.firmware.delete(old);

  await wb.window(NODE, Tab.HARDWARE);
  await wb.sim.play();
  await wb.sim.run(30_000, { waitMs: 30 * 60_000 });
  console.log(`${NODE} is running ${fresh.version}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  // A refusal is a sentence, not a stack: an operator needs the workbench's own
  // words, which is what log.Fatal gives the Go examples and a traceback does not.
  process.exitCode = await main().catch((e) => {
    console.error(e instanceof MeshbenchError ? e.message : e);
    return 1;
  });
}
