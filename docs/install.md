> Moved out of `README.md` so the front page can be read in a minute.
> This is the detail behind it, and it is maintained.

# Installing MeshBench


Downloads are on the [releases page](https://github.com/MeshBench/meshbench/releases).
Every one of them carries the application, the map fixtures, the licences and
an emoji-capable font; nothing else has to be installed first.

### Linux

**AppImage** — one file, any distribution, no install:

```bash
chmod +x meshbench-*-x86_64.AppImage
./meshbench-*-x86_64.AppImage
```

**Debian and Ubuntu** — puts it in the launcher with an icon:

```bash
sudo apt install ./meshbench_*_amd64.deb
meshbench workbench          # or find MeshBench in the applications menu
```

**Tarball** — the same application plus the QEMU and Renode emulators and the
radio model they clock, for emulating real board firmware offline. The
AppImage and the `.deb` carry the radio model but not the emulators, which are
110 MB of the tarball's size:

```bash
tar xzf meshbench-linux-x86_64.tar.gz
cd meshbench && ./meshbench workbench
```

Needs glibc 2.34 or newer (Ubuntu 22.04, Debian 12, RHEL 9, Fedora 35 and
anything since) and a GPU with Vulkan or GL. The `.deb` declares its
dependencies, so apt refuses on a machine that cannot run it rather than
installing something that dies at launch.

### macOS (Apple Silicon)

Open `MeshBench-*-arm64.dmg` and drag MeshBench to Applications.

> **The application is not signed with an Apple Developer ID yet**, so macOS
> will refuse to open it on the first attempt — "MeshBench is damaged" or
> "cannot be opened because the developer cannot be verified". It is neither
> damaged nor unverified in any sense that matters; it is unsigned, and that
> costs an Apple developer account we have not bought yet.
>
> To open it anyway, pick one:
>
> 1. **Right-click the app in Applications and choose Open**, then Open again
>    in the dialog. macOS remembers the decision.
> 2. If that dialog does not offer Open, go to **System Settings → Privacy &
>    Security**, scroll to the message about MeshBench and click **Open
>    Anyway**.
> 3. From a terminal, clear the quarantine flag the browser attached:
>    ```bash
>    xattr -dr com.apple.quarantine /Applications/MeshBench.app
>    ```
>
> This will stop being necessary once the build is signed and notarised.

Intel Macs are not built yet. Ask if you need one.

### Windows

Unzip `meshbench-*-windows-x86_64.zip` anywhere and run `meshbench.exe`.

Windows SmartScreen will warn about an unrecognised publisher for the same
reason macOS does — the binary is unsigned. Click **More info → Run anyway**.

**Emulated boards on Windows come out of the zip, not out of a fetch.** The zip
carries the emulators and the radio model, and a node there reaches the radio
model over TCP rather than over a Unix socket, so a board can come up. What
Help > Setup cannot do on Windows is download a missing one: it reads ELF and
Mach-O headers rather than PE, opens tars rather than zips, and installs by
symlink. If you replace one, put it beside `meshbench.exe` or point
`MESHBENCH_QEMU`, `MESHBENCH_RENODE` or `MESHBENCH_RADIO_SERVER` at it.

Emulated boards on Windows are also newer than the rest of the bundle and have
had less time on real hardware than the Linux and macOS ones. Native nodes, the
channel and the studies are unaffected.

### First run: what is missing, and what it costs

**Help > Setup** is one page listing every dependency, what state it is in,
what it would cost to fetch, and what to do about the ones the application
cannot fetch itself. It opens on its own the first time something is missing,
and stays out of the way afterwards. Over the control socket the same answer is
`setup.check`, which reads the disk and never the network.

The four things it reports:

| what | how it arrives |
|---|---|
| this build | deduced from what is beside the binary, because the tarball, the AppImage and a source checkout carry different things |
| firmware | downloaded on demand from GitHub releases; `firmware.download` takes a per-role tag such as `repeater-v1.17.0`, and a bare `v1.17.0` resolves nothing |
| terrain heights | only once this machine has said it may, with the size quoted first |
| the emulator toolchain | fetched from the Resources page or by `resource.fetch`, into `~/.cache/meshbench/tools` |

Map and basemap tiles fill themselves as the map is panned and are small.

Tools are looked for where `MESHBENCH_RADIO_SERVER`, `MESHBENCH_QEMU` or
`MESHBENCH_RENODE` point, then beside the binary, then in
`~/.cache/meshbench/tools`, then on `PATH`. **`PATH` is the one that will not
help**: a desktop application is not launched from a shell and inherits no
shell environment, so a QEMU or a Renode installed by a package manager is both
invisible here and the wrong build. Ours carry an SX1262 device and the
SEVONPEND fix respectively; a stock build starts, reports no chip or hangs, and
looks like a MeshBench fault.

From a source checkout, `radioserver` can also be built rather than fetched:
`./build.sh radioserver out` in a `MeshBench/meshcore-native` clone, then copy
the binary into the tools directory.

Everything the application ships under is listed in **Help → Licences &
attributions**, and in `LICENCES/` beside the binary.

## Updating

There is no automatic update, and that is deliberate: a run holds unsaved
state, there is no autosave, and replacing a binary underneath one is a way to
lose somebody's work. What the application does is find out, tell you once, and
put a checked copy of the new release on the disk beside the old one.

**Checking is off until you say otherwise.** Nothing asks the release page
until Setup's *version* row, or Configuration > System, is answered. Allowed,
it asks once a day, in the background, and never as a condition of the window
opening; refused, it never asks again and nothing mentions it. A working copy
is never told it is out of date: it is unreleased, not behind. Over the socket
the verbs are `update.allow`, `update.check`, `update.status`,
`update.download`, `update.reveal` and `update.notes`; a headless session never
checks on its own.

**The routine check costs no API budget.** "Is there anything newer" is
answered by the redirect `github.com/MeshBench/meshbench/releases/latest`
already serves: a 302 naming the newest tag, no JSON and no API call. That
matters because GitHub's API allows an unauthenticated caller 60 requests an
hour per address, and an address is a household, an office or an ISP doing
carrier-grade NAT: an updater checking on every launch would spend everybody's
budget on that address. The API is asked once a release is found, because it is
the only route that knows the assets and their sizes.

**"I could not find out" is its own answer.** A rate limit, a captive portal
and a build that is current are three different things, and only one of them is
about your build. A refused or unreachable check says so and names the reason,
including how long a rate limit has left to run; it never reports itself as up
to date.

**What is downloaded is checked.** Every release publishes `SHA256SUMS` beside
its artefacts, and a download whose digest does not match it is deleted rather
than kept. That digest comes from the same release as the file, so what it
proves is that the download arrived intact, not that the release is genuine;
what says the release is ours is the TLS connection to `github.com`, which is
why an asset served from anywhere else is refused before a byte is read. The
size is stated before it is spent.

**Nothing is replaced for you.** The download lands in
`~/.cache/meshbench/updates/<tag>/`, and the row then says what to do with it.
What that is differs per platform, and only two of these are limits of this
application rather than of the operating system:

| bundle | what you get | why |
|---|---|---|
| Linux `.tar.gz` | download and instructions | unpack beside and move over the old one; the tarball carries the emulators and the fixtures, so taking only the binary leaves a build made of two releases |
| Linux AppImage | download and instructions | a rename over a running binary is allowed on Linux, so `mv` works while MeshBench is open; the new one starts next launch |
| Linux `.deb` | refused, on purpose | apt owns those files. `sudo apt install --only-upgrade meshbench` |
| macOS `.dmg` | download and instructions | the swap works, but the build is unsigned and anything downloaded is quarantined, so the new copy needs right-click then Open on its first launch, the same as the first one did |
| Windows `.zip` | download and instructions | a running `.exe` cannot be replaced while it is running. Close MeshBench first, then unzip and move the folder over |

**An update invalidates a pinned client.** A client and the workbench it drives
have to be the same release, and the control socket refuses a pair that is not,
so upgrade them together: `pip install -U meshbench==<new>` and
`npm install @meshbench/client@<new>`.
