// The shape above the wire: the helpers that exist to stop a mistake somebody
// has already made. Each one is held to the sequence it is supposed to ask for,
// because the faults these fix were all silent - a helper that skipped a step
// would still return, and the run would simply be wrong.

import { test } from "node:test";
import assert from "node:assert/strict";

import { Kind, NotFound, Role, Timeout, Unavailable } from "../meshbench.mjs";
import { answers, withFake } from "./fake.mjs";

const idle = { jobs: [] };

// sim.start is the flagship. sim.start the *verb* is the play button's own
// handler and answers four ways; this asks for the three things a script
// actually means, in order, and checks each.
test("sim.start waits out the warm, starts what is down, then plays", async () => {
  const asked = [];
  let running = 0;
  await withFake(answers({
    "session.snapshot": idle,
    "sim.state": () => ({ playing: false, links_measured: true }),
    "firmware.state": () => ({ running, nodes: 4, starting: false }),
    "firmware.start": () => { running = 4; return {}; },
    "nodes.stats": { stats: [] },
    "sim.play": {},
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    await wb.sim.start();
  });
  const order = asked.filter((m) => m !== "session.hello" && m !== "sim.state");
  assert.deepEqual(order, [
    "session.snapshot",   // waitIdle: the links first
    "firmware.state",     // who is down
    "firmware.start",     // start them
    "firmware.state",     // waitStarted
    "sim.play",           // and only then the clock, by its own name
  ]);
});

// The verb starts firmware only when *no* node is running. Pin a build onto two
// nodes of a fifty-eight node fixture and it considers the mesh started, plays
// with fifty-six of them down, and says nothing.
test("sim.start starts firmware when some nodes are up but not all", async () => {
  let started = false;
  await withFake(answers({
    "session.snapshot": idle,
    "sim.state": { playing: false, links_measured: true },
    "firmware.state": () => ({ running: started ? 58 : 2, nodes: 58, starting: false }),
    "firmware.start": () => { started = true; return {}; },
    "nodes.stats": { stats: [] },
    "sim.play": {},
  }), async (wb) => {
    await wb.sim.start();
  });
  assert.ok(started, "firmware.start was not called with 2 of 58 running");
});

// Idle is not the same as measured. A warm that failed or was cancelled
// finishes its own job row, so waitIdle returns having waited for nothing -
// and every study after that answers over free space.
test("sim.start refuses an unmeasured matrix rather than playing over free space", async () => {
  await withFake(answers({
    "session.snapshot": idle,
    "sim.state": {
      playing: false, links_measured: false, warming: false,
      ground: { note: "warm the links first" },
    },
  }), async (wb) => {
    await assert.rejects(wb.sim.start(), (e) => {
      assert.match(e.message, /no link has been measured/);
      assert.match(e.message, /warm the links first/);
      return true;
    });
  });
});

test("sim.start does not press play on a run that is already playing", async () => {
  const asked = [];
  await withFake(answers({
    "session.snapshot": idle,
    "sim.state": { playing: true, links_measured: true },
    "firmware.state": { running: 4, nodes: 4, starting: false },
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    await wb.sim.start();
  });
  assert.ok(!asked.includes("sim.play"), "play cannot pause, and must not be pressed");
});

// Some jobs are removed when they end and some are only marked - infer.run's is
// marked - so waiting for the list to empty waits for ever on half of them.
test("waitIdle ignores jobs that are finished but still listed", async () => {
  await withFake(answers({
    "session.snapshot": { jobs: [{ id: "infer", what: "inferring", finished: true }] },
  }), async (wb) => {
    await wb.waitIdle(2000);
  });
});

test("waitIdle names what it was still waiting on when it gives up", async () => {
  await withFake(answers({
    "session.snapshot": {
      jobs: [{ id: "warm", what: "measuring links", done: 3, total: 9, finished: false }],
    },
  }), async (wb) => {
    await assert.rejects(wb.waitIdle(150), (e) => {
      assert.ok(e instanceof Timeout);
      assert.match(e.message, /measuring links/);
      assert.match(e.message, /3 of 9/);
      return true;
    });
  });
});

// nodes here is the nodes that run firmware, which is not every node: an SDR
// observer and an emitter never boot one. And the wait names the stragglers
// rather than counting them - ten minutes of "56 of 58" tells you nothing.
test("firmware.waitStarted compares against the nodes that run firmware", async () => {
  await withFake(answers({
    "firmware.state": { running: 56, nodes: 56, total: 58, starting: false },
    "nodes.stats": { stats: [] },
  }), async (wb) => {
    await wb.firmware.waitStarted(2000);
  });
});

test("firmware.waitStarted names the stragglers when it gives up", async () => {
  await withFake(answers({
    "firmware.state": { running: 1, nodes: 3, starting: true },
    "nodes.stats": {
      stats: [
        { name: "Alpha", running: true }, { name: "Bravo", running: false },
        { name: "Charlie", running: false },
      ],
    },
  }), async (wb) => {
    await assert.rejects(wb.firmware.waitStarted(200), (e) => {
      assert.match(e.message, /1 of 3 running/);
      assert.match(e.message, /waiting on Bravo, Charlie/);
      return true;
    });
  });
});

// A repeater reads typed text; a companion speaks the framed protocol and its
// command line is meshcore-cli's vocabulary. Text typed at a companion is echoed
// locally and goes nowhere, which looks exactly like a command that ran and did
// nothing.
test("the console routes by node kind", async () => {
  const asked = [];
  await withFake(answers({
    "nodes.list": {
      nodes: [
        { name: "R1", kind: Kind.SIMPLE_REPEATER },
        { name: "C1", kind: Kind.COMPANION },
        { name: "Room", kind: Kind.ROOM_SERVER },
      ],
    },
    "console.type": {},
    "console.cli": {},
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    await wb.node("R1").console.send("advert");
    await wb.node("C1").console.send("advert");
    await wb.node("Room").console.send("advert");
  });
  assert.deepEqual(asked.filter((m) => m.startsWith("console.")),
    ["console.type", "console.cli", "console.cli"]);
});

// A node this client cannot see is not one to guess about: the typed verb is the
// fallback, and its refusal says so in its own words.
test("the console falls back to the typed verb for a node it cannot see", async () => {
  const asked = [];
  await withFake(answers({
    "nodes.list": { nodes: [] },
    "console.type": {},
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    await wb.node("Ghost").console.send("advert");
  });
  assert.ok(asked.includes("console.type"));
});

// A node reads its serial input on its next loop and its loop only runs when the
// engine steps, so reading straight after sending reads the moment *before* the
// command went out.
test("console.ask gives the mesh its own time before reading back", async () => {
  const asked = [];
  let sent = false;
  await withFake(answers({
    "nodes.list": { nodes: [{ name: "R1", kind: Kind.SIMPLE_REPEATER }] },
    "console.read": () => ({ tail: sent ? ["> advert", "sent"] : ["> advert"], lines: 2 }),
    "console.type": () => { return {}; },
    "sim.state": { playing: false, now_ms: 0, step_ms: 50 },
    "sim.settle": () => { sent = true; return {}; },
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    const reply = await wb.node("R1").console.ask("advert", 10);
    assert.equal(reply, "sent");
  });
  const i = asked.indexOf("sim.settle");
  assert.ok(i > 0 && i < asked.lastIndexOf("console.read"),
    "settle must come between the send and the read back");
});

// console.read answers with the lines under "tail"; "lines" is how many there
// are. Reading "lines" hands you an integer where you asked for text.
test("console.read returns the lines, not the count", async () => {
  await withFake(answers({
    "console.read": { tail: ["one", "two"], lines: 2 },
  }), async (wb) => {
    assert.deepEqual(await wb.console("R1").read(), ["one", "two"]);
  });
});

test("the four verbs that answered with a count return their rows", async () => {
  await withFake(answers({
    "nodes.stats": { stats: [{ name: "R1", running: true }], count: 1 },
    "firmware.library": { builds: [{ version: "v1", role: Role.SIMPLE_REPEATER }], count: 1 },
    "boundary.list": { names: ["Fife"], count: 1 },
    "events.recent": { events: [{ kind: "rx" }], total: 900 },
  }), async (wb) => {
    assert.deepEqual((await wb.nodeStats()).map((s) => s.name), ["R1"]);
    assert.deepEqual((await wb.firmware.library()).map((b) => b.version), ["v1"]);
    assert.deepEqual(await wb.boundary.list(), ["Fife"]);
    assert.equal((await wb.events.recent()).length, 1);
    assert.equal(await wb.events.total(), 900);
  });
});

// It refuses by name when a role has nothing, because "no companion build" is a
// thing to go and fix rather than a reason to start a mesh with a silent hole.
test("useWhatIsHere pins each needed role, and refuses by name when it cannot", async () => {
  const pinned = [];
  const library = {
    builds: [
      { version: "old", role: Role.SIMPLE_REPEATER, on_disk: true, board: "" },
      { version: "new", role: Role.SIMPLE_REPEATER, on_disk: true, board: "" },
      { version: "board-only", role: Role.COMPANION_RADIO, on_disk: true, board: "LilyGo_TDeck" },
      { version: "not-here", role: Role.COMPANION_RADIO, on_disk: false, board: "" },
    ],
  };
  await withFake(answers({
    "firmware.library": library,
    "firmware.needed": { roles: [{ role: Role.SIMPLE_REPEATER, nodes: 2 }] },
    "firmware.set": (p) => { pinned.push(p); return {}; },
  }), async (wb) => {
    const chosen = await wb.firmware.useWhatIsHere();
    assert.equal(chosen[Role.SIMPLE_REPEATER].version, "new");
  });
  assert.deepEqual(pinned, [{ role: Role.SIMPLE_REPEATER, version: "new" }]);

  // A board image is not a host build, so the companion role has nothing.
  await withFake(answers({
    "firmware.library": library,
    "firmware.needed": { roles: [{ role: Role.COMPANION_RADIO, nodes: 1 }] },
  }), async (wb) => {
    await assert.rejects(wb.firmware.useWhatIsHere(), (e) => {
      assert.ok(e instanceof NotFound);
      assert.match(e.message, /no companion_radio build on this machine/);
      assert.match(e.message, /meshbench firmware download companion_radio/);
      return true;
    });
  });
});

// Taking the top result unconditionally is how a script ends up sending an
// advert from a node that merely shared a word with what was asked for.
test("nodes.find refuses a weak top answer, and names what it did find", async () => {
  await withFake(answers({
    "nodes.search": { matches: [{ name: "West Kilbride", score: 0.31 }] },
  }), async (wb) => {
    await assert.rejects(wb.nodes.find("West Lomond"), (e) => {
      assert.ok(e instanceof NotFound);
      assert.match(e.message, /nothing matches "West Lomond" well enough/);
      assert.match(e.message, /"West Kilbride" \(0\.31\)/);
      return true;
    });
  });
});

test("nodes.find takes a confident answer", async () => {
  await withFake(answers({
    "nodes.search": { matches: [{ name: "\u{1F3D4}️ West Lomond \u{1F4E1}", score: 0.92 }] },
  }), async (wb) => {
    const n = await wb.nodes.find("West Lomond");
    assert.equal(String(n), "\u{1F3D4}️ West Lomond \u{1F4E1}");
  });
});

// One warm at the end rather than one per node: nodes.place re-measures the
// matrix each time, and on a national network that is minutes repeated.
test("placeMany measures the links once, at the end", async () => {
  const asked = [];
  await withFake(answers({
    "nodes.place": {},
    "links.recompute": {},
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    await wb.nodes.placeMany([
      { name: "A", lat: 56, lon: -3 }, { name: "B", lat: 57, lon: -3 },
    ]);
  });
  assert.deepEqual(asked.filter((m) => m !== "session.hello"),
    ["nodes.place", "nodes.place", "links.recompute"]);
});

test("place sends only what was named, so a zero is not mistaken for a default", async () => {
  let params;
  await withFake(answers({
    "nodes.place": (p) => { params = p; return {}; },
  }), async (wb) => {
    await wb.nodes.place({ name: "A", lat: 56, lon: -3 });
  });
  assert.deepEqual(params, { name: "A", kind: Kind.SIMPLE_REPEATER, lat: 56, lon: -3 });
});

// The import chain's last two steps are the ones that get missed, and missing
// them does not fail: the mesh comes up with regions inferred but never applied,
// which transmits everything, relays nothing, and reports no error at all.
test("live.pull runs all four steps in the order that works", async () => {
  const asked = [];
  await withFake(answers({
    "import.set_source": { url: "https://feed" },
    "import.fetch": { records: 700, nodes: 676, uncertain: 3 },
    "import.commit": { nodes: 676 },
    "infer.run": {},
    "session.snapshot": idle,
    "infer.apply": { applied: 640 },
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    const preview = await wb.live.pull("https://feed");
    assert.equal(String(preview), "700 records, 676 usable, 3 placed only roughly");
  });
  assert.deepEqual(
    asked.filter((m) => m.startsWith("import.") || m.startsWith("infer.")),
    ["import.set_source", "import.fetch", "import.commit", "infer.run", "infer.apply"]);
});

test("live.pull stops when the feed described nothing usable", async () => {
  await withFake(answers({
    "import.set_source": { url: "https://feed" },
    "import.fetch": { records: 12, nodes: 0 },
  }), async (wb) => {
    await assert.rejects(wb.live.pull("https://feed"), /none usable/);
  });
});

// A job that ended is not a job that worked: a read that failed used to finish
// with the reason in its title and nothing else, so every caller either carried
// on as though it had succeeded or matched on the wording.
test("job.wait throws when the job finished badly", async () => {
  await withFake(answers({
    "session.snapshot": {
      jobs: [{ id: "dl", what: "downloading v1: 404", finished: true, failed: true }],
    },
  }), async (wb) => {
    await assert.rejects(wb.job("dl").wait(2000), /job dl failed: downloading v1: 404/);
  });
});

// A headless run has nothing to open, and a script that "opened the Hardware
// tab" in CI and saw no error will be written to assume it did.
test("window refuses a headless session here rather than appearing to work", async () => {
  await withFake(answers({}, { hello: { mode: "headless" } }), async (wb) => {
    await assert.rejects(wb.window("R1", "Hardware"), (e) => {
      assert.ok(e instanceof Unavailable);
      assert.match(e.message, /no interface attached/);
      return true;
    });
  });
});

// The caveats have to be in the value: a scripted number gets pasted into a
// report with them stripped.
test("a report carries the provenance, and no assertions is not a pass", async () => {
  await withFake(answers({
    "assert.check": { passed: 0, total: 0, results: [] },
    "session.snapshot": { rf_mode: "waveform", calibrated: true, seed: 9001 },
  }), async (wb) => {
    const report = await wb.assertions.check();
    assert.equal(report.ok, false, "a report with no assertions must not be ok");
    assert.match(String(report), /no assertions, so this run checked nothing/);
    assert.match(String(report), /waveform reception/);
    assert.match(String(report), /excess loss fitted to real receptions/);
    assert.match(String(report), /a best case; no multipath/);
  });
});

test("a report that passed every assertion is ok", async () => {
  await withFake(answers({
    "assert.check": {
      passed: 2, total: 2,
      results: [{ kind: "delivered", pass: true, got: "9", want: ">= 5" },
        { kind: "sent", node: "R1", pass: true, got: "3", want: "<= 4" }],
    },
    "session.snapshot": { rf_mode: "calculated" },
  }), async (wb) => {
    const report = await wb.assertions.check();
    assert.equal(report.ok, true);
    assert.deepEqual(report.failures, []);
  });
});

// Simulated milliseconds, not yours - which is why neither is called a timeout.
test("schedule.add sends the mesh's own clock, in the wire's spelling", async () => {
  let params;
  await withFake(answers({
    "schedule.add": (p) => { params = p; return { sends: 1 }; },
  }), async (wb) => {
    await wb.schedule.add({ node: "C2", command: "public hello", atMs: 5000, everyMs: 20000 });
  });
  assert.deepEqual(params,
    { node: "C2", command: "public hello", at_ms: 5000, every_ms: 20000 });
});

// A mistyped path reported as a missing file, rather than searched for as a
// place - which answers "nothing is called ./bounds/fife.geojson" and sends the
// reader looking in entirely the wrong direction.
test("boundary.use tells a place name from a GeoJSON path", async () => {
  const asked = [];
  await withFake(answers({
    "boundary.set": { names: ["Fife"] },
    "boundary.accept": { accepted: "Fife" },
    "boundary.load": { loaded: ["Tay catchment"] },
  }, { onCall: (m) => asked.push(m) }), async (wb) => {
    assert.deepEqual(await wb.boundary.use("Fife"), ["Fife"]);
    assert.deepEqual(await wb.boundary.use("./bounds/tay.geojson"), ["Tay catchment"]);
  });
  assert.deepEqual(asked.filter((m) => m.startsWith("boundary.")),
    ["boundary.set", "boundary.accept", "boundary.load"]);
});

test("boundary.search refuses an empty result by name", async () => {
  await withFake(answers({ "boundary.set": { names: [] } }), async (wb) => {
    await assert.rejects(wb.boundary.search("Atlantis"), (e) => {
      assert.ok(e instanceof NotFound);
      assert.match(e.message, /nothing is called "Atlantis"/);
      return true;
    });
  });
});

// Half duplex eats stimuli, so a tap followed by an immediate screen read will
// intermittently read the frame from before the tap landed.
test("device.waitScreen watches the digest, not the lit count", async () => {
  let n = 0;
  await withFake(answers({
    "board.screen": () => {
      n += 1;
      return { has_screen: true, lit: 400, digest: n < 3 ? "aaa" : "bbb" };
    },
  }), async (wb) => {
    const now = await wb.node("Deck").device.waitScreen(3000);
    assert.equal(now.digest, "bbb");
  });
});

test("firmware.find refuses by name when the board makes it ambiguous", async () => {
  await withFake(answers({
    "firmware.library": {
      builds: [{ version: "wadamesh", board: "Heltec_v3", role: Role.COMPANION_RADIO }],
    },
  }), async (wb) => {
    const got = await wb.firmware.find("wadamesh", "Heltec_v3");
    assert.equal(got.board, "Heltec_v3");
    await assert.rejects(wb.firmware.find("wadamesh", "LilyGo_TDeck"), (e) => {
      assert.ok(e instanceof NotFound);
      assert.match(e.message, /no build "wadamesh" for board "LilyGo_TDeck"/);
      return true;
    });
  });
});

// A build carries all three names and they are sent together, so a call cannot
// land on a different build that happens to share a label.
test("a build's three names travel together", async () => {
  let params;
  await withFake(answers({
    "firmware.details": (p) => { params = p; return p; },
  }), async (wb) => {
    await wb.firmware.details(
      { version: "wadamesh", role: Role.COMPANION_RADIO, board: "LilyGo_TDeck" });
  });
  assert.deepEqual(params,
    { version: "wadamesh", role: Role.COMPANION_RADIO, board: "LilyGo_TDeck" });
});
