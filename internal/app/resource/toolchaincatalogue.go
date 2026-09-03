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
	qemuBase   = "https://github.com/MeshBench/qemu/releases/download/v9.2.2-meshbench-sx1262-10/"
	renodeBase = "https://github.com/MeshBench/renode/releases/download/meshbench-20260901-ca9f7e3/"
	chipBase   = "https://github.com/MeshBench/virtual-sx1262/releases/download/v1.3.0/"
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

// windowsFetchesNoEmulators is one refusal said once, and it is about this
// fetcher rather than about emulation.
//
// Emulation itself is not what stops on Windows. An emulated node asks the
// radio model for ":0" there and reaches it over TCP for both emulators, which
// is the path Renode has always used; the Windows zip carries radioserver.exe,
// a qemu-system-xtensa.exe and Renode's portable package, and lookupTool knows
// their unpacked layouts and the .exe suffix. What has never been built is the
// download half: checkExecutable reads ELF and Mach-O headers and would refuse
// a PE binary as not an executable at all, extractTar opens tars and Renode
// publishes its Windows build as a zip, and install finishes by making a
// symlink, which Windows grants only to an elevated process or a machine in
// developer mode. Fetching here would spend the bandwidth and then delete what
// it fetched.
const windowsFetchesNoEmulators = "the Windows zip already carries this, and " +
	"this page cannot install a replacement: it checks a download by reading " +
	"ELF and Mach-O headers, and would refuse a PE binary as not an executable " +
	"at all. Take the emulators from the Windows release, or put one beside " +
	"meshbench.exe and point MESHBENCH_QEMU, MESHBENCH_RENODE or " +
	"MESHBENCH_RADIO_LIB at it"

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
	Version: "v1.3.0",
	MCU:     "",
	Why: "the SX1262 itself, which both emulators load and a native node links; " +
		"every emulated node needs it, ESP32 or nRF52",
	Terms: chipModelTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    chipBase + "virtual-sx1262-linux-amd64.tar.gz",
			SHA256: "33f4453c39bfa1b8f0f4706efb387e7f9588217eabc2713887b7fdf9974d9155",
			Bytes:  26340, Kind: tarGzip, Magic: elfAMD64,
			Root:   "virtual-sx1262-linux-amd64",
			Binary: "virtual-sx1262-linux-amd64/lib/libvirtualsx1262.so",
		},
		"darwin/arm64": {
			URL:    chipBase + "virtual-sx1262-macos-arm64.tar.gz",
			SHA256: "362ed99e8b3b434cc738045531818c8de8e6465f1a05cb057fb7199605f59b6a",
			Bytes:  19118, Kind: tarGzip, Magic: machARM64,
			Root:   "virtual-sx1262-macos-arm64",
			Binary: "virtual-sx1262-macos-arm64/lib/libvirtualsx1262.dylib",
		},
	},
	Unsupported: map[string]string{"windows/amd64": windowsFetchesNoEmulators},
}, {
	Name:    "qemu-system-xtensa",
	Version: "v9.2.2-meshbench-sx1262-10",
	MCU:     "ESP32",
	Why: "the emulator for the ESP32 family, carrying our SX1262 device, its " +
		"DIO1 line and the GPIO implementation upstream has not got",
	Terms: qemuTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    qemuBase + "qemu-xtensa-softmmu-v9.2.2_meshbench_sx1262_10-x86_64-linux-gnu.tar.xz",
			SHA256: "8d5d4cd92ced6a6ebc7fceecbf6da77837beca7ae3ddb3f9706a213761b87cbe",
			Bytes:  17089336, Kind: tarXZ, Magic: elfAMD64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		},
		"darwin/arm64": {
			URL:    qemuBase + "qemu-xtensa-softmmu-v9.2.2_meshbench_sx1262_10-aarch64-apple-darwin.tar.xz",
			SHA256: "f6bf3e4d5fd7e9b66b9632d224ea4c22e860ff4eb5175913033d9f0e73c24f01",
			Bytes:  4573100, Kind: tarXZ, Magic: machARM64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		},
	},
	Unsupported: map[string]string{
		"windows/amd64": windowsFetchesNoEmulators,
		"linux/arm64":   qemuArm64LinuxIsUntried,
	},
}, {
	Name:    "renode",
	Version: "meshbench-20260901-ca9f7e3",
	MCU:     "nRF52",
	Why: "the emulator for the nRF52 boards, carrying the SEVONPEND fix without " +
		"which the firmware sleeps for ever with its wake condition already true",
	Terms: renodeTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    renodeBase + "renode-1.16.1.linux-portable-meshbench.tar.gz",
			SHA256: "f6ad9ce149be700f4d51040e5c370c4ea89735695fe61e6f8159ead04c668b03",
			Bytes:  61647373, Kind: tarGzip, Magic: elfAMD64,
			Root: "renode_1.16.1-portable", Binary: "renode_1.16.1-portable/renode",
		},
	},
	Unsupported: map[string]string{
		"windows/amd64": windowsFetchesNoEmulators,
		// The macOS asset is a build tree rather than the portable package the
		// Linux one is: it has no launcher, and nothing here has ever started
		// it. Saying that is better than shipping 88 MB and a guess.
		"darwin/arm64": "the fork publishes a macOS build tree rather than a " +
			"portable package, and no nRF52 board has been brought up on macOS. " +
			"Build Renode from the meshbench-main branch of MeshBench/renode and put " +
			"it in this directory, or set MESHBENCH_RENODE",
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
