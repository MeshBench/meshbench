// What this machine can run, and what it is running.

import { NotFound, refusal } from "./errors.mjs";
import { FIRMWARE_WAIT_MS, JOB_WAIT_MS, waitFor } from "./wait.mjs";
import { describeBuild } from "./values.mjs";

/** Which build a call means, from either a library row or a bare label.
 *
 *  A row carries all three names and they are sent together, so the call cannot
 *  land on a different build that happens to share a label. A bare label sends
 *  only what was given, and the workbench refuses it when it is ambiguous
 *  rather than guessing - acting on the wrong build is a rename of somebody
 *  else's image. */
function buildID(build, { board = "", role = "" } = {}) {
  if (build && typeof build === "object") {
    return { version: build.version, role: build.role, board: build.board };
  }
  const p = { version: build };
  if (role) p.role = role;
  if (board) p.board = board;
  return p;
}

/** What this machine can run, and what it is running. Live. */
export class Firmware {
  constructor(wb) { this._wb = wb; }

  /** Every build, published or on disk, with what runs it - the rows, not a
   *  count of them. */
  async library() {
    return ((await this._wb.call("firmware.library")) || {}).builds || [];
  }

  /** Only the ones this machine holds, which is the only thing that decides
   *  what a node can run. A build that failed to download and one in daily use
   *  look identical from anywhere else. */
  async onDisk() {
    return (await this.library()).filter((b) => b.on_disk);
  }

  /** One build by version, and by board where the version alone is ambiguous -
   *  which it is for every board image, because "wadamesh" is not a build until
   *  it is wadamesh for a particular piece of hardware. */
  async find(version, board = "") {
    for (const b of await this.library()) {
      if (b.version === version && (!board || b.board === board)) return b;
    }
    throw new NotFound("firmware.library",
      `no build "${version}" for board "${board}"`, "not_found");
  }

  /** Everything known about one build: where it is, what it is, and what has
   *  been decided about it. */
  async details(build, opts) {
    return (await this._wb.call("firmware.details", buildID(build, opts))) || {};
  }

  /** Rename a build, move it to another board or role, or change how it is run,
   *  and report it as it now stands.
   *
   *  Renaming moves the file, because the name is the identity: a board image
   *  is stored as `<board>/<role>@<label>.bin` and nothing else records what it
   *  is. Nodes pinned to the old name are repointed, or they would fail at
   *  their next start with "no image in the cache" about a build sitting in the
   *  library under its new name.
   *
   *  Every setting left out is left alone, which is why they are undefined
   *  rather than "" or false: "leave this" and "turn it off" are different
   *  answers. */
  async update(build, {
    board = "", role = "", label, newRole, newBoard,
    coprocAtReset, cardRequired, notes,
  } = {}) {
    const p = buildID(build, { board, role });
    if (label !== undefined) p.label = label;
    if (newRole !== undefined) p.new_role = newRole;
    if (newBoard !== undefined) p.new_board = newBoard;
    if (coprocAtReset !== undefined) p.coproc_at_reset = coprocAtReset;
    if (cardRequired !== undefined) p.card_required = cardRequired;
    if (notes !== undefined) p.notes = notes;
    const moved = (await this._wb.call("firmware.update", p)) || {};
    // Read back under whatever it is called now, which is not what it was
    // called if this was a rename.
    return this.details(moved.version || "",
      { board: moved.board || "", role: moved.role || "" });
  }

  /** Open the build's own window - what a click on a library row does. Refused
   *  by a workbench with no interface. */
  async window(build, opts) {
    await this._wb.call("firmware.window", buildID(build, opts));
  }

  /** Ask the catalogue what is published, which is how a build nobody has
   *  downloaded becomes offerable. */
  async scan() { await this._wb.call("firmware.rescan"); }

  /** Fetch a published build. It returns once the download has been asked for,
   *  not once it has landed: wait on the job.
   *
   *  `role` is a plain string here and a Role everywhere else, deliberately:
   *  this one names a published release asset, and the catalogue's own
   *  spellings are not always the application names the verbs are keyed on. */
  async download(role, version, board = "") {
    const p = { role, version };
    if (board) p.board = board;
    await this._wb.call("firmware.download", p);
  }

  /** Take a build from a path - the one way a locally built image gets into the
   *  library.
   *
   *  `label` is what the library will know it by and what a node pins. Left out
   *  it is a timestamp, so importing twice gives two builds rather than one
   *  that quietly replaced the other - which matters the moment you want to put
   *  the new one on a node and delete the old. */
  async import(path, role, { board = "", label = "" } = {}) {
    const p = { path, role };
    if (board) p.board = board;
    if (label) p.label = label;
    return (await this._wb.call("firmware.import", p)) || {};
  }

  /** Remove a build from the cache, and say what was removed.
   *
   *  By path, and the workbench refuses any path outside the firmware cache. A
   *  build nodes are still pinned to will go: they keep the pin, which then
   *  cannot be honoured and fails at start - so move them onto the replacement
   *  first. */
  async delete(build) {
    if (!build || !build.path) {
      throw refusal("firmware.delete",
        `${describeBuild(build)} has no path on this machine to delete`,
        "bad_params");
    }
    const got = await this._wb.call("firmware.delete", { path: build.path });
    return (got || {}).deleted || "";
  }

  /** Compile a MeshCore checkout and put the results in the library.
   *
   *  Both roles unless one is named, deliberately. A locally built repeater
   *  compiled against a stale shim once answered console output with 0x06 where
   *  the host expects 0x07: it connected, misbehaved and exited. Two arms of a
   *  comparison built at different moments from different trees measure the
   *  build process rather than the firmware, so the easy thing here is the
   *  thing that builds them together.
   *
   *  Blocks until it is done - a MeshCore build is a minute or two per role -
   *  and returns what the library now holds that was built locally. */
  async build(checkout, { role = "", label = "", waitMs = JOB_WAIT_MS } = {}) {
    const p = { source: checkout };
    if (role) p.role = role;
    if (label) p.label = label;
    const got = (await this._wb.call("firmware.build", p)) || {};
    await this._wb.job(got.job || "firmware-build").wait(waitMs);
    return (await this.library()).filter((b) => b.version.startsWith("local-"));
  }

  /** Pin every role that needs one to the newest build on this machine, and
   *  report what it chose.
   *
   *  What a script wants almost every time: this mesh, whatever this machine
   *  holds, rather than a version typed into the script that goes stale. A run
   *  refuses to start until every role is answered, so the alternative is the
   *  same loop written out in every caller.
   *
   *  It refuses by name when a role has nothing, because "no companion build"
   *  is a thing to go and fix rather than a reason to start a mesh with a
   *  silent hole in it. */
  async useWhatIsHere() {
    const have = (await this.onDisk()).filter((b) => !b.board);
    const chosen = {};
    for (const want of await this.needed()) {
      const pick = have.filter((b) => b.role === want.role).pop();
      if (!pick) {
        throw new NotFound("firmware.needed",
          `no ${want.role} build on this machine: ` +
          `meshbench firmware download ${want.role}`, "not_found");
      }
      await this.useForRole(want.role, pick);
      chosen[want.role] = pick;
    }
    return chosen;
  }

  /** Pin every node of a role to one build. */
  async useForRole(role, build) {
    const version = typeof build === "string" ? build : build.version;
    await this._wb.call("firmware.set", { role, version });
  }

  /** Bring up firmware on every node.
   *
   *  Asynchronous, and always has been: it answers with what it has begun, not
   *  with what is up. It was synchronous once, and on 155 nodes that froze the
   *  window and the socket together for as long as it was left - which read as
   *  a crash and was reported as one. Wait with waitStarted. */
  async start() { await this._wb.call("firmware.start"); }

  /** How far a start has got. */
  async state() { return (await this._wb.call("firmware.state")) || {}; }

  /** The roles this scenario has nodes for and no build pinned to, with what
   *  could be pinned. A run refuses to start until every one is answered. */
  async needed() {
    return ((await this._wb.call("firmware.needed")) || {}).roles || [];
  }

  /** Wait for every node's firmware to be up. `timeoutMs` is wall clock.
   *
   *  `nodes` here is the nodes that run firmware, which is not every node: an
   *  SDR observer and an emitter never boot one. It used to be every node, so a
   *  fixture holding either reported "56 of 58" until the timeout with no way
   *  to see which two.
   *
   *  Which is why this names the stragglers rather than counting them. Ten
   *  minutes of "56 of 58" tells you nothing; two node names tell you whether a
   *  build is missing or a board is wedged. */
  waitStarted(timeoutMs = FIRMWARE_WAIT_MS) {
    let named = 0;
    let last = "";
    return waitFor(async () => {
      const st = await this.state();
      const running = st.running || 0;
      const nodes = st.nodes || 0;
      if (!st.starting && nodes > 0 && running >= nodes) return [true, ""];
      // The names cost a /proc read per node and this polls while firmware is
      // starting, which is the busiest moment there is - every fiftieth of a
      // second is how a diagnostic becomes the fault it was meant to explain,
      // and it timed the socket out. Once every ten seconds is often enough for
      // something a person only reads when the wait fails.
      if (Date.now() - named >= 10_000) {
        named = Date.now();
        last = await this._stragglers();
      }
      return [false, `${running} of ${nodes} running${last}`];
    }, timeoutMs, "firmware to come up");
  }

  async _stragglers() {
    const waiting = (await this._wb.nodeStats())
      .filter((s) => !s.running).map((s) => s.name);
    if (waiting.length === 0) return "";
    let shown = waiting.slice(0, 4).join(", ");
    if (waiting.length > 4) shown += ` and ${waiting.length - 4} more`;
    return `; waiting on ${shown}`;
  }
}
