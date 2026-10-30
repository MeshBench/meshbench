package resource

// What is published, where, and under what terms.
//
// Data rather than code, and pinned by digest rather than trusted by URL,
// because these are the files an emulated node's whole existence rests on and
// a release asset can be replaced under its own tag.
//
// The platform map is the honest half. A key that is absent means no build
// exists for that machine, and the row says so instead of offering a download
// that could not work; Unsupported is the stronger statement, for a platform
// where the build exists and the path still does not.

// Where each tool's release lives, spelled once so a bump moves one line
// rather than four.
//
// Every one of these names a tag and never "latest". The rule a pin is held
// to, and who moves it, is written down once in packaging/emulator-pins.env,
// which the release pipeline fetches by and which the tests beside this file
// check these URLs against: what CI puts in a bundle and what a first run
// downloads have to be the same build, or a bug reproduces on one machine and
// not the other.
const (
	qemuBase   = "https://github.com/MeshBench/qemu/releases/download/v9.2.2-meshbench-sx1262-16/"
	renodeBase = "https://github.com/MeshBench/renode/releases/download/meshbench-20260904-e7196ef/"
	chipBase   = "https://github.com/MeshBench/virtual-sx1262/releases/download/v1.5.0/"
)

// qemuArm64LinuxIsUntried is the one platform the fork builds and this does not
// serve.
//
// Not a missing build: the release carries an aarch64 Linux tarball beside the
// three the bundles are cut for. What is missing is anybody having run an
// emulated board on arm64 Linux, and the pins the bundle and this catalogue are
// held to name three platforms. Pinning a fourth digest nothing has ever
// unpacked would be a claim rather than a check, so the row says where the
// build is instead.
const qemuArm64LinuxIsUntried = "the fork publishes an aarch64 Linux build and " +
	"nothing here has started an emulated board on arm64 Linux, so it is not " +
	"pinned. Take qemu-xtensa-softmmu-*-aarch64-linux-gnu.tar.xz from the " +
	"MeshBench/qemu release and put qemu-system-xtensa in this directory, or " +
	"set MESHBENCH_QEMU"

// toolReleases is every tool the emulator lookup asks for, in the order they
// are needed: nothing boots without the chip, and which emulator follows
// depends on the board.
//
// The chip's version tracks its ABI rather than the library, because a host
// declares the version it needs and refuses one that cannot serve it. 1.3 is
// the one with the byte-at-a-time SPI path, which is the only path an emulator
// can use: its controller clocks one byte and wants the answering byte back
// before it clocks the next.
var toolReleases = []toolRelease{{
	Name:    "virtual-sx1262",
	Version: "v1.5.0",
	MCU:     "",
	Why: "the SX1262 itself, which both emulators load and a native node links; " +
		"every emulated node needs it, ESP32 or nRF52",
	Terms: chipModelTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    chipBase + "virtual-sx1262-linux-amd64.tar.gz",
			SHA256: "bfa992792c080d992a49396e221a6038ee085d23a386289c293b87c7943f34ad",
			Bytes:  44958, Kind: tarGzip, Magic: elfAMD64,
			Root:   "virtual-sx1262-linux-amd64",
			Binary: "virtual-sx1262-linux-amd64/lib/libvirtualsx1262.so",
		},
		"darwin/arm64": {
			URL:    chipBase + "virtual-sx1262-macos-arm64.tar.gz",
			SHA256: "9503d1cb1dd28069ccdecbb2a1b6ac59d7e0d86f89ae2dd646d348dfe8d8afc1",
			Bytes:  29339, Kind: tarGzip, Magic: machARM64,
			Root:   "virtual-sx1262-macos-arm64",
			Binary: "virtual-sx1262-macos-arm64/lib/libvirtualsx1262.dylib",
		},
		"windows/amd64": {
			URL:    chipBase + "virtual-sx1262-windows-amd64.tar.gz",
			SHA256: "ea5296f8f2607f76a82502b1f85f21fb3501038b2ed930033aafe18135ed83b4",
			Bytes:  188504, Kind: tarGzip, Magic: peAMD64,
			Root:   "virtual-sx1262-windows-amd64",
			Binary: "virtual-sx1262-windows-amd64/lib/libvirtualsx1262.dll",
		},
	},
}, {
	Name:    "qemu-system-xtensa",
	Version: "v9.2.2-meshbench-sx1262-16",
	MCU:     "ESP32",
	Why: "the emulator for the ESP32 family, carrying our SX1262 device, its " +
		"DIO1 line and the GPIO implementation upstream has not got",
	Terms: qemuTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    qemuBase + "qemu-xtensa-softmmu-v9.2.2_meshbench_sx1262_16-x86_64-linux-gnu.tar.xz",
			SHA256: "40b9277e23961c25cc31938dcb7b8ca1d06f762be1e617d60322e9fef9bb7401",
			Bytes:  17096732, Kind: tarXZ, Magic: elfAMD64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		},
		"windows/amd64": {
			URL:    qemuBase + "qemu-xtensa-softmmu-v9.2.2_meshbench_sx1262_16-x86_64-w64-mingw32.tar.xz",
			SHA256: "c4ce86714e362de7755a9f796cc01de73240aebfb592d58cae3ea0c0d2aedc1a",
			Bytes:  17624756, Kind: tarXZ, Magic: peAMD64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa.exe",
		},
		"darwin/arm64": {
			URL:    qemuBase + "qemu-xtensa-softmmu-v9.2.2_meshbench_sx1262_16-aarch64-apple-darwin.tar.xz",
			SHA256: "8750392e0adfb4efa52d4db9af775b06173f18bbac81727e4384ed281a88de38",
			Bytes:  5172684, Kind: tarXZ, Magic: machARM64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		},
	},
	Unsupported: map[string]string{
		"linux/arm64": qemuArm64LinuxIsUntried,
	},
}, {
	Name:    "renode",
	Version: "meshbench-20260904-e7196ef",
	MCU:     "nRF52",
	Why: "the emulator for the nRF52 boards, carrying the SEVONPEND fix without " +
		"which the firmware sleeps for ever with its wake condition already true",
	Terms: renodeTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    renodeBase + "renode-1.16.1.linux-portable-meshbench.tar.gz",
			SHA256: "acf9e4703ec44e1561961779152806bfe0884b401f06bb3cc8e81b57819bacea",
			Bytes:  61635919, Kind: tarGzip, Magic: elfAMD64,
			Root: "renode_1.16.1-portable", Binary: "renode_1.16.1-portable/renode",
		},
		// The one app bundle in the catalogue, and the reason its Root and
		// Binary look unlike the others: macOS Renode is published as a .app,
		// so the launcher is inside it rather than at the archive's top level.
		//
		// What this row now claims is that the download is a real Renode and a
		// node will find it - the launcher is a Mach-O arm64 executable where
		// the old asset had none. It does not claim an nRF52 board has been
		// booted on macOS: nothing has run one there, and no workflow does. So
		// this is offered rather than asserted, and the bring-up is still owed.
		"darwin/arm64": {
			URL:    renodeBase + "renode-meshbench-macos-arm64-portable.tar.gz",
			SHA256: "219669baba21ae833b53e0521bcb3614f0243d21ce1d09534bc8be0524f349da",
			Bytes:  56118484, Kind: tarGzip, Magic: machARM64,
			Root: "Renode.app", Binary: "Renode.app/Contents/MacOS/renode",
		},
		// The one zip in the catalogue. Renode publishes its Windows build that
		// way, which is the second of the three things that used to make a
		// Windows fetch impossible.
		"windows/amd64": {
			URL:    renodeBase + "meshbench-renode-1.16.1.windows-portable.zip",
			SHA256: "a132c360c23ad20c7e5d367bd819c7a26c0001457aadc6bd8a17c3ff2e8a3834",
			Bytes:  104045080, Kind: zipArchive, Magic: peAMD64,
			Root: "renode_1.16.1-portable", Binary: "renode_1.16.1-portable/renode.exe",
		},
	},
	Unsupported: map[string]string{
		"darwin/amd64": "no macOS Intel package is published",
	},
}}

// The terms each tool arrives under, readable before the download rather than
// after it. The full texts are in the application's own Licences window and in
// every release bundle's LICENCES directory; what belongs here is the part
// somebody has to have read before pressing Fetch.
const (
	chipModelTerms = "The SX1262 model is MeshBench/virtual-sx1262, which is MIT " +
		"and public. It is a separate repository because four things link the same " +
		"chip - MeshCore built for this host, our QEMU fork, Renode's peripheral, " +
		"and eventually MeshBench - and it stays permissive because QEMU is GPLv2 " +
		"and MeshBench is GPL-3.0-or-later, which no one copyleft licence can serve " +
		"both of. It is loaded by the emulator at runtime and is not linked into " +
		"MeshBench."
	qemuTerms = "QEMU is GPL-2.0, with parts under compatible licences. This build " +
		"is our own fork - the meshbench-main branch of MeshBench/qemu, which is " +
		"public, and every release carries the source archive the licence requires " +
		"beside the binaries. It is run as a separate process and is never linked " +
		"into MeshBench, so the two sit beside each other as aggregation rather " +
		"than as one work."
	renodeTerms = "Renode is MIT, from Antmicro, and this is the meshbench branch " +
		"of MeshBench/renode, which is public. The portable package carries a .NET " +
		"runtime (MIT, Microsoft) and renode-infrastructure (MIT, with LGPL parts) " +
		"inside it. Run as a separate process, never linked into MeshBench."
)
