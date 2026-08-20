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

The zip carries the QEMU and Renode emulators and the radio model, so emulated
ESP32 and nRF52 boards need nothing else installed. That path is newer on
Windows than on Linux and macOS: if a board will not start, run
`meshbench.exe workbench` from a terminal and it prints what it could not find.

### What arrives later, over the network

Nothing needs a toolchain, but three things are fetched on first use and cached
under `~/.cache/meshcoresim` (`%LOCALAPPDATA%` on Windows):

| what | where from | when |
|---|---|---|
| MeshCore firmware builds | `MeshBench/meshcore-native` releases | first time a node runs firmware |
| board images | MeshCore's own releases | first time a board is emulated |
| map and terrain tiles | OpenStreetMap, CARTO, Esri, AWS terrarium | as the map is panned |

Everything the application ships under is listed in **Help → Licences &
attributions**, and in `LICENCES/` beside the binary.
