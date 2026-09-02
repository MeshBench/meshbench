// What a run is told to send, and what has to be true of it afterwards.
//
// These were reachable only through call("schedule.add", {at_ms: 5000,
// every_ms: 20000}), which is the shape this package exists to remove: a verb
// name spelled by hand, parameters in milliseconds because that is what the
// wire happens to use, and nothing to tell a reader which clock they are in. An
// example written that way is an advertisement for not using the library.

import fs from "node:fs";

/** What the mesh is told to send, and when. Live. */
export class Schedule {
  constructor(wb) { this._wb = wb; }

  /** Have a node send something, once or repeatedly.
   *
   *  `atMs` and `everyMs` are simulated time - the mesh's own clock, not
   *  yours - which is why neither is called a timeout.
   *
   *  Repeating traffic has worked all along and nothing said so, which to
   *  somebody writing a script is the same as it not existing. */
  async add({ node, command, atMs = 0, everyMs = 0 }) {
    const params = { node: String(node), command };
    if (atMs > 0) params.at_ms = atMs;
    if (everyMs > 0) params.every_ms = everyMs;
    return ((await this._wb.call("schedule.add", params)) || {}).sends || 0;
  }

  /** Forget all of them. */
  async clear() {
    return ((await this._wb.call("schedule.clear")) || {}).cleared || 0;
  }

  /** How many are scheduled. */
  async count() {
    return (await this._wb.snapshot()).scheduled_sends || 0;
  }
}

/** One assertion, and what the run made of it. */
export class Check {
  constructor(raw = {}) {
    this.kind = raw.kind || "";
    this.node = raw.node || "";
    this.passed = Boolean(raw.pass);
    this.got = raw.got || "";
    this.want = raw.want || "";
  }

  toString() {
    const mark = this.passed ? "pass" : "FAIL";
    const where = this.node ? ` at ${this.node}` : "";
    return `${mark}  ${this.kind}${where}: got ${this.got}, want ${this.want}`;
  }
}

/** What a run passed and failed, with what it was measured under. */
export class Report {
  constructor(raw = {}, provenance = null) {
    this.passed = raw.passed || 0;
    this.total = raw.total || 0;
    this.checks = (raw.results || []).map((r) => new Check(r));
    /** What the numbers were measured under, carried with the verdict because a
     *  delivery figure without it is the number this project exists not to
     *  publish. */
    this.provenance = provenance;
  }

  /** Whether every assertion held.
   *
   *  A report with no assertions is not ok. A fixture that carries none can
   *  report but cannot pass, and a green tick that checked nothing is the worst
   *  outcome available here. */
  get ok() { return this.total > 0 && this.passed === this.total; }

  get failures() { return this.checks.filter((c) => !c.passed); }

  toString() {
    const lines = [];
    if (this.provenance) lines.push(String(this.provenance));
    lines.push(this.total === 0
      ? "no assertions, so this run checked nothing"
      : `${this.passed} of ${this.total} assertions passed`);
    for (const c of this.failures) lines.push(`  ${c}`);
    return lines.join("\n");
  }

  /** Write a JUnit file, with the caveats inside it.
   *
   *  In the file rather than only on stdout, because the file is what a CI
   *  system keeps and shows six months later - and a delivery figure with no
   *  note of what the model assumed is exactly the number this project exists
   *  not to publish. */
  writeJUnit(path, suite = "meshbench") {
    const lines = ['<?xml version="1.0" encoding="utf-8"?>',
      `<testsuite name="${xml(suite)}" tests="${this.total}" ` +
      `failures="${this.failures.length}">`];
    if (this.provenance) {
      lines.push("  <properties>",
        `    <property name="meshbench.provenance" value="${xml(this.provenance)}"></property>`,
        "  </properties>");
    }
    for (const c of this.checks) {
      const name = c.kind + (c.node ? ` at ${c.node}` : "");
      const open = `  <testcase classname="${xml(suite)}.assertions" name="${xml(name)}"`;
      if (c.passed) {
        lines.push(open + "></testcase>");
        continue;
      }
      lines.push(open + ">",
        `    <failure message="got ${xml(c.got)}, want ${xml(c.want)}"></failure>`,
        "  </testcase>");
    }
    lines.push("</testsuite>", "");
    fs.writeFileSync(path, lines.join("\n"));
  }
}

/** Escape text for an XML attribute or body.
 *
 *  Written out rather than pulled in: a dependency for five replacements would
 *  cost this package the one thing it promises, which is that `npm install`
 *  fetches nothing but itself. */
function xml(s) {
  return String(s)
    .replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;").replaceAll("'", "&apos;");
}

/** What has to be true for a run to have passed. Live. */
export class Assertions {
  constructor(wb) { this._wb = wb; }

  /** At least this many nodes received something. */
  async delivered(atLeast, { withinMs = 0 } = {}) {
    return this.add({ kind: "delivered", atLeast, withinMs });
  }

  /** This node - or the whole mesh - transmitted within these bounds.
   *
   *  `atMost` is the interesting one: it is how a relay-suppression change is
   *  held to not having made the mesh chattier. */
  async sent({ node = "", atLeast = 0, atMost = 0, withinMs = 0 } = {}) {
    return this.add({ kind: "sent", node, atLeast, atMost, withinMs });
  }

  /** The general form, for a kind this package has no name for yet.
   *
   *  `withinMs` is simulated time, like everything else the mesh is measured
   *  over. A kind this build does not understand is a failure rather than a
   *  pass, because a green run that checked nothing is the worst outcome
   *  available here. */
  async add({ kind, node = "", atLeast = 0, atMost = 0, maxPct = 0, withinMs = 0 }) {
    const params = { kind };
    if (node) params.node = String(node);
    if (atLeast) params.at_least = atLeast;
    if (atMost) params.at_most = atMost;
    if (maxPct) params.max_pct = maxPct;
    if (withinMs) params.within_ms = withinMs;
    return ((await this._wb.call("assert.add", params)) || {}).assertions || 0;
  }

  /** Measure every assertion against the run so far.
   *
   *  The provenance travels with the verdict, because a delivery figure without
   *  what the model assumed is the number this project exists not to publish. */
  async check() {
    const got = (await this._wb.call("assert.check")) || {};
    return new Report(got, await this._wb.provenance());
  }

  /** How many are recorded. */
  async count() { return (await this._wb.snapshot()).assertions || 0; }
}

/** The assertion kinds this build understands. */
export const ASSERTION_KINDS = Object.freeze(
  ["delivered", "deliveries", "unique_deliveries", "sent", "transmissions"]);
