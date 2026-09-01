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
	qemuBase   = "https://github.com/MeshBench/qemu/releases/download/meshbench-20260901-6b3a41a/"
	renodeBase = "https://github.com/MeshBench/renode/releases/download/meshbench-20260814-339f4df/"
	radioBase  = "https://github.com/MeshBench/meshcore-native/releases/download/radioserver-v2/"
)

// qemuIsLinuxOnly is why the other platforms get no QEMU here.
//
// The fork publishes two kinds of release. The Espressif-derived job builds
// every platform, and its newest build predates the DIO1 wiring this version
// drives - an emulated board fetched from it dies at start with "Property
// 'esp32-machine.radio-dio1' not found", which reads as a broken emulator
// rather than as an old one. The fork's own job carries the wiring and builds
// Linux x86_64 only, its macOS and Windows legs disabled for want of a runner.
// So a current QEMU exists for one platform, and the honest answer elsewhere
// is to say where to get one rather than to serve a build that cannot boot a
// board.
const qemuIsLinuxOnly = "the fork's release job builds Linux x86_64 only, and " +
	"the multi-platform build predates the DIO1 wiring this version drives - a " +
	"board started against it dies asking for a machine property it has not got. " +
	"Build from the meshbench-sx1262 branch of MeshBench/qemu and put " +
	"qemu-system-xtensa in this directory, or set MESHBENCH_QEMU"

// windowsHasNoEmulation is one refusal said once. Builds are published for
// Windows and they would download perfectly; what does not exist there is a
// working emulated node, because the QEMU side of the radio model is a Unix
// socket and nothing has ever run this path on Windows. Offering the download
// would be offering three quarters of an hour of disk and bandwidth for a
// board that cannot come up.
const windowsHasNoEmulation = "emulation has never run on Windows: the radio " +
	"model reaches QEMU over a Unix socket, and the TCP path Renode uses has " +
	"not been wired for it. The download exists; the working node does not"

// toolReleases is every tool the emulator lookup asks for, in the order they
// are needed: nothing boots without radioserver, and which emulator follows
// depends on the board.
var toolReleases = []toolRelease{{
	Name:    "radioserver",
	Version: "v2",
	MCU:     "",
	Why: "the SX1262 model both emulators reach over a socket; every emulated " +
		"node needs it, ESP32 or nRF52",
	Terms: radioserverTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    radioBase + "radioserver-linux-amd64",
			SHA256: "0ae3a3b02965be43a729eba8639cffe4503c47540d95ef671db64520e8d97711",
			Bytes:  40952, Kind: plainFile, Magic: elfAMD64,
		},
		"darwin/arm64": {
			URL:    radioBase + "radioserver-darwin-arm64",
			SHA256: "f64d880d827757c581447b701395be2c199da2b336fe9571eb263a76d13f1253",
			Bytes:  56728, Kind: plainFile, Magic: machARM64,
		},
	},
	Unsupported: map[string]string{"windows/amd64": windowsHasNoEmulation},
}, {
	Name:    "qemu-system-xtensa",
	Version: "meshbench-20260901-6b3a41a",
	MCU:     "ESP32",
	Why: "the emulator for the ESP32 family, carrying our SX1262 device and " +
		"the GPIO implementation upstream has not got",
	Terms: qemuTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    qemuBase + "qemu-meshbench-linux-x86_64.tar.gz",
			SHA256: "d0902d9e7b90fda16360f686974daca9da60906b065c5de02c27f2d6564be152",
			Bytes:  45826158, Kind: tarGzip, Magic: elfAMD64,
			Root: "qemu-meshbench", Binary: "qemu-meshbench/bin/qemu-system-xtensa",
		},
	},
	Unsupported: map[string]string{
		"windows/amd64": windowsHasNoEmulation,
		"linux/arm64":   qemuIsLinuxOnly,
		"darwin/arm64":  qemuIsLinuxOnly,
		"darwin/amd64":  qemuIsLinuxOnly,
	},
}, {
	Name:    "renode",
	Version: "meshbench-20260814-339f4df",
	MCU:     "nRF52",
	Why: "the emulator for the nRF52 boards, carrying the SEVONPEND fix without " +
		"which the firmware sleeps for ever with its wake condition already true",
	Terms: renodeTerms,
	Assets: map[string]toolAsset{
		"linux/amd64": {
			URL:    renodeBase + "renode-1.16.1.linux-portable-meshbench.tar.gz",
			SHA256: "0b181a669fdab4b3fe10442a187f82a3d5dbfb4b2554a6baea064902f9d9b82e",
			Bytes:  61656398, Kind: tarGzip, Magic: elfAMD64,
			Root: "renode_1.16.1-portable", Binary: "renode_1.16.1-portable/renode",
		},
	},
	Unsupported: map[string]string{
		"windows/amd64": windowsHasNoEmulation,
		// The macOS asset is a build tree rather than the portable package the
		// Linux one is: it has no launcher, and nothing here has ever started
		// it. Saying that is better than shipping 88 MB and a guess.
		"darwin/arm64": "the fork publishes a macOS build tree rather than a " +
			"portable package, and no nRF52 board has been brought up on macOS. " +
			"Build Renode from the meshbench branch of MeshBench/renode and put " +
			"it in this directory, or set MESHBENCH_RENODE",
		"darwin/amd64": "no macOS Intel package is published",
	},
}}

// The terms each tool arrives under, readable before the download rather than
// after it. The full texts are in the application's own Licences window and in
// every release bundle's LICENCES directory; what belongs here is the part
// somebody has to have read before pressing Fetch.
const (
	radioserverTerms = "radioserver is built from MeshBench/meshcore-native, which " +
		"is MIT, and links MeshCore's own VirtualSX1262 (MIT, MeshCore contributors). " +
		"It runs as a separate process and is not linked into MeshBench."
	qemuTerms = "QEMU is GPL-2.0, with parts under compatible licences. This build " +
		"is our own fork - the meshbench-sx1262 branch of MeshBench/qemu, which is " +
		"public and is the source offer that licence requires. It is run as a " +
		"separate process and is never linked into MeshBench, so the two sit " +
		"beside each other as aggregation rather than as one work."
	renodeTerms = "Renode is MIT, from Antmicro, and this is the meshbench branch " +
		"of MeshBench/renode, which is public. The portable package carries a .NET " +
		"runtime (MIT, Microsoft) and renode-infrastructure (MIT, with LGPL parts) " +
		"inside it. Run as a separate process, never linked into MeshBench."
)
