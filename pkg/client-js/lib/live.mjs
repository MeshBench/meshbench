// Bringing a real deployment in from a live feed.
//
// Four steps in a fixed order, and every one of them has been skipped by
// somebody at least once. The two that get missed are the last two, and missing
// them does not fail: the mesh comes up with regions inferred but never
// applied, which transmits everything, relays nothing, and reports no error at
// all. It reads as bad RF.
//
// So the steps are here individually, because sometimes you want to look at a
// preview before committing - and `pull` runs all four, because the ordinary
// case is wanting the whole deployment and the ordinary mistake is stopping
// early.

import { MeshbenchError } from "./errors.mjs";
import { Strategy } from "./sets.mjs";
import { ImportPreview } from "./values.mjs";
import { JOB_WAIT_MS } from "./wait.mjs";

/** How far back to read traffic when working out what each node holds.
 *
 *  A week, because that is what it takes for the quiet regions to say anything
 *  at all: on ScotMesh a small region is about sixty packets in seven days, and
 *  a shorter window drops it entirely rather than reporting it as thin. */
export const DEFAULT_WINDOW_HOURS = 7 * 24;

/** A live feed, and the deployment it describes. Live in both senses. */
export class Live {
  constructor(wb) { this._wb = wb; }

  /** Fetch, commit, read the traffic, and apply what it implies.
   *
   *  The whole chain, in the order that works. `windowHours` is how far back
   *  into the feed's history to read - the mesh's own past, not your patience;
   *  `waitMs` is yours.
   *
   *  Returns what the fetch found. Link measurement is still running when this
   *  returns on anything but a small mesh, so follow it with `wb.waitIdle()`
   *  before starting a run. */
  async pull(url, { strategy = Strategy.REPLACE,
    windowHours = DEFAULT_WINDOW_HOURS, waitMs = JOB_WAIT_MS } = {}) {
    const preview = await this.fetch(url);
    if (preview.nodes === 0) {
      throw new MeshbenchError(
        `${url} described ${preview.records} nodes, none usable`);
    }
    await this.commit(strategy);
    await this.infer({ windowHours, waitMs });
    await this.applyRegions();
    return preview;
  }

  /** Point at a feed without reading it, and say how the URL was tidied.
   *
   *  A method rather than a property, because a property implies something to
   *  read back and the session offers no way to ask what its source currently
   *  is. One that answered from a value this object happened to remember would
   *  be right until anything else set it. */
  async setSource(url) {
    return ((await this._wb.call("import.set_source", { url })) || {}).url || url;
  }

  /** Read the deployment and say what would change, changing nothing. */
  async fetch(url = "") {
    if (url) await this.setSource(url);
    return new ImportPreview((await this._wb.call("import.fetch")) || {});
  }

  /** Apply the fetched nodes to the scenario, and say how many it now holds.
   *
   *  "replace-all" is what the shipped fixtures were built with; "add" keeps
   *  what is already here and skips names that clash.
   *
   *  Measuring the links afterwards is a job rather than part of this call - 676
   *  nodes is 228,000 terrain paths over real ground - so this returns while
   *  that is still running. */
  async commit(strategy = Strategy.REPLACE) {
    return ((await this._wb.call("import.commit", { strategy })) || {}).nodes || 0;
  }

  /** Read the feed's recent traffic to work out what each node holds.
   *
   *  This is the step that decides whether anything relays. A node whose regions
   *  are unknown forwards nothing, and nothing says so.
   *
   *  `windowHours` is the feed's own past; `waitMs` is how long you will sit
   *  here for it. A week of ScotMesh is around 150,000 packets and several
   *  minutes of paging. */
  async infer({ windowHours = DEFAULT_WINDOW_HOURS, waitMs = JOB_WAIT_MS } = {}) {
    if (!(windowHours > 0)) {
      throw new MeshbenchError("infer() needs a window, in hours of the feed's past");
    }
    await this._wb.call("infer.run", { hours: windowHours });
    await this._wb.job("infer").wait(waitMs);
  }

  /** Put the inferred regions onto the nodes, and say how many took one.
   *
   *  The forgotten step. Everything above can succeed and the mesh still be
   *  silent until this runs. */
  async applyRegions() {
    return ((await this._wb.call("infer.apply")) || {}).applied || 0;
  }
}
