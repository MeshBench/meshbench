// The few values this client builds rather than passes through.
//
// Everything the socket answers with reaches a script as the object the
// workbench sent, keys and all: a translation layer that renamed `height_m` to
// `heightM` would be a second vocabulary to keep in step with the wire, and it
// would go stale the week a verb grew a field. What is here instead is the
// handful of values that carry behaviour a caller would otherwise write out -
// the caveats that must travel with a number, and the two names a build has.

/** What a measurement was measured under.
 *
 *  Carried with any result that is a number about the world, because a scripted
 *  number gets pasted into a report with the caveats stripped. The caveats have
 *  to be in the value. */
export class Provenance {
  constructor({ rf_mode = "", excess_loss_db = 0, calibrated = false, seed = 0 } = {}) {
    /** "calculated" or "waveform". */
    this.rfMode = rf_mode;
    /** The calibration term in force, and whether it was fitted against real
     *  receptions rather than left at the default. */
    this.excessLossDb = excess_loss_db;
    this.calibrated = calibrated;
    this.seed = seed;
  }

  /** One line, meant to be printed above any number a script emits. */
  toString() {
    const fit = this.calibrated
      ? "excess loss fitted to real receptions"
      : "default excess loss";
    return `MeshBench: ${this.rfMode} reception, ${fit} - a best case; ` +
      "no multipath, no body loss, no oscillator error";
  }
}

/** What a fetch found, before anything has been changed.
 *
 *  `skipped_no_position` and `uncertain` are the two worth reading before
 *  committing. A node with no position cannot be simulated at all, and an
 *  uncertain one is being placed to within kilometres - the answer it gives is
 *  that vague too, however confident the rest of the output looks. */
export class ImportPreview {
  constructor(raw = {}) {
    this.records = raw.records || 0;
    this.nodes = raw.nodes || 0;
    this.skippedNoPosition = raw.skipped_no_position || 0;
    this.uncertain = raw.uncertain || 0;
  }

  toString() {
    let out = `${this.records} records, ${this.nodes} usable`;
    if (this.skippedNoPosition) out += `, ${this.skippedNoPosition} with no position`;
    if (this.uncertain) out += `, ${this.uncertain} placed only roughly`;
    return out;
  }
}

/** How a build is named where a person will read it.
 *
 *  Version, board and role travel together because a board image is not a build
 *  on its own: "wadamesh" means nothing until it is wadamesh for a LilyGo_TDeck,
 *  built as a companion. A host build carries neither of the other two. */
export function describeBuild(build) {
  if (!build || !build.board) return (build && build.version) || "";
  return `${build.board} - ${build.role} ${build.version}`;
}
