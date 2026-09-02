// A blank setup, one companion, and its screen on show.
//
//   node blank-setup-with-a-board.mjs
//
// wadamesh is imported, not downloaded: it must be in the library or reachable
// through WADAMESH_IMAGE. Needs a display. It opens the node's own window on
// the Hardware tab at the end, which is the point of it.

import { pathToFileURL } from "node:url";

import {
  Board, Kind, MeshbenchError, NotFound, Role, Tab, Workbench, describeBuild,
} from "../meshbench.mjs";

const WADAMESH = "wadamesh";
const BOARD = Board.LILYGO_TDECK;

export async function main() {
  const wb = await Workbench.launch();
  try {
    await wb.project.new("Fife");
    const deck = await wb.nodes.place({
      name: "Deck", kind: Kind.COMPANION, lat: 56.19, lon: -3.17, board: BOARD,
    });

    // Whatever the catalogue has, so this does not go stale against a version
    // number typed here.
    await wb.firmware.scan();
    let build;
    try {
      build = await wb.firmware.find(WADAMESH, BOARD);
    } catch (e) {
      if (!(e instanceof NotFound)) throw e;
      // wadamesh is imported, not in the download catalogue - import a built
      // image, named by WADAMESH_IMAGE.
      const image = process.env.WADAMESH_IMAGE;
      if (!image) {
        throw new Error(`${WADAMESH} is not in the library; set WADAMESH_IMAGE ` +
          "to a built image, or import one in the workbench first");
      }
      build = await wb.firmware.import(image, Role.COMPANION_RADIO_USB,
        { board: BOARD, label: WADAMESH });
    }

    // Applied: stop, provision, start. On a board that means an emulator, which
    // is why the wait below is generous.
    await deck.setFirmware(build);
    await wb.sim.start();
    await deck.waitRunning(5 * 60_000);

    // The Hardware tab is where the board draws its own screen, which is the
    // whole reason for making this node a T-Deck.
    const tab = await wb.window(deck, Tab.HARDWARE);

    console.log(`${deck.name} is up on ${describeBuild(build)}; ` +
      `its window is open on ${tab}`);
    console.log(String(await wb.provenance()));
    // Held open for somebody looking at it, and only then. Piped or run from CI
    // there is nobody to press enter, so say which happened rather than
    // appearing to have been dismissed.
    if (process.stdin.isTTY) {
      console.log("press enter to close the workbench");
      await new Promise((r) => process.stdin.once("data", r));
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
