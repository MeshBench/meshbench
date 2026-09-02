// The network: what is in it, and what one node can be asked to do.

import { NotFound } from "./errors.mjs";
import { Device } from "./device.mjs";
import { Kind, Transport } from "./sets.mjs";
import { FIRMWARE_WAIT_MS, waitFor } from "./wait.mjs";

/** The score below which `find` will not act on a top answer.
 *
 *  Taking the top result unconditionally is how a script ends up sending an
 *  advert from a node that merely shared a word with what was asked for, and it
 *  does that silently. */
export const FIND_LEAST = 0.5;

/** Names, whether handles or strings were passed.
 *
 *  `search` and `near` hand back handles and every verb takes names, so without
 *  this each caller writes the same map - and the one that forgets sends an
 *  object down the socket and is told there is no node named "[object
 *  Object]". */
const names = (ns) => ns.map((n) => String(n));

/** The collection. Live: every call reads the session. */
export class Nodes {
  constructor(wb) { this._wb = wb; }

  /** Every node, as the network currently has them. */
  async list() {
    return ((await this._wb.call("nodes.list")) || {}).nodes || [];
  }

  /** How many there are. */
  async count() { return (await this.list()).length; }

  /** One by name. */
  async info(name) {
    for (const n of await this.list()) if (n.name === String(name)) return n;
    throw new NotFound("nodes.list", `no node named "${name}"`, "not_found");
  }

  /** Whether the network holds one by that name. */
  async has(name) {
    return (await this.list()).some((n) => n.name === String(name));
  }

  /** A handle, checked: a typo fails here rather than three calls later. */
  async get(name) {
    await this.info(name);
    return new Node(this._wb, String(name));
  }

  /** Find nodes by name, best first, when you cannot type the name.
   *
   *  Imported names carry emoji and accents - "\u{1F3D4}️ West Lomond \u{1F4E1}"
   *  is one real node - so matching is done on letters and digits alone, with
   *  accents folded and word order ignored. The ranking happens at the workbench
   *  rather than here, so all three clients agree about which result is the top
   *  one.
   *
   *  An empty result is not an error: "nothing matched" is an answer, and the
   *  caller usually wants to widen the query rather than handle a refusal. */
  async search(query, limit = 10) {
    const got = await this._wb.call("nodes.search", { query, limit });
    return (got || {}).matches || [];
  }

  /** The one node a search meant, or a refusal naming what it did find. */
  async find(query, least = FIND_LEAST) {
    const matches = await this.search(query, 5);
    if (matches.length === 0 || matches[0].score < least) {
      const near = matches.slice(0, 3)
        .map((m) => `"${m.name}" (${m.score.toFixed(2)})`).join(", ");
      throw new NotFound("nodes.search",
        `nothing matches "${query}" well enough` +
        (near ? `; nearest were ${near}` : ""), "not_found");
    }
    return new Node(this._wb, matches[0].name);
  }

  /** The nodes closest to this one, nearest first, at most `count` of them (all
   *  of them when it is zero).
   *
   *  Trimming an imported deployment to a neighbourhood is the first thing
   *  anybody does with one, and the distance is the workbench's own - the same
   *  great circle its path losses use. */
  async near(node, count = 0) {
    const got = await this._wb.call("nodes.near", { node: String(node), count });
    return (got || {}).near || [];
  }

  /** Filter by kind. Evaluated here rather than at the workbench: it is a
   *  question about a list somebody already has. */
  async ofKind(kind) {
    return (await this.list()).filter((n) => n.kind === kind);
  }

  /** Put one node down, and hand back a handle to it.
   *
   *  It inherits its neighbours' regions and their firmware, because somebody
   *  dropping a repeater on a map is adding a repeater to this network, not
   *  choosing a firmware strategy.
   *
   *  A board name nothing matches is refused rather than ignored: the board
   *  decides the transmit ceiling, the noise figure and the battery, so a silent
   *  fallback would be a different node answering the question. */
  async place({ name, kind = Kind.SIMPLE_REPEATER, lat = 0, lon = 0,
    heightM, txDbm, board } = {}) {
    const params = { name, kind, lat, lon };
    if (heightM !== undefined) params.height_m = heightM;
    if (txDbm !== undefined) params.tx_dbm = txDbm;
    if (board) params.board = board;
    await this._wb.call("nodes.place", params);
    return new Node(this._wb, name);
  }

  /** Put several down, then measure the links once.
   *
   *  One warm at the end rather than one per node: nodes.place re-measures the
   *  matrix each time, and on a national network that is minutes repeated. */
  async placeMany(placements) {
    const out = [];
    for (const p of placements) out.push(await this.place(p));
    await this._wb.call("links.recompute");
    return out;
  }

  /** Remove them, in one rebuild.
   *
   *  All or none: a name that is not there refuses and removes nothing, because
   *  half a deletion leaves a scenario nobody described and no way to tell which
   *  half survived without asking again. */
  async delete(...nodes) {
    if (nodes.length) await this._wb.call("nodes.delete_many", names(nodes));
  }

  /** Delete everything these do not name.
   *
   *  The complement is worked out at the workbench rather than here, so it
   *  cannot be computed against a list that changed in between. */
  async keep(...nodes) {
    await this._wb.call("nodes.keep", names(nodes));
  }

  /** Replace the selection, or add to it. */
  async select(nodes, { add = false } = {}) {
    await this._wb.call(add ? "nodes.add_to_selection" : "nodes.select_many",
      names(nodes));
  }

  /** Who is selected now. */
  async selected() {
    return (await this.list()).filter((n) => n.selected).map((n) => n.name);
  }

  /** Sample what every node is costing, rather than waiting for the window to
   *  ask. */
  async stats() { return this._wb.nodeStats(); }

  /** Give every node the same antenna, or every node of one kind.
   *
   *  The fleet-level default, and the only way a large scenario gets one:
   *  setting fifty-eight nodes by hand is not a workflow anybody will use. What
   *  is not named is left alone, so this can retune the whole mesh's feedlines
   *  without restating what is on top of the masts. */
  async setAntenna({ kind = "", ...change } = {}) {
    const p = antennaParams(change);
    if (kind) p.kind = kind;
    await this._wb.call("nodes.antenna", p);
  }
}

/** What an antenna change says, in the wire's own spelling.
 *
 *  What is left out is left alone, because "leave this" and "set it to zero"
 *  are different answers and one number cannot say both. */
function antennaParams({ pattern, gainDbiPeak, beamwidthDeg, frontToBackDb,
  bearingDeg, downtiltDeg, polarisation, feedlineDb } = {}) {
  const p = {};
  if (pattern !== undefined) p.pattern = pattern;
  if (gainDbiPeak !== undefined) p.gain_dbi_peak = gainDbiPeak;
  if (beamwidthDeg !== undefined) p.beamwidth_deg = beamwidthDeg;
  if (frontToBackDb !== undefined) p.front_to_back_db = frontToBackDb;
  if (bearingDeg !== undefined) p.bearing_deg = bearingDeg;
  if (downtiltDeg !== undefined) p.downtilt_deg = downtiltDeg;
  if (polarisation !== undefined) p.polarisation = polarisation;
  if (feedlineDb !== undefined) p.feedline_db = feedlineDb;
  return p;
}

/** One node. Live: a handle, not a copy - it holds a name and asks. */
export class Node {
  constructor(wb, name) {
    this._wb = wb;
    this.name = name;
  }

  toString() { return this.name; }

  // ---- what it is ------------------------------------------------------

  /** What the network says about it, now. */
  async info() { return new Nodes(this._wb).info(this.name); }

  /** Its row from the statistics sample, or null when it has none yet. */
  async stat() {
    return (await this._wb.nodeStats()).find((s) => s.name === this.name) || null;
  }

  /** Whether its firmware process is up. */
  async running() {
    const s = await this.stat();
    return Boolean(s && s.running);
  }

  /** "running", "stopped", or one of the transitions. A boolean cannot say
   *  "changing firmware", and a row that goes blank while it happens looks like
   *  a node that has died. */
  async state() {
    const s = await this.stat();
    return s ? s.state : "unknown";
  }

  /** The build this node runs, or null when it is pinned to nothing.
   *
   *  The whole row rather than the version string, because deleting a build or
   *  comparing two needs its path and its board, and reassembling those from a
   *  version is the kind of guesswork that deletes the wrong file. */
  async build() {
    const want = (await this.info()).firmware;
    if (!want) return null;
    return (await this._wb.firmware.library()).find((b) => b.version === want) || null;
  }

  /** What this node's radio is set to - the same thing the workbench shows
   *  under Radio. What the model assumes, and, for a node that is running, what
   *  it reports back and where the two differ. Left as the workbench sent it
   *  because a repeater and a companion answer it differently. */
  async radio() {
    return (await this._wb.call("node.radio", { node: this.name })) || {};
  }

  /** What this node stands under, and which way it points.
   *
   *  Gain is directional in azimuth, so `bearing_deg` is not decoration: a beam
   *  is twenty decibels or more down off its boresight, and which way it faces
   *  decides which links close. */
  async antenna() {
    return (await this._wb.call("node.antenna", { node: this.name })) || {};
  }

  /** Choose and aim this node's antenna. What is not named is left alone, so
   *  turning a beam does not restate the beam. */
  async setAntenna(change) {
    const p = antennaParams(change);
    p.node = this.name;
    await this._wb.call("nodes.antenna", p);
  }

  /** Turn this node's antenna towards another node.
   *
   *  The bearing between two placed nodes is exact, so this is a better answer
   *  than reading one off a map and typing it back. What comes back says what
   *  the turn won, which on an omni is nothing. */
  async aim(at) {
    return (await this._wb.call("node.aim",
      { node: this.name, at: String(at) })) || {};
  }

  // ---- what it does ----------------------------------------------------

  async start() { await this._wb.call("node.start", this.name); }

  async stop() { await this._wb.call("node.stop", this.name); }

  /** Remove it from the scenario, and re-measure what is left. */
  async delete() { await this._wb.call("nodes.delete", { node: this.name }); }

  /** Put it somewhere else. The physics moves with it: cached losses for this
   *  node are forgotten. */
  async move(lat, lon) {
    await this._wb.call("nodes.move", { node: this.name, lat, lon });
  }

  /** What this node relays flood traffic for. */
  async setRegions(...regions) {
    await this._wb.call("nodes.regions", { node: this.name, regions });
  }

  /** Change what it runs.
   *
   *  Applied by default, which means stop, provision, start: firmware is chosen
   *  when a node launches, so recording it and leaving the node on its old build
   *  is the control somebody presses twice and then distrusts. Pass
   *  `{apply: false}` to record it for the next start instead - and know that is
   *  what you have done. */
  async setFirmware(build, { apply = true } = {}) {
    const b = typeof build === "string" ? { version: build } : build;
    await this._wb.call(apply ? "node.set_firmware" : "node.set_firmware_only", {
      node: this.name, version: b.version,
      board: b.board || "", role: b.role || "",
    });
  }

  /** What hardware this node is.
   *
   *  A change to the physics rather than a label, so it rebuilds and re-warms -
   *  and it clears a firmware pin made for a different board, because that image
   *  cannot run on this one and a pin nobody can honour reads as a configured
   *  node right up until it refuses to start. */
  async setBoard(board) {
    await this._wb.call("node.set_board", { node: this.name, board });
  }

  /** Take waveform verdicts whatever the run's mode - the hybrid flag, for
   *  measuring one node honestly inside a cheap run. */
  async setTrueRF(on = true) {
    await this._wb.call("node.truerf", { node: this.name, on });
  }

  /** Originate a packet without firmware.
   *
   *  It exercises the radio model and the channel; what it does not exercise is
   *  relaying, which is a firmware behaviour and needs a firmware. */
  async inject() { await this._wb.call("sim.inject", this.name); }

  /** What this node is told at boot, in the console's own words. */
  async provisioning() {
    return ((await this._wb.call("node.provisioning", this.name)) || {}).commands || [];
  }

  /** Hand this companion to a real client - meshcore-cli, or an app over a
   *  bridge - and say where to point it. */
  async serve(over = Transport.TCP) {
    const got = await this._wb.call("bench.serve", { node: this.name, kind: over });
    return (got || {}).addr || "";
  }

  /** Take it back. */
  async unserve() { await this._wb.call("bench.drop", { node: this.name }); }

  // ---- looking at it ---------------------------------------------------

  /** What this node printed, from one of four voices - the lines, not a count
   *  of them.
   *
   *  "serial" is the board's own port (a native node's standard error), "boot"
   *  is the ROM's on a board whose application talks over USB, "emulator" is
   *  what QEMU or Renode said about running it, and "radio" is the radio
   *  model's log. A board that has gone quiet is read by looking at what it last
   *  said. */
  async output(source = "serial", lines = 200) {
    const got = await this._wb.call("node.output",
      { node: this.name, source, lines });
    return (got || {}).tail || [];
  }

  /** Open one of this node's logs in a window of its own.
   *
   *  A tab is one pane. What people do while a board is misbehaving is watch its
   *  screen and two of its logs together - what the board printed beside what
   *  the emulator said about running it - and that needs windows. */
  async outputWindow(source = "serial") {
    await this._wb.call("node.output_window", { node: this.name, source });
  }

  /** This node's console, on whichever of the two verbs its kind reads. */
  get console() { return this._wb.console(this.name); }

  /** This node as a device to drive: its screen, buttons and panel. All of it
   *  works headless - the display is the framebuffer the controller holds, not a
   *  picture of the desktop. */
  get device() { return new Device(this._wb, this.name); }

  // ---- its storage -----------------------------------------------------

  /** What is in this node's card slot, and changing it.
   *
   *  A slot is not a fitted card: the board says the slot exists, this says
   *  whether it is filled. `file` hands the node a card of your own - shared
   *  between runs, or prepared in advance; an empty string returns it to its
   *  own, named after it and kept beside its flash. `wipe` erases it, which is
   *  what reformatting one is, and is refused while the node is running.
   *
   *  A firmware marked as needing a card fills the slot whatever this says,
   *  because a build that keeps its settings there boots into nothing without
   *  one. */
  async card({ fitted, file, wipe = false } = {}) {
    const p = { node: this.name };
    if (fitted !== undefined) p.fitted = fitted;
    if (file !== undefined) p.file = file;
    if (wipe) p.wipe = true;
    return (await this._wb.call("node.card", p)) || {};
  }

  /** Put this board back to factory: its flash, its card, its files.
   *
   *  A board keeps what it was told between runs, as hardware does, so a node
   *  configured into a corner stays there until this is called. Refused while it
   *  is running, rather than rewriting a flash underneath the emulator holding
   *  it. */
  async wipe() { await this._wb.call("node.wipe", { node: this.name }); }

  // ---- waiting ---------------------------------------------------------

  /** Wait for its firmware process to be up.
   *
   *  `timeoutMs` is wall clock - how long you are prepared to sit here - not
   *  simulated time. Starting a process is real work on the real machine. */
  waitRunning(timeoutMs = FIRMWARE_WAIT_MS) {
    return waitFor(async () => {
      const s = await this.stat();
      if (s && s.running) return [true, ""];
      return [false, s ? s.state : "no stat row yet"];
    }, timeoutMs, `firmware on ${this.name}`);
  }
}
