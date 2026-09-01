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

**Emulated boards do not work on Windows.** The radio model reaches QEMU over a
Unix socket and the TCP path Renode uses has never been wired for it, so a
board cannot come up whatever is in the zip. Native nodes, the channel, the
studies and everything else do work. Help > Setup says the same thing on the
machine itself rather than leaving it to be found here.

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
