// A running board, as something a script can look at and prod.
//
// Read what the display is showing, capture it as an image, press the buttons,
// type at the keyboard, touch the panel. All of it works headless - the display
// is the framebuffer the controller holds, not a picture of anybody's desktop -
// which is the point: a board test that needs a screen in front of it does not
// run in CI.

import { MeshbenchError } from "./errors.mjs";

/** How long to wait for the screen to change before giving up, when a caller
 *  names no timeout of its own. */
export const SCREEN_WAIT_MS = 30_000;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/** One running node's board, as a device to drive. A handle, not a copy. */
export class Device {
  constructor(wb, name) {
    this._wb = wb;
    this.name = name;
  }

  // ---- looking ---------------------------------------------------------

  /** What the display is showing, as numbers rather than a picture.
   *
   *  Enough to answer "did anything change" after a press or a touch, which is
   *  what every check of an input comes down to; for the picture itself, ask for
   *  a screenshot. `digest` identifies the frame - two screens with the same one
   *  are the same picture, which `lit` cannot promise. */
  async screen() {
    return (await this._wb.call("board.screen", { node: this.name })) || {};
  }

  /** Write the display to a PNG and say where it landed. The frame is exactly
   *  what the controller holds, at the size it holds it. */
  async screenshot() {
    return (await this._wb.call("board.screenshot", { node: this.name })) || {};
  }

  // ---- prodding --------------------------------------------------------

  /** Hold a button pin down, or release it.
   *
   *  Held rather than clicked because the firmware cares: MeshCore wakes a
   *  sleeping display on a press and powers the board off on a long one, so a
   *  caller times the release itself - or uses `tap`, which does not hold. */
  async press(pin, down = true) {
    await this._wb.call("board.press", { node: this.name, pin, down });
  }

  /** Press a button and let go - the ordinary click. */
  async tap(pin) {
    await this.press(pin, true);
    await this.press(pin, false);
  }

  /** Enter text at the board's own keyboard, one character at a time - which is
   *  what the keyboard sends, and what the firmware polls for. */
  async type(text) {
    await this._wb.call("board.key", { node: this.name, text });
  }

  /** Put a finger on the panel at a point, or lift it off. */
  async touch(x, y, down = true) {
    await this._wb.call("board.touch", { node: this.name, x, y, down });
  }

  /** Touch a point and lift off - a tap on the panel. */
  async tapAt(x, y) {
    await this.touch(x, y, true);
    await this.touch(x, y, false);
  }

  // ---- waiting ---------------------------------------------------------

  /** Wait until the display changes from what it shows now and return the new
   *  frame, or fail with what it was still showing when the time ran out.
   *
   *  This is the honest way to check an input. Half duplex eats stimuli - a
   *  board handed a packet while transmitting never hears it - so a tap followed
   *  by an immediate screen read will intermittently read the frame from before
   *  the tap landed. Change is by digest, so a redraw that keeps the same number
   *  of lit pixels still counts. */
  async waitScreen(timeoutMs = SCREEN_WAIT_MS) {
    const before = await this.screen();
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      await sleep(50);
      const now = await this.screen();
      if (now.digest !== before.digest) return now;
    }
    throw new MeshbenchError(
      `board ${this.name}: the screen did not change within ${timeoutMs} ms`);
  }
}
