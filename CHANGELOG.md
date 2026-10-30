# Changelog

Notable changes to MeshBench, newest first.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and
the version numbers follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**MeshBench is 0.x deliberately, and there is no 1.0 scheduled.** While the
major version is 0 the interface may change between releases, and this file is
the only notice: there is no deprecation period behind it. Three things are the
exception, because a mismatch there is refused at runtime rather than guessed
at: the control protocol number, the rule that a client and the workbench it
drives are the same release, and the fixture format, which refuses a file
written by a later build. [`docs/compatibility.md`](docs/compatibility.md) says
what each of those promises, and what would have to be true before 1.0 was
worth cutting.

**The 0.0.x entries below were reconstructed from the commit history.** Those
four releases shipped with identical installation notes and no record of what
had changed in them - which is the gap this file exists to close.

## [Unreleased]

### Added

- **Script input at an instant of simulated time.** `console.type`,
  `console.cli` and the `companion.*` verbs that write to a node take `at_ms`:
  the call is held and runs when the run reaches that instant, so a script
  configuring a mesh while it plays lands every line at the same moment on
  any machine. Without it, a line typed at a playing clock lands wherever the
  clock happens to be when the call arrives, which is how a loaded host
  changed a native run's results. The Python, Go and JavaScript clients' console
  `send` take it too.

### Changed

- **The Windows installer asks where to go and says when it has finished.**
  It is built by the WiX toolset again, now on a Windows runner, which is the
  only place WiX can build one: a Destination Folder page with a Change button,
  a completion page, the brand on both, and the install location in Apps and
  Features. Staging stays on Linux, and the tables are read back there before
  the release takes the installers.
- **`sim.state` says when the wall clock decided something.** `reproducible`
  is false, with the reason, once a node-writing verb ran at a playing clock
  without `at_ms`, a client outside the simulation wrote to a served
  companion, or a node was dropped for missing a tick's deadline.

### Fixed

- **Native nodes see CRC errors and a busy channel.** The chip model,
  virtual-sx1262 v1.5.0, answers the four-byte status reads RadioLib sends in
  full. Native MeshCore builds made on it no longer repeat a frame that failed
  its CRC as a good one, and listen-before-talk sees preamble and header
  detection. Emulated boards pin the same version.
- **Renode's chip binding is safe with a newer chip library.** Its
  `VsxState` and `VsxCounters` were laid out for virtual-sx1262 ABI 1.3 while
  the version check accepted any later minor, so a 1.4+ library wrote past
  the end of both. They now follow the 1.7 header, a library newer than that
  is refused, and the sizes are checked against C's on load. Renode nodes also
  report `irq_suppressed` from a chip that counts it.
- **A Linux tarball install knows it is one.** The updater recognised the
  tarball by a `radioserver` beside the binary, which no tarball has shipped
  since the chip became a library, so every tarball install reported itself
  as unpackaged and got a generic instruction instead of its own. It now looks
  for `VARIANT`.
- **Headless echoes every status line.** It printed the latest status line at
  each look, so several lines said close together lost all but the last, and
  a line said just before exit was lost. A failed update check left nothing in
  the log at all.
- **The compact macOS build carries the chip model and Renode's support
  files.** Both were added after the compact image was cut, so a compact Mac
  build could start no emulated board at all, and after fetching Renode still
  no nRF52 one. They now go in first, and the compact tree is checked by
  `verify-bundle.sh` like the other platforms' are.
- **A board's cached capability row names the emulator it ran under.** On a
  machine with both emulators set up, every nRF52 row was stamped with QEMU's
  binary, so upgrading Renode retired none of them and upgrading QEMU retired
  all of them. The fingerprint now follows the board, and also covers the chip
  library and, for Renode, our peripherals. Rows cached by an earlier build
  read as stale once.
- **Adding a scheduled send no longer repeats the ones already said.** Any
  `schedule.add` made every one-shot send whose moment had passed go out
  again.
- **A reset or new seed runs the schedule again.** The schedule remembered
  what it had fired across a rebuild, so the second run of a fixture was
  silent.
- **Scheduled sends fire during `sim.step`, `sim.settle` and a console
  reply's settle**, not only while playing, so one that falls due there is
  not held until the clock next plays.
- **One step moves the clock one step.** It moved nothing.
- **Closing the workbench no longer races a rebuild.** A signal or quit read
  and cleared the engine off the store's goroutine, so a close landing during
  a rebuild could miss the new engine and leave its firmware running. The
  engine is now handed over by the store before it is closed.
- **The Events panel shows its rows when docked in a side rail.** Its
  columns were wider than the rail, so the header wrapped a letter to a line
  and left room for one row. A panel too narrow for the full row now lays
  rows out the way the Inspector does.
- **The firmware library says "checking" until the catalogue answers.** A
  build not yet downloaded was marked unavailable while the catalogue was
  still being asked, and for good if it could not be reached, so a first run
  opened on a library of builds that looked unpublished. A row whose half of
  the catalogue is unreachable now offers a retry. `firmware.library` rows
  carry `checking` and `catalogue_down`.
- **GPU coverage no longer depends on the driver's trig.** The fold kernel's
  bearing cancelled two near-equal numbers, so a driver whose `sin` and `cos`
  are only as accurate as Vulkan requires, as on an Intel UHD 620, put a
  directional antenna's gain 0.9 dB out. The bearing, distance and elevation
  now use the kernel's own series.

## [0.3.0] - 2026-10-24

What a companion does beside flooding, and what a radio is told when a frame
goes wrong. A companion can now message, log in and ask for status the way a
phone does, and a receiver that locks onto a frame and loses it hears about it
as an SX1262 would, instead of hearing nothing.

### Added

- **A companion's command line does more than flood.** `msg`, `login`,
  `req_status` and `scope` work as meshcore-cli has them, typed in the
  companion's CLI tab or scheduled in a fixture. Each is answered in the
  transcript when the firmware takes it and again when the ack or reply
  arrives, with the simulated time; a message whose ack never comes says so
  once the firmware's wait is over. `-companion-mode Client|CLI|TCP` opens the
  companion window on that tab. (#753)
- **Each node's airtime and duty cycle from a script.** `nodes.stats` rows
  carry `airtime_ms` and `duty_cycle_pct`, the numbers the Scores panel
  already showed. (#754)
- **A frame the receiver locked onto and lost reaches its chip.** A collision
  in the header is a header error, a payload lost after a good header is a CRC
  error, and a frame the receiver detected but never locked onto is reported
  as that. Native firmware built from meshcore-native from this release takes
  them; older native, Renode and QEMU nodes are sent nothing new and behave as
  before. A collision's miss now says whether it hit the header. (#757)

### Changed

- **The published native firmware is rebuilt on virtual-sx1262 v1.4.0**, for
  repeater, companion and room server at v1.17.1 and v1.18.0. Its receiver
  takes only frames it was listening for: none that began while it was in
  standby, in CAD or transmitting, and its detection flags stay set until the
  firmware clears them, as silicon does. Results of runs with collisions in
  them move with it, and there is no switch back in this release.
- **`experiment.define` reads every setting an arm names** (`cad`,
  `loop_detect`, `path_hash_mode`, `rep_path_hash`, `spread_ms`, `set`) and
  refuses a field it does not know with `bad_params`. Before, everything past
  the label and versions was dropped, and the arm ran as the scenario had it.
  (#758)
- **`console.read` on a companion held by its session returns the
  transcript**, not the raw serial bytes. (#753)
- **Built with Go 1.27, so the macOS app needs macOS 13 Ventura or later.**
  (#767)

### Fixed

- **The speed control changes the speed.** Slower, faster and `sim.speed`
  only ever changed the label; a tick now covers its size in engine steps,
  bounded so the interface stays responsive on a large mesh. (#755)
- **`radio.preset` kept the preset's coding rate.** Every preset came out four
  too high (EU/UK Narrow became 4/12), and a companion refused it. A placed
  node joins the modem its nearest neighbours are on rather than the engine's
  own SF10 at 250 kHz, which nothing else used. (#756)
- **Import keeps to the study area it shows.** An opened network's areas were
  listed in the Import panel and not applied, a blank network kept filtering
  by the last one's, and of several places with one name the first was always
  taken. Places are now offered by their full name. (#766)
- **Typing in a question over the map.** Backspace and Delete reached the
  map instead of the field, and with a node selected asked to delete it; a
  space pressed play. (#766)
- **The lint ratchet fails when lint did not run**, instead of reading a
  missing linter as a clean tree and, with `--update`, erasing the baseline.
  (#760)
- **Board report cache keys stay inside the cache.** (#761)
- **A build's label lookup cannot hang**: the git call that names a local
  build has a deadline. (#763)
- **Every GPU kernel has a CPU twin and a comparison**, enforced, and a
  non-finite value is a disagreement rather than a pass. (#762)

## [0.2.0] - 2026-10-14

What the radio was told, beside what the mesh did. Two firmwares can flood a
network identically while one of them is blind to half of what its chip saw;
this release puts the chip's own counters where that difference can be read.

### Added

- **What each node's IRQ mask kept out.** `nodes.stats` carries
  `irq_suppressed`: events the chip raised that the firmware's IRQ mask kept
  out of the register, and absent rather than zero for a chip model that does
  not keep the count. The node window's Radio tab shows it as **IRQ
  suppressed**, directly under the masks, in amber above zero and as "not
  counted" for an older chip; **spurious detections** joins it under busy
  time. (#745, #749)
- **Every experiment cell records what each node's chip saw.** Each cell of
  `experiment.export` carries `chip`: interrupt reads, busy reads, busy time,
  spurious detections and suppressed events for every node, measured from the
  cell's own baseline as its delivery totals are, plus the mask and modem
  settings each chip ended on. `experiment.results` arm summaries carry the
  `irq_suppressed` and `busy_ms` means, and the sweep's **Results** table shows
  them as **IRQ suppressed** and **busy time**, beside **delivered**, as a dash
  where no seed's chip counted them. (#746, #749)

### Changed

- **The published native firmware is rebuilt on virtual-sx1262 v1.3.3.** Native
  builds are downloaded at runtime rather than shipped in this binary, so this
  reaches an existing install too. The chip model enforces the
  IRQ mask, raises HeaderValid once the programmed preamble, sync word and
  header have gone by rather than twelve symbols in, and raises HeaderValid
  alone for a LoRa header, so a firmware that rightly leaves the GFSK-only
  SyncWordValid out of its mask is no longer counted as missing every header.

## [0.1.0] - 2026-09-09

The first public stable release. It carries everything the 0.0.x line built -
the RF-accurate channel, real MeshCore firmware native and emulated, the study
and experiment verbs, the two release channels - and closes the batch of fixes
the pre-release passes found.

### Added

- **`node.regions` reads a running node's real region map.** A region set
  straight over a node's own console lands in the firmware and never in the
  model, so `nodes.list` reported none for a node that held one. `node.regions`
  asks the running firmware (`region list allowed`, `region default`), parses
  the reply, and writes the model; `nodes.list` also carries `default_scope`
  now.

### Fixed

- **The calculated noise floor fluctuates, so seeds move a run.** Calculated
  reception was a pure function of geometry, so every seed produced the
  identical run and a sweep's `rx_spread` - the figure every experiment delta
  is read against - was structurally zero. The floor now carries a per-reception
  Gaussian fluctuation (σ = 2 dB, tunable), so a link within a couple of dB of
  threshold decodes on one seed and misses on the next. The run stays
  reproducible, and the waveform path is untouched.
- **Regions applied to a running mesh stay in the view.** They reached the
  scenario and the firmware - the mesh relayed scoped traffic - but vanished
  from `nodes.list` within a tick, so the one place the manual says to read
  regions answered "none" on a mesh where every node held one. The snapshot's
  region columns are now re-projected from the scenario every readout, the way
  the scores and trails already are, so a row cannot drift from the scenario
  for longer than a readout.

## [0.0.11] - 2026-09-09

### Added

- **Two release channels.** A plain tag - `v0.0.11` - is the stable channel;
  a tag with a pre-release suffix - `v0.0.11-dev.3` - is the development
  channel, published as a GitHub pre-release and never offered to a stable
  build, because the release page's own redirect never names one. A build
  follows its own channel unless told otherwise: `update.channel` reads or
  changes it, and it is remembered in the settings file. A development build
  says "development build" beside its version in the status bar, since a
  screenshot is what reaches an issue. The apt repository and the Homebrew
  tap carry stable only. `dev cut` in Actions tags the next development build
  from `main` with one press; a stable tag stays a deliberate act by hand.
- **A pre-release is a release of its own.** The same-release rule between a
  client and a workbench compares the whole tag, so a stable client meeting a
  development workbench is refused as a mismatch rather than let through as
  though one end were a working copy.

- **`meshbench test` boots every node factory-fresh.** The same fixture,
  build and seed gave 350 deliveries on a fresh machine and 322 on every run
  after it, with an identical header, because the second run's nodes loaded
  what the first had stored. A test run now gets node storage of its own,
  removed afterwards, so the number depends on the fixture, the build and
  the seed alone; `-keep-node-storage` reuses the machine's storage as
  hardware would, and the header says which it did.
- **A development build has development clients.** The Python and Node
  clients are published beside a development build as pre-releases:
  `0.0.11.dev1` on PyPI, which pip installs only with the exact pin or
  `--pre`, and `0.0.11-dev.1` on npm under the `dev` dist-tag, so a plain
  install keeps resolving to stable. The Python client says the tag's
  spelling on the wire, and a development workbench's refusal says its
  client is a pre-release that has to be asked for.

- **A sweep records which build each arm ran.** Its results carried the arm,
  the seed and the measurements and nothing about the firmware, so two arms
  that came back identical could not be told from two arms that never switched
  build. Each run and each arm summary now carries `builds`: the firmware every
  role actually ran, as `{role, version, file, bytes}`. The file and its size
  tell two builds under one label apart, since `local-main` is rebuilt in
  place.

### Fixed

- **Every native node drew the same receiver noise.** The chip model's noise
  seed was never set, so every node ran one stream from one counter, and each
  node's RNG - its CSMA backoff and advert jitter - was seeded off it and
  correlated across the whole mesh. Each node now draws its own, keyed on its
  identity seed, so contention is between independent nodes. The native
  firmware binaries carry it from their release.
- **A native node's identity is keyed on its name, not its index.** Nodes were
  seeded by their position, so node 0 of every fixture with the same seed
  shared a keypair with node 0 of every other, and two fixtures opened in turn
  into one profile saw each other's nodes as themselves. The seed is now
  derived from the run seed and the node's name, so a node is the same across
  runs of one fixture and different from every other's.
- **Regions applied to a running node reach the node.** `nodes.regions` and
  `infer.apply` wrote the region set to the scenario and the map and typed
  nothing into the firmware, so a node whose firmware was already up relayed
  under whatever it booted with. The change now reaches a running node's
  console, and both verbs report how many running nodes were re-provisioned.
- **Waveform mode names a collision, and a passing CRC is a reception.** A
  frame whose CRC matched but whose FEC flagged a codeword was recorded as a
  miss whose detail read "CRC true"; it is accepted now, as MeshCore accepts
  it. And a miss above the noise floor with a stronger overlapping signal was
  left unclassified - waveform mode never named a collision the calculated
  model could not - so it now reads interference, from the interferer the
  summed window already held.

- **A build pinned to nodes of the wrong role is refused, not run as half a
  mesh.** `firmware.set` with a version and no role filter pinned a repeater
  build onto companions too, answered as though all took it, and the run
  started only the repeaters with nothing said. The build's role is now
  checked against each node's: `firmware.set` leaves a mismatched node alone
  and reports how many, `firmware.needed` counts it as still needing a build,
  and `firmware.start` refuses with the reason. A build whose role cannot be
  known, a local label, is pinned as before.
- **A companion shows the contacts it has learned.** A companion loaded its
  contacts at boot and reported none to every client, because the native
  build compiled two of MeshCore's files with different values of a
  contact-table size and they disagreed on where the count lived. Fixed in
  the firmware build; the companion binaries carry it from their next
  release.
- **Every native node on Windows shared one identity.** The bridge parsed
  `--seed` with `strtoul`, which saturates where a long is 32 bits, so every
  node whose seed was above 2^32 was seeded `0xFFFFFFFF`: 57 of 58 nodes,
  private key included, and no companion ever learned a contact. The seed
  now goes down as its low 32 bits, which is all the firmware ever read, so
  nothing changes on Linux or macOS and the published binaries are right
  without a rebuild.
- **A mesh with no regions applied floods, and the manual said it did not.**
  Skipping `infer.apply` was documented in a dozen places as a mesh that
  transmits everything and relays nothing. MeshCore relays every unscoped
  flood, adverts included, whatever a node holds; what a region-less mesh
  drops, silently, is scoped traffic, and nothing it originates is scoped in
  the first place. Every copy now says so. The same reading settles the
  permissive fixture: `region allowf *` changes nothing on a fresh node and
  never makes a scoped flood forward, which is why strict and permissive
  measured the same.
- **The screenshot sweep photographed the workbench under a popout's name on
  macOS.** The window chooser skipped every floating window of ours, and
  46 of 115 steps filed the main window under another window's name with a
  clean exit. The chooser tests size alone now, and on every platform a
  picture of another window that comes back at the workbench's own size is
  removed and reported rather than filed.
- **The emulator pin moves to `sx1262-15`, and ESP32 boards boot again.** The
  UART's `apb_freq` was a static qdev property, and the classic ESP32's clock
  handler sets it when the second-stage bootloader switches to the PLL - long
  after realize, where qdev aborts rather than refuses. Every ESP32 board on
  Linux, macOS and Windows died at hand-over on "Attempt to set property
  'apb_freq' ... after it was realized", with an empty console because the
  firmware had not printed a line yet. The property is now registered the way
  the two timer devices beside it always have been, and the fork's CI runs the
  binary against every device that clock handler writes to, on a pull request
  as well as on a tag.
- **A node dropped from a run says what the emulator said.** "Stopped
  answering and has been dropped from this run" is equally true of a board
  whose firmware hung and one whose emulator never got past reset, and the
  second one's reason was already on disk with nothing pointing at it.

## [0.0.10] - 2026-09-07

The last of the Windows pass, and the board status the published pages were
still getting wrong.

### Fixed

- **Wireshark opens where it is asked to, on every platform.** The launch was
  written for Linux: it named `lo`, which is Linux's word for the loopback
  interface. macOS calls it `lo0`, and Windows has none at all until Npcap
  supplies `\Device\NPF_Loopback`. Two of the three platforms this ships on
  were pointed at an interface that does not exist, and macOS had been broken
  quietly the whole time. The command handed back when the launch fails is now
  the one that would have worked.

- **A basemap cache that already holds watermarked tiles is left behind.**
  Fetching keyed and keyless tiles into separate directories fixed this going
  forward and did nothing for a machine that already had the problem: the keyed
  build still read the old shared directory, where tiles from before the split
  sit with nothing to tell the two kinds apart. Both sides move off it now. The
  old directory is left rather than deleted, because it is a cache the Resources
  page counts and offers to remove, and deleting somebody's files to reclaim a
  megabyte is the worse trade.

- **A node window answers with the tab it settled on.** `node.window` reported
  the tab that was *asked for*. A node whose board declares nothing was asked
  for Hardware and answered `Hardware` while drawing its console, and the
  reference promised that field was the tab the window actually opened on. The
  tab is now settled at open time from the same functions the frame draws
  through, so the answer cannot differ from what appears.

- **The board table says what the boards do.** The front page showed five
  boards as booting, transmitting, hearing and not forwarding, and was missing
  two boards entirely; the shortcomings page still carried a 20 August
  measurement saying one board of twelve passed. Ten of the twelve pass every
  column, and every one of them forwards somebody else's packet. Only
  `Station_G2` and `Heltec_v2` are blank, and those were never attempted.

### Added

- **The documentation site has a board table.** It had none, so the only
  statement about board status anywhere on it was a paragraph in a page about
  shortcomings. It is on
  [Emulating a board](https://meshbench.github.io/docs/emulation.html#which-boards-have-been-run),
  with what each column asks for and why rows are measured one board at a time.

### Infrastructure

- The two publishing jobs no longer upload a Go cache the lab runners already
  hold. The upload does not finish on that uplink, and it cancelled the 0.0.9
  documentation publish after the job had done all of its work, in a post step
  where nothing looks.


## [0.0.9] - 2026-09-06

Everything a full walk of the pre-release pass turned up on the released 0.0.8
binary, run against an empty profile so the application was met the way a new
machine meets it.

### Fixed

- **The refusal for a machine with no firmware says how many nodes it means.**
  It read "no firmware for 5 of 58 nodes ... and 52 more" - a sentence that
  disagrees with itself, because the count was the length of the shortened list
  rather than the number of nodes. The truth on a fresh machine is every node,
  so it understated the gap in the one direction that reads as ignorable. It is
  also the first thing anybody sees after pressing play.

- **Setup no longer opens in front of the map on every launch.** A machine with
  everything it needs, which had simply never answered the update-check
  question, was told "this machine is not set up yet" by the status bar while
  the page it had just opened said "nothing is broken". Both on screen at once,
  and one of them wrong. The page opens only for something that is actually
  blocking; the outstanding question is said in a line instead.

- **A study's terrain verdict asks about the ground the study walks.** It
  judged a grid across the bounding box while the fetch covers the tiles under
  the links, and on a coastal study a fifth of that grid is sea no profile
  crosses. Those tiles were never going to arrive, so the answer was stuck at
  "partial terrain" however much was downloaded. It can reach complete now, and
  a machine that has everything its links need is told so.

- **Three shipped fixtures carried the same study area twice**, byte for byte -
  fife-strict, fife-permissive and fem-e22, each with two identical copies of
  Fife. Everything that walked the areas walked it twice, and the Boundary
  panel listed one place on two rows with the same numbers. The duplicate is
  gone from the files, and a fixture that carries one is corrected on the way
  in.

- **The Events and Inspector panels start under their header.** With fewer rows
  than the pane holds they were pressed against its bottom edge, leaving the
  column header labelling several hundred pixels of nothing - which reads as a
  panel that has not loaded. A run longer than the pane still follows its
  newest row.

- **Labels are cut rather than folded.** A table header narrower than its own
  title laid the word out one character per line, a squeezed button drew its
  name as a vertical strip of letters inside its own outline, and a filter chip
  came out twenty pixels wide with its label in half. A button with no room for
  even an ellipsis drew nothing at all.

- **Every event filter can be reached in a docked panel.** The chips ran off
  the edge with no wrap and no scroll, so three of the eight could not be
  pressed. They wrap now, and in a pane too short for both the per-class
  summary cards give way to them - every count on those cards is on the chip
  that filters it, so what goes is the percentage rather than the figure.

- **The Resources page stops denying what it counts.** Its last line read
  "Nothing here is fetched without being asked" three rows under its own card
  counting the megabytes that had been. It now describes the split its rows
  already draw.

- **"1 assertion".** Three counted headings had no singular, including the last
  line of a passing `meshbench test` - the line CI logs and the one people
  paste into reports.

- **Help reaches the manual.** The menu had no route to the documentation at
  all, which is the one thing a Help menu is for.

### Changed

- **`-node-tab` takes a tab name rather than an index.** The index list was
  written out by hand in three places and two of them had missed a tab added in
  the middle, so five of the capture steps had been photographing the tab after
  the one they were named for and one had never been photographed at all. A
  name cannot drift that way, and an unknown one is refused with the list.

- **`-config-section` and `-licence-section` open their panel** as well as
  scoping it. They had been scoping a panel that was never shown, so eleven
  capture steps produced eleven identical pictures of a different page. An
  unknown section is refused with the list, which five of the six configured
  names turned out to be.

- **`-board-decode` opens the board view's console with its decode tick on**, so
  the one control in that window with no way in from outside can be captured
  without a hand on the mouse.

### Infrastructure

- The pipelines build on the lab runners rather than on hosted ones, and the
  Windows check runs when somebody asks for it. A release still publishes from
  hosted, and so do the package indexes.


## [0.0.8] - 2026-09-06

A release of repairs, most of them found by a person working through the
pre-release pass on Windows.

### Fixed

- **`-look` frames what it is asked for.** The flag's third field was read as
  the camera's own scale, which is pixels per degree, rather than as the zoom
  level on a slippy map - so `-look 56.33,-3.32,16`, meant to be a street,
  asked for sixteen pixels per degree and drew most of the planet. Every level
  anybody tried came out at roughly world scale. The same conversion now
  applies to `map.centre`.

- **Three node names in the shipped fixtures are whole again.** Ten U+FFFD
  replacement characters across four fixtures, the mark of a name cut
  mid-character before it ever reached this repository. What was valid is kept
  and what was lost is dropped, and a check refuses a fixture carrying either
  spelling of that character.

- **Two workbenches can both start.** Asking for any free port was refused when
  another workbench was running, and the refusal named `tcp:127.0.0.1:0` - a
  sentinel meaning "whatever is free", which nothing can hold and nobody types.
  An ephemeral request is never in conflict, and a real conflict now names the
  address that actually answered.

- **A second workbench no longer hides the first.** `control.json` names the
  session a client with no address finds, and a newcomer overwrote it: the
  first kept running and answering, and stopped being reachable by every
  documented route, with nothing raising an error. A live entry is left alone
  now, and the newcomer says so - it is still reachable at its own address and
  still listed among the running sessions. This matters most on Windows, where
  reading that file is the only way anything finds anything.

- **A control connection that greets wrongly is answered.** Putting the token
  inside the first request authorised, had that request read as the greeting,
  and then waited for a reply to something nothing had queued - so the
  connection simply hung.

- **A basemap tile fetched without a key no longer poisons the cache.** CARTO
  serves a tile either way: with a key it is the map, without one it is the map
  under a watermark. The cache recorded neither, so a single run of a locally
  built binary - which has no key, because the key belongs to the release
  pipeline - left watermarked tiles that every release build installed
  afterwards re-served for ever. They are cached apart now, which needs no
  migration and stops both directions.

- **Emoji in node names have a font to fall back to on Windows.** The search
  covered Linux and macOS and no Windows path at all, so a build whose bundled
  copy was missing drew every supplementary-plane emoji as a box while the
  older symbols came through - which reads as mangled names rather than an
  absent font. The bundle check now requires the font as well, in both
  variants: it is fetched with a warning rather than an error, so a failed
  fetch used to ship quietly.

### Changed

- **The pipelines run on this project's own machines.** Eight jobs move off
  GitHub-hosted runners, which a free account pays for in minutes, onto the
  lab. The two that stay are the release itself and the PyPI publish, which
  needs a Docker daemon the lab runners have not got. A workflow a fork's pull
  request can trigger keeps the guard that sends that run to a hosted runner
  instead.

- **The Windows GPU check is dispatched rather than tagged.** What it uniquely
  proves is the kernels on DX12 on a real Windows machine, and that is now run
  when somebody asks. A release is still proven to *build* for Windows on every
  tag, by the cross-compile that packages it.

- **A release refuses to publish without a changelog entry.** The notes link to
  this file at the tag, which is only true if the entry was written before the
  tag was cut - and for 0.0.7 it was not.

- **The release notes say which download is which.** Every platform ships a
  bundled and a compact build, and the notes described one row of six. They
  carry the banner and a link here as well.


## [0.0.7] - 2026-09-06

The release where a board can be looked at rather than only run, and where the
console says what it is actually saying.

### Added

- **The board view: one board, and whether it is behaving like the board its
  profile says it is.** A window of its own, opened from a node's Hardware tab
  or by `node.boardview`. The board's panel at a whole-number scale, every part
  it declares as an index, the controls for everything it has wired, and two
  tables - the radio as the firmware left it, and the wiring as the profile
  declares it - with a verdict on every row. The verdicts keep apart the two
  answers that are both about an absence: a line nothing has happened on, and a
  part we have no model for. Documented at
  [the board view](https://meshbench.github.io/docs/board-view.html).

- **The console strip reads the framed protocol.** A companion's serial carries
  MeshCore's own framing, so a byte at a time it is a wall of escapes with the
  answer buried in it. The `decode` tick reads the frames on screen and prints
  each as a line - name, position, frequency, spreading factor, transmit power -
  and leaves everything that is not a frame exactly as it was, because the
  bootloader is what says whether the board started at all.

- **Type at a board from the window watching it.** A box along the bottom of
  the console, routed by what the node is: a repeater reads typed text, a
  companion speaks meshcore-cli's vocabulary, and text typed at a companion
  through the repeater's console goes nowhere while looking exactly like a
  command that ran and did nothing.

- **The console rate is reported rather than guessed.** The board view shows
  what rate the firmware set its console to, read from the divider the guest
  wrote rather than offered as a list to pick from. A board whose console is
  the USB peripheral says it has no line rate, which is a different fact from
  not knowing.

- **Five nRF52 board profiles**, transcribed from MeshCore's own variants: the
  RAK 4631, the XIAO nRF52, the Heltec Mesh Solar, T096 and T114. The XIAO's
  Arduino pin numbering is resolved through the variant's own map rather than
  assumed to be flat.

- **A capture step for every window.** `tools/shots/steps.json` names one
  picture per panel, view, node-window tab, board-view table, configuration
  section, licence section and menu, and `tools/shots/shots.py` drives the
  binary once per step to take them. A test checks the manifest against the
  application's own panel table, so a panel added without a step is a red
  build.

### Changed

- **The emulator pin moves to `sx1262-14`.** The console rate arrives in the
  chip model's stats record, which the previous pin does not carry, so on it
  every UART board reported nothing. That release also corrects the UART's
  clock: the rate was computed against a hard-coded forty megahertz where the
  part runs at eighty, so every figure was half what the firmware asked for.

- **The manual is part of the change.** A change that alters what somebody sees
  or does now opens a pull request against
  [MeshBench/docs](https://github.com/MeshBench/docs) in the same stroke as the
  code, written as a manual rather than as a changelog. Enforced by review
  rather than by CI, and stated in `CLAUDE.md` and `CONTRIBUTING.md`.

- **Nine packages split out of the session and workbench layers**, each named
  for what it holds: the map verbs, the firmware library, the A/B matrix, the
  node view, the packet inspector, and the shared controls into the packages
  shared things live in. A test enforces that a file is named for its contents.

- **A node's identity is what makes it that node.** An emulated node was
  identified by name alone, so two nodes that swapped names swapped identities
  with them.

### Fixed

- **The T-Deck forwards.** A dropped GPIO input, and a stimulus the board could
  not have heard because it arrived while the board was transmitting.

- **Windows and macOS publishing** calls the apt and tap publishers directly
  rather than waiting for an event that a release does not always raise.


## [0.0.6] - 2026-09-04

The release where every emulated board relays, and where a download says what
is in it.

### Added

- **Every asset says whether it carries the emulators, and both are built.**
  Each platform now publishes a `-bundled` build with QEMU and Renode in it and
  a `-compact` build that is the application alone, from one build and two
  packaging passes over the same tree. Until now the choice existed on Linux by
  accident - the tarball had the emulators, the AppImage and the `.deb` did not
  - and nothing in any name said so. The version has gone from the filenames,
  so `releases/latest/download/<name>` resolves to the current build for ever;
  `meshbench -version` answers which build you have.

- **A download page, at [meshbench.github.io/download](https://meshbench.github.io/download/).**
  The two Download buttons used to drop a first-time visitor onto a release
  listing with no indication which file they wanted. The page detects the
  platform, offers one command that installs through apt or Homebrew, and makes
  bundled-or-compact a single switch that changes every filename and command at
  once.

- **`apt install meshbench` and `brew install --cask meshbench`.** A signed apt
  repository at `meshbench.github.io/apt` and a Homebrew tap at
  `MeshBench/homebrew-meshbench`, both written by a release. The plain name is
  the application on its own in both, because a package manager re-downloads
  every release for a tool you may never point at an emulated board;
  `meshbench-bundled` is there for anyone who would rather spend the bandwidth
  once.

- **Windows can fetch its emulators.** Configuration > Setup could not download
  one there: the check that a download is a program read ELF and Mach-O and
  refused a PE outright, the unpacker opened tars while Renode publishes a zip,
  and the install finished with a symbolic link, which Windows grants only to
  an elevated process. All three are dealt with, the last by looking in the
  same places on both sides of the search so no link is needed.

### Changed

- **The emulators hold the SX1262 themselves.** An emulated node used to run in
  two processes, the second owning the chip and answering SPI over a socket one
  byte at a time. QEMU and Renode now load
  [virtual-sx1262](https://github.com/MeshBench/virtual-sx1262) directly, so
  there is one process per node and the only socket left carries the simulated
  air. DIO1 is pushed by the chip rather than sampled on a millisecond timer.

- **The flood row is judged over several attempts.** One advert deciding a
  board's whole result made a board that relays nine times in ten look broken,
  because its own periodic advert can land badly. It also refuses to measure at
  all when the board and the sender share a public key, which measures the
  harness rather than the board.

### Fixed

- **macOS gets a Renode that can actually start.** The fork's packaging looked
  for a `*portable*.tar.gz` while the macOS script produces a disk image, found
  nothing, and quietly tarred the build tree instead - so the asset had no
  launcher, the bundle carried 83 MB nothing could run, and Configuration >
  Setup declined to offer it at all. The image is now unwrapped to a real
  application, the silent fallback is gone, and the packaging refuses rather
  than substituting. An nRF52 board still has not been booted on macOS; this is
  the half that was in the way.

- **Every nRF52 board now forwards, and it was two faults masking each other.**
  MeshCore verifies signatures in CryptoCell hardware, and our model loaded
  modular operands at the operation's own width while the firmware keeps
  Ed25519 field elements unreduced in wider registers - so every operation was
  arithmetically perfect on numbers the firmware never had, and every advert was
  dropped as unsigned. Separately, three boards read a button pin that is held
  high by a resistor on the real hardware and low in the emulator, so MeshCore
  saw a long press and powered the node off seconds after boot. RAK4631 and
  Xiao_nrf52 were unaffected by the second, which is why fixing the first made
  those two work while others still failed.

- **Every ESP32-S3 board now relays.** Three register-map faults kept DIO1, the
  packet-received line, from reaching the firmware: the per-pin configuration
  registers start at a different offset on the S3, so the pin was never armed;
  the GPIO block's interrupt output was never wired to the interrupt matrix;
  and the pin decode had to win over an overlapping register range. The boards
  received every frame and forwarded none.

- **Two emulated nodes are two nodes.** Every emulated board came up with the
  same keypair, byte for byte, so a mesh of them was one node repeated and their
  adverts were duplicates any receiver was right to drop. MeshCore takes entropy
  from the radio, and our chip had none to give. It has per-node receiver noise
  now, seeded from the run so the result stays reproducible.

- **No published board image could be downloaded.** The catalogue derived a
  version from the asset name while MeshCore tags releases by role, so every
  image request 404ed and nobody without a warm cache could start an emulated
  board.

- **The rx row could claim a reception the firmware never saw.** The engine
  records a delivery before the firmware reads it, so a board passed `rx` while
  its driver never collected the packet. The row now waits for the board to stop
  talking first, owns its own terrain rather than inheriting a fixture's, and
  says why it was silent.

- **A board's console could be on the wrong port.** The EoRa-S3 prints to the
  USB Serial/JTAG rather than UART0, so everything the application said went to
  a peripheral nobody was holding: a node that started and appeared never to
  have spoken.

- **A machine too old for the build says so.** Both the firmware and a
  downloaded emulator now check what they are before running it, so a
  wrong-architecture or too-old-glibc build is refused by name rather than
  producing `exec format error` from a process nobody is watching.

- **Opening a packet in its own window crashed the workbench.** A list grew a
  sixth entry and the five chips beside it did not, so drawing the legend
  indexed past the end. The arrays are declared from the lists' own lengths now,
  which is a compile-time check rather than a pair somebody has to remember.

- **Windows: the application started and disappeared.** `meshbench.exe` with no
  arguments printed usage and exited, which is what the Start menu shortcut and
  a double-clicked binary both do - and a release is linked `-H windowsgui`, so
  the text explaining it went nowhere. A bare invocation opens the workbench. A
  failure now writes `meshbench-error.log`, including a panic's stack trace,
  which previously reached nobody at all.

- **Playing over a held warm said only "playing".** A warm stopped to ask about
  terrain leaves no links measured, so every transmission reaches nobody while
  the counts still look right. Play says what will happen.

- **Fetching buildings pointed at a tool no release ships**, and its size cap
  had never fired: the dataset index writes sizes for a person to read, they
  were parsed as integers, and every file was priced at zero.

### Removed

- **`radioserver`.** The process that owned the chip model is gone, along with
  `MESHBENCH_RADIO_SERVER`. `MESHBENCH_RADIO_LIB` points at the chip library
  instead, and a release bundle carries it.

## [0.0.5] - 2026-09-02

### Added

- **A written compatibility story, and the parts of it the build enforces.**
  `docs/compatibility.md` says what 0.x means here rather than leaving it to
  the convention that anything may break: the control protocol number and when
  it moves, the client and workbench pairing rule, the fixture format, how
  every platform stamps its version, where the GPL source archive stands, and
  seven concrete things that would have to be true before 1.0 was worth
  cutting. `MeshBench/gio` is the fork the `replace` directive points at, so a
  private one would make the source archive worthless; it is public, checked
  rather than assumed, and `docs/repositories.md` now says so and how it was
  checked.
- **A fixture carries the format it was written by, and a build refuses one
  from the future.** A file whose `format` is higher than this build reads is
  refused by name, with both numbers and the release to install, rather than
  read for the parts it recognises. Reading three quarters of a fixture does
  not fail: it answers a question about a network nobody described. Older
  files, including every one written before the field existed, still open.
- **The version stamp is pinned across all three build paths.** Linux, macOS
  and Windows are built by different jobs, and once they disagree the
  difference is invisible until a release is out. A test now reads the three
  build commands out of the pipeline and fails if any of them builds the binary
  without stamping the tag, drops its `v`, or if a fourth path appears without
  one. All three also refuse a version that is not a plain `X.Y.Z`, because a
  workbench stamped `vv0.1.0` is not a release as far as the pairing rule can
  tell and would pair with anything.

- **The emulator toolchain is fetchable.** `radioserver`, QEMU and Renode are
  rows on the Resources page like anything else the application downloads, with
  a size, the terms to read first, and a fetch that verifies the digest and
  unpacks into `~/.cache/meshbench/tools/`, already the third place the
  emulator lookup searches, so a fetched tool needs no configuration. A release
  tarball carries all three beside the binary, but the AppImage and the `.deb`
  carry only `radioserver` and a source checkout had no path to any of them.
  Where a platform has no build, or where this fetcher cannot install the one
  there is, the row says which rather than offering a download that could not
  work.
- **Resource manager.** Everything the application downloads that is not
  firmware, in one page: the Nordic SoftDevice plus the caches that fill
  themselves as the map is used - terrain, basemap, map tiles, building
  footprints - with measured sizes, the terms each arrived under, and a way to
  delete them. On the machine it was written for those caches had reached
  7.4 GB with nothing in the application to say so.
- **Board compatibility matrix.** Eight capabilities measured per board -
  build, boot, radio, tx, rx, flood, fem, power - against real published
  firmware under emulation, with three states and never a silent blank. Ten
  boards are described; the results are in the README.
- **Tabbed docking shell.** Panels dock as tabs and every one of them is
  reachable from a menu, replacing a modal chooser that could only pop panels
  out into windows that would not stay on top.
- **Coverage rasters on the GPU**, folded on the device, with CPU twins tested
  against each kernel - and drawn on the map rather than only written to a PNG.
- **Link profiles for the pair you actually picked**, including endpoints that
  are points on the ground rather than nodes.
- **A companion bench** where a companion is chosen, and a Radio tab whose two
  numbers stop claiming more than they know.
- **Version stamping**: one answer, set by the release pipeline and shown in
  the window.
- **Community furniture**: `SECURITY.md`, `CODE_OF_CONDUCT.md`,
  `THIRD_PARTY_NOTICES.md`, `CITATION.cff`, issue and pull-request templates.

### Changed

- **A run carrying an emulated node now says it cannot be repeated, instead of
  being quoted as though it could.** Determinism is documented as a property of
  the whole simulator, and it is not one on a scenario with an emulator in it:
  that node's firmware is a published image with nothing in it that can receive
  the engine's tick, so the acknowledgement comes from the chip model on our
  side of the socket while the guest runs against QEMU's or Renode's clock.
  Measured on one repeater at one seed, three runs put its first transmission at
  49.83 s, 45.72 s and 55.86 s. `sim.state` and `experiment.start` now answer
  `reproducible` and `not_reproducible_why`, the Go and Python clients carry
  both on their `SimState`, the sweep says it beside the cost estimate before
  the run as well as over the results, restoring a checkpoint says the replay
  will not land where it did, and `CLAUDE.md`, `CONTRIBUTING.md`,
  `docs/architecture.md` and `docs/shortcomings.md` state the exception rather
  than the rule alone. Use native for anything being compared.
- **What buildings buy off the excess-loss term is now measured rather than
  assumed: 0.70 dB.** Fitted against 451 live ScotMesh nodes with 4.5 million
  Microsoft ML footprints over Scotland loaded, the term comes out at 29.07 dB
  against the same night's bare-earth fit of 29.77 dB, while the footprints
  remove 37.6% of the link matrix. Both are true because the fit only sees
  paths that were heard, and a path buildings price into the ground stops being
  heard at all. `docs/studies/excess-loss-with-buildings.md` is the record and
  `docs/shortcomings.md` carries the consequence, including that a crossed
  building is charged once per building with no combination rule, so an
  environment-loaded urban path says "blocked" rather than a number of
  decibels. `DefaultExcessLossDB` is unchanged at 25.1 dB, and the study says
  what would move it.
- **QEMU on all three platforms, from one release.** The pin moves to
  `v9.2.2-meshbench-sx1262-10`, which cross-compiles Linux, macOS and Windows
  from one commit and refuses to publish if the SX1262 device, its DIO1 line or
  the GPIO interrupt behind it has gone missing. The macOS bundle and the
  Windows zip had been shipping without an ESP32 emulator, and the two
  explanations for that - that the fork built Linux alone, and that emulation
  had never run on Windows - are both retired. A node on Windows reaches the
  radio model over TCP, which is the path Renode has always used; what it still
  cannot do is fetch an emulator, because the download side reads ELF and
  Mach-O headers rather than PE.
- **`resource.list` returns the rows, not a count of them.** It answered
  `{"rows": 5}` and left the rows in the snapshot, where only a panel could
  reach them, so from outside the window there was no way to ask what this
  machine holds or what it could fetch. The rows now come back under
  `resources`, the fifth verb to be fixed the way `nodes.stats`,
  `firmware.library`, the study area and `console.read` were.
- **The control protocol is enforced at connect.** A client declares the wire
  version it speaks on the frame it was already sending - the token line on
  loopback TCP, the first request on a unix socket - and a version this build
  does not speak is refused before any verb runs, with both numbers and which
  end to upgrade. The rule is an exact match in both directions, because the
  number moves only when something an older client relied on has changed. A
  client that declares nothing is still served: it finds out from
  `session.hello`, as the shipped clients always have.
- **The tree is nine layers** (`diag`, `rf`, `mesh`, `firmware`, `world`,
  `sim`, `study`, `app`, `ui`), with `internal/layers_test.go` failing the
  build on an import that points upward, and `internal/layoutmap_test.go`
  failing when the layout map in `CLAUDE.md` stops matching the tree. Several
  packages were split out along the way: `rf/geo` (one great-circle
  implementation, not thirteen), `rf/propagation` out from under the raster,
  and the frame decoder out from under the capture recorder.
- **Verify became Validate**, and says where you are in it.
- Six files came back under the length limit; panel filenames say which panel
  they hold; the panel list is one file per family.


- **A cached link matrix could not say whether buildings were priced into it.**
  The measured matrix persists to disk under a fingerprint of the geometry it
  is about, and the environment was not in that fingerprint, so one key covered
  two different physics: a session opened over bare earth restored a
  building-priced matrix, found it already answered every pair, skipped the
  warm and reported itself measured, with a third of the country's links
  missing and nothing saying why. Unlike the excess-loss term, a priced rooftop
  is baked into the cached number and cannot be taken back out where the cache
  is read, so the environment now keys the matrix, switching it re-keys the
  session, and `matrixVersion` moves to 3 so no file written before this can be
  mistaken for either. Found while measuring what buildings buy, twice, as two
  arms that agreed to sixteen significant figures.
- **`licgen` wrote to a directory the seven-layer move had renamed**, so it
  exited 1 on every invocation and the release pipeline had been broken since
  19 August. It is now checked on every pull request rather than only at a tag.
- **A stopped store made its callers wait for ever** instead of refusing.
- **`resource.fetched` deadlocked the store** by posting a command to the
  goroutine that was running it - which would have hung the workbench the first
  time any download completed.
- **Disabled buttons still accepted clicks.** `comp.Button` applied
  `gtx.Disabled()` inside the `Clickable`, so every disabled control in the
  application pressed perfectly while drawn faded.
- **The job cancel that had existed for months and could never be called.**
- **A release stopped keeping two copies of itself.**
- Four sections of `docs/shortcomings.md` that had stopped being true, and the
  `CLAUDE.md` layout map that a new package had shipped without.

## [0.0.4] - 2026-08-16

Sweeps, and measuring rather than assuming.

- A sweep happens in front of you, and sends under a scope again.
- An arm has to demonstrate its manipulation before a cell measures anything.
- A fixture with a front-end module, because no shipped one had ever had one.
- The first published study, and the data behind it.
- Corrections: how close the links actually are, what a decibel was worth on
  the deliveries a run already made, and a fault that needs nobody to switch
  it on.

## [0.0.3] - 2026-08-14

- Removed the catalogue probe.

## [0.0.2] - 2026-08-14

The three bugs the 0.0.1 install turned up.

- The firmware library was losing half its contents; the published board images
  are back in it, it rebuilds when the catalogue answers, and it filters to the
  boards that can actually be emulated.
- Fixtures that install, and a GPU that has to prove itself before being used.
- A Windows check in CI, because DX12 had never been exercised.
- macOS bundles carry their emulators inside them.

## [0.0.1] - 2026-08-14

First release: one binary per platform.

- AppImage, `.deb` and tarball for Linux; `.dmg` for Apple Silicon; `.zip` for
  Windows. Neither the macOS nor the Windows build is code-signed.
- The radio model reachable over TCP where there is no unix socket.
- Two data races found by `-race` and fixed.

[Unreleased]: https://github.com/MeshBench/meshbench/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/MeshBench/meshbench/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/MeshBench/meshbench/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/MeshBench/meshbench/compare/v0.0.11...v0.1.0
[0.0.11]: https://github.com/MeshBench/meshbench/compare/v0.0.10...v0.0.11
[0.0.10]: https://github.com/MeshBench/meshbench/compare/v0.0.9...v0.0.10
[0.0.9]: https://github.com/MeshBench/meshbench/compare/v0.0.8...v0.0.9
[0.0.8]: https://github.com/MeshBench/meshbench/compare/v0.0.7...v0.0.8
[0.0.7]: https://github.com/MeshBench/meshbench/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/MeshBench/meshbench/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/MeshBench/meshbench/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/MeshBench/meshbench/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/MeshBench/meshbench/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/MeshBench/meshbench/compare/v0.0.1...v0.0.2
[0.0.1]: https://github.com/MeshBench/meshbench/releases/tag/v0.0.1
