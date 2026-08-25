# Every verb, and the call that covers it

The wire underneath [scripting-api.md](scripting-api.md). All **213** verbs
registered on the store at `210d9ec`, plus the two the socket owns itself, with
what each reads, what each returns, and which façade call reaches it.

**The mapping is complete in both directions.** Every verb has a row; every row
either names a call or says why it deliberately has none. A verb added without
a decision here is a verb no client can reach, which is the gap
[#213](https://github.com/MeshBench/meshbench/issues/213) exists to close.

## How this was made, and why that matters

Everything below the prose is generated:

```
tools/verbdoc/verbdoc.py            rewrite the tables
tools/verbdoc/verbdoc.py --check    fail if they are out of date
```

The verb, **takes** and **returns** columns are read out of the tree, not
transcribed: it walks `internal/app/session`, brace-matches each `st.Handle`
body, and collects the `stringField` / `numField` / `m["..."]` reads and the
keys of every `map[string]any` returned. The 🪟 flag is read the same way, off
the `needUI()` guard, so a verb that grows or loses it moves by itself. The
**façade** column is authored in the script, because it is the design decision
this document exists to record and is the one column a machine cannot fill in.

**A verb added with no façade decision fails the script.** That is the point: a
verb no client can reach is a verb nobody outside the workbench can use, and it
is how the MCP server came to ship a tool calling a verb that had been deleted.
The prose above the first table is written by hand and is left alone.

That split is the shape of the manifest in #213. This document is its
prototype, and it already found things nothing else had: `session.journal` is
exposed as an MCP tool and is not registered anywhere, and 77 verbs take
parameters no extraction can see because they are read positionally.

Two verbs are **not** in this table:

- `session.verbs` and `session.snapshot` are answered by the socket rather than
  the store (`internal/app/session/socket.go`), so they are addressable but not
  registered. `wb.verbs()` and `wb.snapshot()`.
- `test.residuals` is registered only inside a test.

## Reading the table

- **takes** — *a bare string* means the verb accepts its one parameter
  unwrapped as well as in an object, via `soleString`. Named parameters are
  given with the type the handler reads them as. `any` means the handler takes
  the value without asserting a type.
- **returns** — the keys of the result object. A dash means the verb answers
  with `nil`, or with something that is not a flat object; several return
  domain structs straight out, which is why the manifest needs a return type
  and not only a key list.
- 🪟 — **windowed sessions only.** These 23 refuse with *"this session has no
  interface attached, so there is nothing to show"* when nothing is attached.
  They are all gated on the same check in `internal/app/session/ui.go`, which
  is most of what [#215](https://github.com/MeshBench/meshbench/issues/215)
  needs and is worth knowing before estimating it.
- *none* — deliberately no façade. The store talking to itself; still reachable
  through `wb.call`.

## What this table shows about the surface

- **213 verbs, 28 of them internal.** The façade covers 185, over roughly 60
  calls once objects and properties absorb the rest.
- **The naming is not regular.** `node.*` and `nodes.*` are both node verbs and
  the split is not singular-versus-plural: `nodes.stats`, `nodes.allow_flood`
  and `nodes.regions` all act on one node. `firmware.set` is by role while
  `node.set_firmware` is by node. The façade regularises this; the wire is left
  alone, because renaming verbs would break every script written against the
  socket so far, including `tools/soak`.
- **Some verbs are a pair.** `link.pair` starts the work and `link.pair_set`
  publishes it; likewise profile, coverage, plan, import, feed and shade. A
  client calling only the first gets nothing and concludes the verb is broken.
  The façade waits and returns the answer, which is most of what it is for.

---

### Session and lifecycle

| verb | takes | returns | façade |
|---|---|---|---|
| `app.quit` | — | `closing`, `headless` | `wb.quit()` |
| `log.path` | — | `path` | `wb.log.path` |
| `logs.export` | *a bare string* | `path` | `wb.log.export(path)` |
| `session.describe` | — | `nodes`, `seed`, `now_ms`, `playing` | `wb.describe()` |
| `session.status` | — | — | `wb.status()` |
| `ui.keep_above` | `on` bool | `on` | `wb.keep_above()` |
| `ui.said` | *a bare string* | `said` | `wb.say(text)` |
| `ui.scale` 🪟 | `scale` number | `scale` | `wb.ui.scale = x` |
| `ui.state` 🪟 | — | — | `wb.ui.state()` |

### Project

| verb | takes | returns | façade |
|---|---|---|---|
| `project.list` | — | `projects`, `dir` | `wb.project.list()` |
| `project.new` | `place` string | `nodes`, `place` | `wb.project.new(place=None)` |
| `project.open` | *a bare string* | `opened`, `nodes`, `links` | `wb.project.open(path)` |
| `project.save` | `name` string | `saved`, `path`, `nodes` | `wb.project.save(name)` |

### Nodes

| verb | takes | returns | façade |
|---|---|---|---|
| `node.energy` | *a bare string* | `node` | `node.energy()` |
| `node.output` | *a bare string*, `node` string, `source` string, `lines` number | `node`, `source`, `lines`, `total`, `path`, `tail`, `note`, `tracing` | `node.output(source)` |
| `node.provisioning` | *a bare string* | `node`, `commands` | `node.provisioning` |
| `node.radio` | `node` string | — | `node.radio` |
| `node.radio_adopt` | `node` string | `node`, `tx_dbm` | `node.adopt_radio()` |
| `node.reflash_failed` | *a bare string* | — | *none* — the store telling itself a reflash failed |
| `node.reflashed` | *a bare string* | — | *none* — the store telling itself a reflash finished |
| `node.set_board` | `node` string, `board` string | `node`, `board` | `node.board = ...` |
| `node.set_firmware` | `node` string, `version` string, `board` string, `role` string | `node`, `version`, `board`, `role` | `node.firmware = build` |
| `node.set_firmware_only` | `node` string, `version` string, `board` string, `role` string | `node`, `version`, `board`, `role` | `node.set_firmware(build, apply=False)` |
| `node.start` | *a bare string* | `started` | `node.start()` |
| `node.stop` | *a bare string* | `stopped` | `node.stop()` |
| `node.truerf` | `node` string, `on` bool | `node`, `true_rf` | `node.true_rf = bool` |
| `node.window` 🪟 | *a bare string*, `node` string, `tab` string | `node`, `tab` | `wb.window(node, tab=None)` |
| `node.wipe` | *a bare string*, `node` string | `node`, `wiped`, `removed` | `node.wipe()` |
| `nodes.add_to_selection` | — | `added` | `wb.nodes.select(*names, add=True)` |
| `nodes.allow_flood` | `node` string, `on` bool | `nodes`, `allow_any_flood` | `node.allow_flood = bool` |
| `nodes.delete` | `node` string | `deleted`, `nodes` | `wb.nodes.delete(*names) / node.delete()` |
| `nodes.delete_many` | — | — | `wb.nodes.delete()` |
| `nodes.keep` | — | — | `wb.nodes.keep()` |
| `nodes.list` | — | `nodes`, `count` | `wb.nodes  (iterate)` |
| `nodes.move` | `name` string, `lat` number, `lon` number | `name`, `lat`, `lon` | `node.move(lat, lon)` |
| `nodes.near` | *a bare string*, `node` string, `count` number | `node`, `near` | `wb.nodes.near()` |
| `nodes.place` | `name` string, `kind` string, `board` string, `lat` number, `lon` number, `height_m` number, `tx_dbm` number | `placed`, `kind`, `regions`, `board`, `nodes` | `wb.nodes.place(name, kind, lat, lon, ...)` |
| `nodes.regions` | `node` string, `regions` list | `nodes`, `regions` | `node.regions = [...]` |
| `nodes.search` | `query` string, `limit` number | `query`, `matches`, `total` | `wb.nodes.search() / wb.nodes.find()` |
| `nodes.select` | *a bare string* | `selected` | `wb.nodes.select(name)` |
| `nodes.select_many` | — | `selected` | `wb.nodes.select(*names)` |
| `nodes.stats` | — | `nodes`, `stats` | `wb.nodes.refresh_stats()` |

### Boards

| verb | takes | returns | façade |
|---|---|---|---|
| `board.key` | `node` string, `text` string | `node`, `typed` | `node.board.type(text)` |
| `board.matrix` | `version` string | `version`, `boards` | `wb.boards.matrix(version)` |
| `board.press` | `pin` number, `node` string, `down` bool | `node`, `pin`, `down` | `node.board.press(pin, down)  /  .tap(pin)` |
| `board.probe` | `board` string, `version` string | `probing`, `board`, `version` | `wb.boards.probe(board, version)` |
| `board.probe_finished` | `board` string, `version` string | `board`, `passed`, `failed` | *none* — a probe worker reporting back |
| `board.screen` | `node` string | `node`, `has_screen`, `width`, `height`, `bpp`, `on`, `lit` | `node.board.screen  (numbers, not a picture)` |
| `board.touch` | `x` number, `y` number, `node` string, `down` bool | `node`, `x`, `y`, `down` | `node.board.touch(x, y, down) / .tap_at(x, y)` |

### Simulation

| verb | takes | returns | façade |
|---|---|---|---|
| `sim.faster` | — | `step_ms` | `wb.sim.faster()` |
| `sim.inject` | *a bare string* | `at` | `node.inject(payload=None)` |
| `sim.kind` | `real` bool | `real`, `running` | `wb.sim.real_firmware = bool` |
| `sim.pause` | — | `playing` | `wb.sim.pause()` |
| `sim.play` | — | `playing` | `wb.sim.play()` |
| `sim.reset` | — | `seed`, `now_ms` | `wb.sim.reset()` |
| `sim.run` | `for_ms` number | `running`, `until_ms`, `now_ms` | `wb.sim.run(ms=|seconds=|minutes=)` |
| `sim.seed` | `seed` number | `seed` | `wb.sim.seed = n` |
| `sim.settle` | `steps` number | `now_ms`, `steps` | `wb.sim.settle(steps=...)` |
| `sim.slower` | — | `step_ms` | `wb.sim.slower()` |
| `sim.speed` | `step_ms` number, `factor` number | `step_ms` | `wb.sim.step_ms = n  /  wb.sim.faster(x)` |
| `sim.start` | — | `playing`, `warming`, `starting_firmware`, `started_firmware` | `wb.sim.start()` |
| `sim.state` | — | `playing`, `now_ms`, `until_ms`, `events`, `step_ms`, `seed` | `wb.sim.state()` |
| `sim.step` | — | `now_ms` | `wb.sim.step()` |
| `sim.toggle` | — | `playing` | `wb.sim.toggle()` |
| `sim.unverified_wiring` | `on` bool | `on` | `wb.sim.unverified_wiring = bool` |

### Firmware

| verb | takes | returns | façade |
|---|---|---|---|
| `firmware.build` | `source` string, `from` string, `role` string, `label` string | `building`, `source`, `job` | `wb.firmware.build()` |
| `firmware.build_failed` | *a bare string* | — | *none* — the build worker telling the store it failed |
| `firmware.built` | `version` string | `built` | *none* — the build worker telling the store it finished |
| `firmware.delete` | `path` string | `deleted` | `wb.firmware.delete(build)` |
| `firmware.details` | *a bare string*, `version` string, `role` string, `board` string | `role`, `version`, `board`, `native`, `on_disk`, `path`, `settings_path`, `bytes`, `modified`, `in_use`, `kind`, `bootable`, `flash_mb`, `coproc_at_reset`, `spi_controller`, `notes` | `wb.firmware.details(build)` |
| `firmware.download` | `role` string, `version` string, `board` string | `downloading`, `role`, `version` | `wb.firmware.download(role, version, board=None)` |
| `firmware.failed` | *a bare string* | — | *none* — the firmware starter reporting a failure |
| `firmware.import` | `path` string, `role` string, `board` string, `label` string, `version` string | `version`, `role`, `board`, `path`, `bytes` | `wb.firmware.import_(path, role, board=None, label="")` |
| `firmware.installed` | — | `cache`, `installed` | `wb.firmware.installed` |
| `firmware.library` | — | `builds`, `count` | `wb.firmware.library` |
| `firmware.needed` | — | `roles` | `wb.firmware.needed()` |
| `firmware.published` | — | `published`, `builds` | `wb.firmware.scan()` |
| `firmware.set` | `version` string, `node` string, `role` string | `version`, `nodes`, `considered` | `wb.firmware.use(version, role=|node=)` |
| `firmware.start` | — | `starting` | `wb.firmware.start()` |
| `firmware.started` | — | `running`, `playing` | *none* — the firmware starter reporting back |
| `firmware.state` | — | `running`, `nodes`, `total`, `starting` | `wb.firmware.state()  /  wb.firmware.wait_started()` |
| `firmware.update` | *a bare string*, `version` string, `role` string, `board` string, `label` string, `new_role` string, `new_board` string, `spi_controller` number, `coproc_at_reset` bool, `notes` string | `role`, `version`, `board`, `path`, `renamed`, `repinned`, `settings` | `wb.firmware.update(build, label=|new_role=|coproc_at_reset=|notes=)` |
| `firmware.window` | *a bare string*, `version` string, `role` string, `board` string | `role`, `version`, `board` | `wb.firmware.window(build)` |
| `firmware.wipe` | — | `wiped`, `root` | `wb.firmware.wipe()` |

### Console and fleet

| verb | takes | returns | façade |
|---|---|---|---|
| `console.cli` | `node` string, `command` string | `node`, `reply`, `failed` | `node.companion.cli(line)` |
| `console.read` | *a bare string*, `node` string | `node`, `lines`, `tail` | `node.console.read()  /  node.console.tail` |
| `console.type` | `node` string, `command` string | `node`, `sent`, `note` | `node.console.send(line)` |
| `fleet.replies` | — | `replies` | *none* — the reply collector, called only by its own goroutine |
| `fleet.send` | `command` string, `node` string, `kind` string | — | `wb.fleet.send(command, kind=|node=)` |

### Companion

| verb | takes | returns | façade |
|---|---|---|---|
| `companion.add_channel` | `node` string, `index` number | `asked_for_channel` | `node.companion.add_channel(index)` |
| `companion.advert` | `node` string, `flood` bool | `advert`, `flood` | `node.companion.advert(flood=False)` |
| `companion.configure` | `node` string, `name` string, `lat` number, `lon` number, `tx_dbm` number, `path_hash` number | `set` | `node.companion.configure(...)` |
| `companion.connect` | `node` string | `connected` | `node.companion.connect()` |
| `companion.disconnect` | `node` string | `disconnected` | `node.companion.disconnect()` |
| `companion.raw` | `node` string, `bytes` list | `sent_bytes` | `node.companion.raw(bytes)` |
| `companion.read` | `node` string, `channel` number | `node`, `channel` | `node.companion.messages(channel=)` |
| `companion.refresh` | `node` string | `node` | `node.companion.refresh()` |
| `companion.scope` | `node` string, `scope` string | `node`, `scope` | `node.companion.scope = name` |
| `companion.send` | `node` string, `text` string, `channel` number, `path_hash` number | `sent`, `channel` | `node.companion.send(text, channel=, path_hash=)` |
| `companion.state` | `node` string | — | `node.companion  (properties)` |

### Serving to real clients

| verb | takes | returns | façade |
|---|---|---|---|
| `bench.drop` | `node` string | `dropped` | `node.unserve()` |
| `bench.refresh` | — | — | `wb.endpoints` |
| `bench.serve` | `node` string, `kind` string | `node`, `addr` | `node.serve(kind='tcp'|'serial')` |
| `bench.stray` | — | `at` | `wb.endpoints.stray()` |
| `sdr.serve` | `node` string | `node`, `addr`, `rate_hz` | `node.serve_sdr()` |
| `sdr.stop` | `node` string | `stopped` | `node.unserve_sdr()` |

### Events, packets and capture

| verb | takes | returns | façade |
|---|---|---|---|
| `capture.file` | `path` string | `path` | `wb.capture.start(path)` |
| `capture.stop` | — | `path`, `frames` | `wb.capture.stop()` |
| `capture.wireshark` | — | — | `wb.capture.wireshark()` |
| `events.dump` | *a bare string*, `path` string | `path`, `written`, `total` | `wb.events.dump(path)` |
| `events.recent` | `limit` number | `events`, `total`, `shown` | `wb.events.recent(limit=)` |
| `packet.close` | — | — | `wb.packets.close()` |
| `packet.open` | `id` number, `seek` number | `id`, `origin`, `heard`, `missed`, `transmissions`, `reached` | `wb.packets.open(id, seek=)` |
| `waterfall.capture` | *a bare string* | `captured` | `wb.capture.waterfall(node)` |

### Links, budgets and profiles

| verb | takes | returns | façade |
|---|---|---|---|
| `budget.for_selection` | — | `budgets` | `wb.links.budget()` |
| `link.pair` | `a` any, `b` any | `from`, `to` | `wb.links.pair(a, b)` |
| `link.pair_set` | — | `from`, `to`, `km`, `edges` | *none* — the pair worker publishing its answer |
| `link.profile` | — | `from`, `to` | `wb.links.profile(a, b)` |
| `link.profile_set` | — | `from`, `to`, `km`, `edges` | *none* — the profile worker publishing its answer |
| `links.recompute` | — | `warming` | `wb.links.recompute()` |
| `links.set` | — | `links` | *none* — the warm publishing its matrix |
| `study.margin` | `km` number | `km` | `wb.study.margin_km = n` |

### Coverage and planning

| verb | takes | returns | façade |
|---|---|---|---|
| `coverage.clear` | — | — | `wb.study.clear_coverage()` |
| `coverage.combined` | `mode` string, `combined` any | — | `wb.study.coverage(mode='combined')` |
| `coverage.compute` | *a bare string* | — | `wb.study.coverage(node)` |
| `coverage.failed` | *a bare string* | — | *none* — the raster worker reporting a failure |
| `coverage.map` | — | — | `wb.study.coverage_map()` |
| `coverage.resolution` | `cells` number | `cells` | `wb.study.coverage_cells = n` |
| `coverage.set` | — | `node` | *none* — the raster worker publishing its answer |
| `coverage.start` | `mode` string | `mode`, `nodes`, `started` | `wb.study.coverage(mode=)` |
| `energy.for_selection` | — | `node` | `wb.study.energy()` |
| `plan.failed` | *a bare string* | — | *none* — the planner reporting a failure |
| `plan.routes` | — | `from`, `to` | `wb.study.plan(a, b)` |
| `plan.set` | — | `routes` | *none* — the planner publishing its answer |

### Boundary, import and feeds

| verb | takes | returns | façade |
|---|---|---|---|
| `boundary.accept` | `name` string | `accepted`, `areas` | `wb.boundary.accept(name)` |
| `boundary.list` | — | `areas`, `names` | `wb.boundary.list()` |
| `boundary.load` | `path` string, `geojson` string, `name_field` string, `name` string | `loaded`, `areas`, `polygons` | `wb.boundary.load() / wb.boundary.use()` |
| `boundary.prune` | `margin_km` number | `removed`, `nodes` | `wb.boundary.prune(margin_km=)` |
| `boundary.remove` | `name` string | `removed`, `areas` | `wb.boundary.remove(name)` |
| `boundary.set` | `query` string | `found`, `names` | `wb.boundary.search(query)` |
| `feed.failed` | *a bare string* | — | *none* — the feed reporting a failure |
| `feed.pull` | *a bare string*, `url` string | `url` | `wb.feed.pull(url)` |
| `feed.set` | — | `receptions` | *none* — the feed publishing receptions |
| `feed.stop` | — | `stopped` | `wb.feed.stop()` |
| `import.commit` | `strategy` string | `nodes`, `strategy` | `wb.import_.commit(strategy=)` |
| `import.describe` | *a bare string* | `url` | `wb.import_.describe(url)` |
| `import.failed` | *a bare string* | — | *none* — the fetch reporting a failure |
| `import.fetch` | `url` string | `records`, `nodes`, `skipped_no_position`, `uncertain` | `wb.import_.fetch(url)` |
| `import.set` | — | — | *none* — the fetch publishing its preview |
| `import.set_source` | `url` string | `url` | `wb.import_.source = url` |
| `infer.apply` | — | `applied` | `wb.import_.apply_inference()` |
| `infer.progress` | — | — | *none* — the traffic reader saying how far it has got |
| `infer.result` | — | `packets`, `nodes`, `regions` | `wb.import_.inference` |
| `infer.run` | `hours` number | `reading`, `hours` | `wb.import_.infer(hours=)` |

### Experiments and sweeps

| verb | takes | returns | façade |
|---|---|---|---|
| `experiment.base` | `run_for_ms` string, `send_at_ms` number | — | `wb.experiment.base(...)` |
| `experiment.compare` | `arm_a` string, `arm_b` string | `a`, `b`, `delta`, `note` | `wb.experiment.compare(a, b)` |
| `experiment.define` | `run_for_ms` number, `send_at_ms` number, `spread_ms` number, `bytes` number, `label` string, `repeater_version` string, `companion_version` string, `scope` string, `arms` list, `seeds` list, `senders` list | — | `wb.experiment.define(...)` |
| `experiment.export` | `path` string | `path`, `bytes` | `wb.experiment.export(path)` |
| `experiment.finished` | — | `runs`, `warning` | *none* — the sweep runner reporting it finished |
| `experiment.results` | `arm` string | — | `wb.experiment.results(arm=)` |
| `experiment.seeds` | `seeds` list | — | `wb.experiment.seeds = [...]` |
| `experiment.senders` | `senders` list | — | `wb.experiment.senders = [...]` |
| `experiment.start` | — | `running`, `runs` | `wb.experiment.start()` |
| `experiment.state` | — | — | `wb.experiment.state()` |
| `experiment.stop` | — | `stopped`, `done`, `total` | `wb.experiment.stop()` |
| `experiment.vary` | `parameter` string, `values` list | — | `wb.experiment.vary(parameter, values)` |
| `sweep.run` | — | `arms`, `seeds` | `wb.sweep.run()` |
| `sweep.set` | — | — | `wb.sweep.set(...)` |

### Validation

| verb | takes | returns | façade |
|---|---|---|---|
| `validate.calibrate` | `db` number | `db`, `links` | `wb.validate.calibrate(db=None)` |
| `validate.compare` | — | `matched`, `unmatched`, `median_db`, `iqr_db`, `suggested_excess_loss_db` | `wb.validate.compare()` |
| `validate.failed` | *a bare string* | — | *none* — the observation fetch reporting a failure |
| `validate.fetch` | `url` string, `hours` number | `fetching`, `hours` | `wb.validate.fetch(url, hours=)` |
| `validate.uncalibrate` | — | `db` | `wb.validate.uncalibrate()` |

### The radio model

| verb | takes | returns | façade |
|---|---|---|---|
| `environ.failed` | *a bare string* | — | *none* — the building fetch reporting a failure |
| `environ.fetch` | `source` string | `source`, `started` | `wb.rf.fetch_environment(source)` |
| `environ.fetched` | *a bare string* | — | *none* — the building fetch reporting success |
| `environ.list` | — | `dirs`, `current` | `wb.rf.environments` |
| `radio.preset` | `preset` string, `node` string | `presets`, `preset`, `nodes` | `wb.radio.presets  /  wb.radio.apply(preset, node=)` |
| `rf.environment` | `dir` string, `on` bool | `environment` | `wb.rf.environment = dir` |
| `rf.excess_loss` | `db` number | `db`, `links` | `wb.rf.excess_loss_db = n` |
| `rf.mode` | `mode` string | — | `wb.rf.mode = 'calculated'|'waveform'` |
| `rf.realism` | `osc_ppm` number, `multipath_db` number, `fading_hz` number, `impl_loss_db` number, `saturation_dbm` number | `realism` | `wb.rf.realism(...)` |
| `rf.toggle` | — | — | `wb.rf.toggle()` |

### Provisioning, schedule and assertions

| verb | takes | returns | façade |
|---|---|---|---|
| `assert.add` | `kind` string, `node` string, `at_least` number, `at_most` number, `max_pct` number, `within_ms` number | `assertions` | `wb.assertions.add(kind, ...)` |
| `assert.check` | — | `passed`, `total`, `results` | `wb.assertions.check()  ->  Report` |
| `provisioning.apply` | — | `nodes` | `wb.provisioning.apply()` |
| `provisioning.get` | — | — | `wb.provisioning.settings` |
| `provisioning.set` | `loop_detect` string, `cad` string, `extra` string, `advert_hops` number, `advert_minutes` number, `stagger_ms` number, `flood_max_advert` number, `path_hash_mode` number, `comp_path_hash_mode` number | — | `wb.provisioning.set(...)` |
| `run.save` | *a bare string* | `path` | `wb.save_run(path)` |
| `schedule.add` | `node` string, `command` string, `at_ms` number, `every_ms` number | `sends` | `wb.schedule.add(node, command, at=, every=)` |
| `schedule.clear` | — | `cleared` | `wb.schedule.clear()` |

### Machine resources

| verb | takes | returns | façade |
|---|---|---|---|
| `gpu.set` | `on` bool | — | `wb.gpu.enabled = bool` |
| `gpu.state` | — | — | `wb.gpu` |
| `job.cancel` | *a bare string*, `id` string | `stopping` | `wb.jobs[id].cancel()` |
| `job.done` | *a bare string* | — | *none* — a worker retiring its own progress row |
| `job.progress` | — | — | *none* — a worker reporting progress; read wb.jobs instead |
| `resource.fetch` | `name` string, `version` string, `kind` string | `fetching`, `version` | `wb.resources.fetch(kind, name, version)` |
| `resource.fetched` | `name` string, `version` string | `name` | *none* — the downloader reporting it finished |
| `resource.licence` | `name` string, `version` string, `kind` string | `name`, `version`, `text` | `wb.resources.licence(kind, name, version)` |
| `resource.licence.hide` | — | `hidden` | *none* — closing a box only a window has |
| `resource.list` | — | `rows` | `wb.resources` |
| `resource.remove` | `name` string, `version` string, `kind` string | `removed` | `wb.resources.remove(kind, name, version)` |
| `terrain.cache` | `gb` number | `gb`, `dir` | `wb.terrain.cache_gb = n` |
| `terrain.cache_dir` | *a bare string*, `path` string | `dir`, `moving`, `to` | `wb.terrain.cache_dir = path` |
| `terrain.cache_moved` | `files` number, `dir` string | `dir` | *none* — the cache mover reporting it finished |
| `terrain.prefetch` | — | `tiles`, `to_fetch`, `bytes_rough` | `wb.terrain.prefetch()` |
| `terrain.shade` | — | `shading` | `wb.ui.map.shade()` |
| `terrain.shade_failed` | — | — | *none* — the hillshade worker reporting a failure |
| `terrain.shade_set` | — | — | *none* — the hillshade worker publishing its raster |

### The window

| verb | takes | returns | façade |
|---|---|---|---|
| `layout.reset` 🪟 | — | `reset` | `wb.ui.layouts.reset()` |
| `map.basemap` | `id` string | `id` | `wb.ui.map.basemap = id` |
| `map.centre` 🪟 | *a bare string*, `node` string, `lat` number, `lon` number, `zoom` number | `lat`, `lon`, `zoom` | `wb.ui.map.centre(node=|lat=, lon=, zoom=)` |
| `map.filter` 🪟 | `query` string | `query` | `wb.ui.map.filter = query` |
| `map.fit` 🪟 | — | `nodes` | `wb.ui.map.fit()` |
| `map.layer` 🪟 | *a bare string*, `name` string, `on` bool | `layers` | `wb.ui.map.layers[name] = bool` |
| `map.layers` 🪟 | — | `layers` | `wb.ui.map.layers` |
| `map.zoom` 🪟 | `factor` number | `factor` | `wb.ui.map.zoom(factor)` |
| `panel.close` 🪟 | `name` string | `panel` | `wb.ui.panel(name).close()` |
| `panel.dock` 🪟 | `name` string | `panel`, `where` | `wb.ui.panel(name).dock()` |
| `panel.open` 🪟 | `name` string | `panel` | `wb.ui.panel(name).open()` |
| `panel.pop_out` 🪟 | `name` string | `panel`, `where` | `wb.ui.panel(name).pop_out()` |
| `panels.list` 🪟 | — | `panels`, `count` | `wb.ui.panels` |
| `tool.set` 🪟 | `name` string | `tool` | `wb.ui.map.tool = name` |
| `view.delete` 🪟 | `name` string | `deleted` | `wb.ui.layouts.delete(name)` |
| `view.list` 🪟 | — | `views` | `wb.ui.layouts` |
| `view.load` 🪟 | `name` string | `loaded` | `wb.ui.layouts.load(name)` |
| `view.save` 🪟 | `name` string | `saved` | `wb.ui.layouts.save(name)` |
| `window.close` 🪟 | `name` string | `closed` | `wb.ui.window(name).close()` |
| `window.open` 🪟 | `name` string | `window` | `wb.ui.window(name).open()` |
| `workspace.set` 🪟 | *a bare string*, `view` string | `view` | `wb.ui.view = name` |
