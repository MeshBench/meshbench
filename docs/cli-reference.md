# CLI reference

`workbench` opens the desktop application. **Every other command is headless**,
and that split is deliberate and permanent: the headless path is what scripted
runs and regression suites are built on, not a stopgap for the window.

Nothing but `workbench` needs a GPU, a display, or anything running anywhere
else.

```console
meshbench <command> [flags]
```

There are **16**<!--flagdoc:commands--> commands and
**164**<!--flagdoc:flags--> flags between them, of which
**34**<!--flagdoc:capture--> exist only so a panel, a menu or a view can be
reached without a click.

Everything below the first heading is generated from the flag declarations
themselves, by `tools/flagdoc/flagdoc.py`, which builds the binary and asks it.
A flag that exists is on this page and a flag that has gone is not, because CI
fails when the two disagree. The examples are run by hand and held beside the
commands in `cmd/meshbench/commands.go`; what is checked mechanically is that
every flag they name is still real.

Every command also takes `-h`, which prints the same flags with the same
defaults.

Two conventions in the tables below. A default shown as `<cache>/...` is the
per-user cache directory, which is `~/.cache` on Linux and different elsewhere;
the binary prints the real path. And **required** in the default column means
the command refuses to run without it.

Results are a best case. The model has no multipath, bare-earth terrain and an
idealised demodulator. If it says a link will not work, believe it; if it says a
link works marginally, go and measure.

## What a flag is for

Every flag below carries one of these, because a flag that arranges a screenshot and a flag that changes an answer are not the same kind of thing and a reference that lists them together is misleading.

<!-- BEGIN GENERATED CLI -->

| for | meaning |
|---|---|
| capture | capture and scripting: it reaches a view, a panel or a menu without a click, so a screenshot or a script does not need a hand on the mouse. None of these changes a result. |
| data | where the inputs come from, and whether the network may be used to get them. Nothing here is part of the physics. |
| diagnostic | measures the application itself rather than the network it is simulating. |
| output | where the answer is written and how much of it is said. It does not change what was computed. |
| result | an input the answer depends on. Change it and the number changes. |

## The commands

| command | what it does | flags |
|---|---|---|
| `link` | link budget between two points, both directions | 15 |
| `profile` | terrain profile and the worst obstruction on a path | 10 |
| `coverage` | coverage raster from one station, written as a PNG | 15 |
| `spectrum` | what an SDR observer captures: waterfall PNG and audio | 8 |
| `terrain` | download elevation tiles for an area | 8 |
| `boards` | the hardware profiles this build knows about | 0 |
| `firmware` | list, download or import MeshCore firmware | 7 |
| `energy` | will a solar node survive the winter | 7 |
| `airtime` | LoRa time on air, as the firmware computes it | 4 |
| `traffic` | flood a message through a network and report what happened | 15 |
| `basemap` | download map tiles for an area | 8 |
| `dev` | build a MeshCore checkout and give it to the workbench | 5 |
| `serve` | run a mesh and expose a companion to your app | 8 |
| `test` | run a fixture on real firmware and check its assertions | 9 |
| `headless` | run the verbs over the control socket, with no window | 7 |
| `workbench` | open the desktop workbench: build a scenario on a map and run it | 38 |

## `meshbench link`

Link budget between two points, in both directions.

```console
meshbench link -from-lat 56.3980 -from-lon -3.4260 -from-height 20 -to-lat 56.3327 -to-lon -3.3239 -to-height 10 -offline
```

A mast on the hill against a repeater in the glen, 9.6 km apart. Both directions are reported because reachability is asymmetric, and this pair happens to be balanced at +5.4 dB.

| flag | default | for | meaning |
|---|---|---|---|
| `-freq` | `869.525` | result | frequency, MHz |
| `-from-gain` | `2.15` | result | antenna gain, dBi |
| `-from-height` | `10` | result | antenna height above ground, metres |
| `-from-lat` | **required** | result | latitude of the first station |
| `-from-lon` | **required** | result | longitude of the first station |
| `-from-tx` | `22` | result | transmit power, dBm |
| `-offline` | `false` | data | never download; answer from the cache and fail loudly otherwise |
| `-sensitivity` | `-137` | result | receiver sensitivity, dBm |
| `-terrain-cache` | `<cache>/meshbench/terrain` | data | where downloaded elevation tiles live |
| `-to-gain` | `-2` | result | antenna gain, dBi |
| `-to-height` | `1.5` | result | antenna height above ground, metres |
| `-to-lat` | **required** | result | latitude of the second station |
| `-to-lon` | **required** | result | longitude of the second station |
| `-to-tx` | `22` | result | transmit power, dBm |
| `-zoom` | `12` | result | tile zoom; 12 is about 30 m per pixel and matches the data |

## `meshbench profile`

Terrain profile and the worst obstruction on a path.

```console
meshbench profile -from-lat 56.3980 -from-lon -3.4260 -from-height 20 -to-lat 56.0700 -to-lon -3.4530 -samples 400 -offline
```

What stands in the way when a link budget comes back short. This one names the hill, 15.7 km along and 272 m into the path.

| flag | default | for | meaning |
|---|---|---|---|
| `-from-height` | `10` | result | antenna height above ground, metres |
| `-from-lat` | **required** | result | latitude of the first point |
| `-from-lon` | **required** | result | longitude of the first point |
| `-offline` | `false` | data | never download; answer from the cache and fail loudly otherwise |
| `-samples` | `200` | result | profile samples |
| `-terrain-cache` | `<cache>/meshbench/terrain` | data | where downloaded elevation tiles live |
| `-to-height` | `1.5` | result | antenna height above ground, metres |
| `-to-lat` | **required** | result | latitude of the second point |
| `-to-lon` | **required** | result | longitude of the second point |
| `-zoom` | `12` | result | tile zoom; 12 is about 30 m per pixel and matches the data |

## `meshbench coverage`

Coverage raster from one station.

```console
meshbench coverage -lat 56.3980 -lon -3.4260 -height 20 -radius 15 -pixels 200 -o perth.png -offline
```

A 30 km square around the mast, at 200 by 200 cells. One-way cells get their own colour: they are neither covered nor not.

| flag | default | for | meaning |
|---|---|---|---|
| `-freq` | `869.525` | result | frequency, MHz |
| `-gain` | `2.15` | result | antenna gain, dBi |
| `-height` | `10` | result | antenna height above ground, metres |
| `-lat` | **required** | result | station latitude |
| `-lon` | **required** | result | station longitude |
| `-o` | `coverage.png` | output | output PNG |
| `-offline` | `false` | data | never download; answer from the cache and fail loudly otherwise |
| `-pixels` | `400` | result | raster width in pixels |
| `-radius` | `20` | result | half-width of the area, km |
| `-remote-height` | `1.5` | result | height of the imagined far station, metres |
| `-remote-tx` | `22` | result | far station transmit power, dBm |
| `-sensitivity` | `-137` | result | receiver sensitivity, dBm |
| `-terrain-cache` | `<cache>/meshbench/terrain` | data | where downloaded elevation tiles live |
| `-tx` | `22` | result | transmit power, dBm |
| `-zoom` | `12` | result | tile zoom; 12 is about 30 m per pixel and matches the data |

## `meshbench spectrum`

What an SDR observer captures.

```console
meshbench spectrum -sf 10 -bandwidth 250 -rx -120 -o waterfall.png -wav chirp.wav
```

An SF10 chirp 6 dB under the noise floor, as a picture and as a sound. A chirp through a narrow filter is a rising whistle.

| flag | default | for | meaning |
|---|---|---|---|
| `-bandwidth` | `250` | result | bandwidth, kHz |
| `-freq` | `869.525` | result | centre frequency, MHz |
| `-noise-figure` | `6` | result | observer noise figure, dB |
| `-o` | `waterfall.png` | output | waterfall PNG |
| `-rx` | `-100` | result | received signal level, dBm |
| `-sf` | `10` | result | spreading factor of the transmission |
| `-symbols` | `8` | result | symbols to capture |
| `-wav` | none | output | also write audio here, as an SDR would sound |

## `meshbench terrain`

Download elevation tiles for an area.

```console
meshbench terrain -south 56.0 -north 56.5 -west -3.6 -east -2.8 -estimate
```

What the download would cost, before spending it. Drop -estimate to fetch. Tiles cache permanently, so -offline answers from them afterwards.

| flag | default | for | meaning |
|---|---|---|---|
| `-east` | **required** | data | eastern edge |
| `-estimate` | `false` | data | report the download and stop |
| `-north` | **required** | data | northern edge |
| `-offline` | `false` | data | never download; answer from the cache and fail loudly otherwise |
| `-south` | **required** | data | southern edge |
| `-terrain-cache` | `<cache>/meshbench/terrain` | data | where downloaded elevation tiles live |
| `-west` | **required** | data | western edge |
| `-zoom` | `12` | data | tile zoom; 12 is about 30 m per pixel and matches the data |

## `meshbench boards`

The hardware profiles this build knows about.

```console
meshbench boards
```

RADIATED is what leaves the antenna: chip power minus board loss plus the antenna it ships with. That is the number that decides range, and it is not the number on the box.

It takes no flags.

## `meshbench firmware`

List, download or import MeshCore firmware.

```console
meshbench firmware -offline
```

What is already on this machine. Without -offline it lists the published catalogue; -get fetches one, -import takes a build of your own.

| flag | default | for | meaning |
|---|---|---|---|
| `-board` | none | data | filter by board, or name the board when importing |
| `-cache` | `<cache>/meshbench/firmware` | data | where downloaded images live |
| `-get` | none | data | download an image by name, e.g. RAK_4631/repeater |
| `-import` | none | data | import your own .uf2, .bin or .elf |
| `-label` | none | data | what to call an imported build; defaults to a timestamp |
| `-offline` | `false` | data | list and use only what is already downloaded |
| `-role` | `repeater` | data | role, when importing |

## `meshbench energy`

Will a solar node survive the winter.

```console
meshbench energy -lat 56.34 -panel 10 -battery 6000 -tx 22
```

A 10 W panel and a 6 Ah cell at Scottish latitude, over a year. Receive current, not transmit power, is what usually decides this.

| flag | default | for | meaning |
|---|---|---|---|
| `-always-on` | `true` | result | a repeater listens continuously |
| `-battery` | `3400` | result | battery capacity, mAh |
| `-lat` | **required** | result | latitude, north positive |
| `-lon` | `0` | result | longitude, east positive |
| `-panel` | `0` | result | panel peak watts; 0 for no solar |
| `-tilt` | `50` | result | panel tilt from horizontal, degrees |
| `-tx` | `22` | result | transmit power, dBm |

## `meshbench airtime`

LoRa time on air, as the firmware computes it.

```console
meshbench airtime -sf 10 -bandwidth 250 -bytes 32
```

259 ms, and 139 transmissions an hour at a 1% duty cycle. The same arithmetic the firmware's own getEstAirtimeFor() does.

| flag | default | for | meaning |
|---|---|---|---|
| `-bandwidth` | `250` | result | bandwidth, kHz |
| `-bytes` | `32` | result | payload length |
| `-coding-rate` | `1` | result | 1 to 4, for 4/5 to 4/8 |
| `-sf` | `10` | result | spreading factor |

## `meshbench traffic`

Flood a message through a network and report what happened.

```console
printf '[{"Name":"Perth Hill","HasPosition":true,"Lat":56.398,"Lon":-3.426,"HeightAGLm":20,"Kind":"repeater"},{"Name":"Abernethy Repeater","HasPosition":true,"Lat":56.33271,"Lon":-3.32386,"HeightAGLm":10,"Kind":"repeater"},{"Name":"Glenrothes","HasPosition":true,"Lat":56.198,"Lon":-3.178,"HeightAGLm":10,"Kind":"repeater"},{"Name":"Kirkcaldy","HasPosition":true,"Lat":56.113,"Lon":-3.16,"HeightAGLm":8,"Kind":"repeater"}]\n' > fife.json
meshbench traffic -nodes fife.json -from "Perth Hill" -for 20000 -offline
```

One message into four nodes, with a cause for every node it did not reach. Add -firmware to run real MeshCore on each instead of injecting traffic.

| flag | default | for | meaning |
|---|---|---|---|
| `-bandwidth` | `250` | result | bandwidth, kHz |
| `-board` | `RAK4631` | result | board profile for imported nodes |
| `-firmware` | `false` | result | run a real MeshCore build on every node, rather than injecting traffic |
| `-for` | `20000` | result | how long to simulate, ms |
| `-freq` | `869.525` | result | frequency, MHz |
| `-from` | none | result | node to send from; the first repeater by default |
| `-nodes` | none | data | scenario JSON, or a CoreScope/Beacon export |
| `-offline` | `false` | data | never download; answer from the cache and fail loudly otherwise |
| `-sf` | `10` | result | spreading factor |
| `-source` | none | data | load nodes from a provider: corescope or beacon |
| `-terrain-cache` | `<cache>/meshbench/terrain` | data | where downloaded elevation tiles live |
| `-token` | none | data | provider token, if it needs one |
| `-url` | none | data | provider base URL |
| `-v` | `false` | output | print every event rather than a summary |
| `-zoom` | `12` | result | tile zoom; 12 is about 30 m per pixel and matches the data |

## `meshbench basemap`

Download map tiles for an area.

```console
meshbench basemap
```

The layers, with the attribution each one requires. Naming one with -layer and an area downloads it.

```console
meshbench basemap -layer carto-light -south 56.0 -north 56.5 -west -3.6 -east -2.8 -zoom 11 -estimate
```

36 tiles, about 1 MB. Every layer here contacts a third party.

| flag | default | for | meaning |
|---|---|---|---|
| `-cache` | `<cache>/meshbench/basemap` | data | tile cache |
| `-east` | `0` | data | eastern edge |
| `-estimate` | `false` | data | report the download and stop |
| `-layer` | none | data | which layer; omit to list them |
| `-north` | `0` | data | northern edge |
| `-south` | `0` | data | southern edge |
| `-west` | `0` | data | western edge |
| `-zoom` | `11` | data | tile zoom |

## `meshbench dev`

Build a MeshCore checkout and give it to the workbench.

```console
meshbench dev -from ~/src/MeshCore -role simple_repeater -assign=false
```

Builds the checkout into the firmware cache and stops there. Nothing is written into the MeshCore tree. Add -watch for a rebuild on every save, and drop -assign=false to put it on every node of that role.

| flag | default | for | meaning |
|---|---|---|---|
| `-assign` | `true` | capture | assign the build to every node of that role |
| `-from` | `.` | data | a MeshCore checkout to build |
| `-name` | none | data | what to call the build; the git branch by default |
| `-role` | `simple_repeater` | data | which application: simple_repeater, companion_radio or simple_room_server |
| `-watch` | `false` | capture | rebuild and reassign whenever a source file changes |

## `meshbench serve`

Run a mesh and expose a companion to your app.

```console
meshbench serve -fixture fixtures/fixture-fife-strict.json
```

58 nodes on real firmware, one companion on a loopback port it prints. Point a client at that address; -serial gives a pty instead.

| flag | default | for | meaning |
|---|---|---|---|
| `-addr` | `127.0.0.1:0` | output | address to listen on; port 0 picks a free one |
| `-fixture` | none | result | network to run; the smallest shipped one by default |
| `-node` | none | output | which companion to expose; the first one by default |
| `-offline` | `false` | data | never download; answer from the cache and fail loudly otherwise |
| `-quiet` | `false` | output | print the endpoint and nothing else |
| `-serial` | `false` | output | expose a virtual serial device instead of TCP |
| `-terrain-cache` | `<cache>/meshbench/terrain` | data | where downloaded elevation tiles live |
| `-zoom` | `12` | result | tile zoom; 12 is about 30 m per pixel and matches the data |

## `meshbench test`

Run a fixture and check its assertions.

```console
meshbench test -fixture fixtures/fixture-fife-strict.json -for 60000 -quiet
```

The one a pipeline calls. Exit 0 if every assertion passed, 1 if any failed; -junit writes a report with one case per assertion.

| flag | default | for | meaning |
|---|---|---|---|
| `-endpoint` | none | output | serve a companion node to a real client: "tcp:<node>" or "serial:<node>" |
| `-fixture` | **required** | result | fixture JSON to run |
| `-for` | `120000` | result | how long to simulate, ms |
| `-junit` | none | output | write a JUnit XML report here |
| `-offline` | `false` | data | never download; answer from the cache and fail loudly otherwise |
| `-quiet` | `false` | output | only print the verdict |
| `-seed` | `0` | result | override the fixture's seed |
| `-terrain-cache` | `<cache>/meshbench/terrain` | data | where downloaded elevation tiles live |
| `-zoom` | `12` | result | tile zoom; 12 is about 30 m per pixel and matches the data |

## `meshbench headless`

Run the verbs with no window, for scripts and CI.

```console
meshbench headless -fixture fife-strict -play -for 15s -control-socket /tmp/meshbench.sock
```

The same session the window builds, with nothing attached to look at it. A client connects to that socket and drives it with the verbs.

| flag | default | for | meaning |
|---|---|---|---|
| `-control-socket` | none | capture | where to answer: a path for a unix socket, or "tcp" for loopback with a token (the default on Windows). Two runs on one machine need two addresses |
| `-fixture` | none | result | open this fixture or project at startup |
| `-for` | `0s` | result | exit after this long; the default is to run until interrupted |
| `-play` | `false` | capture | start the run immediately |
| `-quiet` | `false` | output | do not echo status lines to stderr |
| `-seed` | `0` | result | override the scenario's seed |
| `-unverified-wiring` | `false` | result | run boards whose wiring nobody has watched boot |

## `meshbench workbench`

Open the desktop workbench: build a scenario on a map and run it.

```console
meshbench workbench -list-fixtures
```

The networks built into this binary, without opening a window.

```console
meshbench workbench -fixture fife-strict -panel Nodes -filter Abernethy -look 56.34,-3.32,11 -quit-after 20s
```

One panel filling the window, filtered, over a fixed view, closing itself. That is the capture shape: every one of those flags exists so a screenshot does not need a hand on the mouse.

| flag | default | for | meaning |
|---|---|---|---|
| `-capture` | none | capture | capture the waterfall at this node once the run has traffic |
| `-config-section` | none | capture | open the Configuration page on this section |
| `-control-socket` | none | capture | where the control socket answers: a path for a unix socket, or "tcp" for loopback with a token (the default on Windows, which has no unix socket a Python client can reach). MESHBENCH_CONTROL_SOCKET does the same, and two workbenches need two |
| `-coverage` | none | capture | compute and show coverage from this node at startup |
| `-cpuprofile` | none | diagnostic | write a CPU profile here |
| `-drop-menu` | none | capture | open this menu's dropdown at startup, so it can be captured |
| `-energy` | `false` | capture | run the site study for the selected node at startup |
| `-filter` | none | capture | preset the node view's search box, so a filtered table can be captured |
| `-fixture` | `scotland-ireland-strict` | result | network to load: a name (see -list-fixtures) or a path to a .json |
| `-fps` | `false` | diagnostic | report frames per second to stderr and /tmp/wb2-fps.log |
| `-import` | none | capture | describe an import from this CoreScope URL at startup |
| `-inject` | none | capture | originate a packet at this node once running |
| `-inject-every` | `0s` | capture | keep originating at that node this often; for looking at the traffic layer |
| `-layers` | none | capture | switch these map layers on at startup, comma separated |
| `-licence-section` | none | capture | scope the Licences panel to one section: forks, bundled, golibs, runtime, data |
| `-list-fixtures` | `false` | output | list the built-in networks and exit |
| `-look` | none | capture | start the camera at lat,lon,zoom - a capture cannot drag the map |
| `-memprofile` | none | diagnostic | write a heap profile here on exit |
| `-menu` | none | capture | fire this menu action at startup, so what it opens can be captured |
| `-node-menu` | none | capture | open this node's context menu at startup |
| `-node-tab` | `0` | capture | which tab a node window opens on: 0 console, 1 companion, 2 SDR, 3 settings, 4 radio, 5 stats, 6 activity, 7 connect, 8 hardware, 9 output |
| `-node-window` | none | capture | open this node's own window at startup |
| `-open-firmware` | none | capture | open this node's firmware list at startup |
| `-packet-tab` | `0` | capture | which tab the packet window opens on: 0 dissection, 1 journey (the propagation graph), 2 reception ledger, 3 where it went |
| `-panel` | none | capture | draw only this panel, filling the window |
| `-plan` | none | capture | plan between the selected node and this one at startup |
| `-play` | `false` | capture | start the simulation immediately |
| `-pop-out` | none | capture | open this panel in its own window at startup |
| `-provisioning` | none | capture | show what this node is told at boot, at startup |
| `-quit-after` | `0s` | capture | exit after this long; 0 runs until closed |
| `-save-run` | none | capture | save a run record under this name, then keep running |
| `-seed` | `0` | result | override the scenario's seed |
| `-sweep` | `false` | capture | run the default sweep at startup |
| `-terrain` | `false` | capture | shade the relief at startup |
| `-theme` | `dark` | capture | dark or light |
| `-unverified-wiring` | `false` | result | run boards whose emulation wiring nobody has watched boot yet |
| `-version` | `false` | output | print the version and exit |
| `-view` | `plan` | capture | which view to open |

<!-- END GENERATED CLI -->
