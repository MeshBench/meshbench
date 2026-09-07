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

[Unreleased]: https://github.com/MeshBench/meshbench/compare/v0.0.8...HEAD
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
