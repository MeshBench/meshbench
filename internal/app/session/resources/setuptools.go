// The half of the readiness check that is about this machine rather than about
// a cache: which artefact this build came out of, and whether an emulated node
// would find the three binaries it starts.
//
// Split from the check itself because it asks a different question of a
// different thing. The cache listing measures the tools directory, which is
// the honest answer to "what is on the disk"; a node starting up searches four
// places, and a tarball install carries its emulators in one of the other
// three. A readiness check that disagreed with the boot it is predicting would
// send somebody to download what they already have.
package resources

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/resource"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
	"github.com/MeshBench/meshbench/internal/firmware/emulated"
)

// toolsFound is where each emulator tool would actually be started from, by the
// same lookup a boot does, keyed by name and absent when there is none.
func toolsFound() map[string]string {
	out := map[string]string{}
	for name := range emulated.ToolEnv {
		if p, err := emulated.FindTool(name); err == nil {
			out[name] = p
		}
	}
	return out
}

// buildGroup says which artefact this is, because nothing else does.
//
// It is the first thing a new install needs and the last thing it can find
// out: the tarball carries the emulators, the AppImage and the .deb carry only
// radioserver, and a source checkout carries nothing - and all three look
// identical from inside the window. Deduced from what is beside the binary
// rather than stamped at build time, so it stays true for somebody who
// unpacked half of one bundle next to another.
func buildGroup(found map[string]string, ver state.SetupRow) state.SetupGroup {
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	return state.SetupGroup{
		Name: "This build",
		Note: "Everything below is per machine, not per session: what is " +
			"fetched here is fetched once and every scenario afterwards finds it.",
		Rows: []state.SetupRow{{
			Name:  "this build",
			State: string(state.SetupReady),
			What: "MeshBench " + version.Detail() + ", running on " +
				runtime.GOOS + "/" + runtime.GOARCH,
			Where: exe,
			Do:    artefactWords(found, exe),
		}, ver},
	}
}

// artefactWords names the bundle and says what follows from it.
func artefactWords(found map[string]string, exe string) string {
	beside, emulators := 0, 0
	for name, p := range found {
		if exe == "" || !besideBinary(p) {
			continue
		}
		beside++
		if name != "radioserver" {
			emulators++
		}
	}
	switch {
	case emulators > 0:
		return "this is laid out like a release tarball: the emulators are " +
			"beside the binary, so nothing below has to be fetched for an " +
			"emulated board."
	case beside > 0:
		return "this is laid out like an AppImage or a .deb: they carry " +
			"radioserver beside the binary but not the emulators, which are " +
			"most of the difference between a 26 MB AppImage and a 110 MB " +
			"tarball. Fetch them below if you want emulated boards."
	default:
		return "nothing is bundled beside the binary, which is what a source " +
			"checkout looks like. Everything below is fetched or built, and " +
			"the tarball is the download that would have carried it."
	}
}

// toolchainGroup is what an emulated board needs before it can start, and the
// four places each of them is looked for.
func toolchainGroup(rows []state.ResourceRow, found map[string]string) state.SetupGroup {
	var tools, softdevices []state.SetupRow
	for _, r := range rows {
		switch resource.Kind(r.Kind) {
		case resource.ToolchainKind:
			tools = append(tools, toolRow(r, found))
		case resource.SoftDeviceKind:
			softdevices = append(softdevices, softDeviceRow(r))
		}
	}
	return state.SetupGroup{
		Name: "Emulator toolchain",
		Note: "Each of these is looked for where " +
			emulated.EnvRadioLib + ", " + emulated.EnvQEMU + " or " +
			emulated.ToolEnv["renode"] + " points, then beside the binary, " +
			"then in " + emulated.ToolsDir() + ", then on PATH. PATH is the " +
			"one that will not save you: a desktop application is not launched " +
			"from a shell and inherits no shell environment, so a tool " +
			"installed by a package manager is both invisible here and the " +
			"wrong build. Native nodes need none of this.",
		Rows: append(tools, softdevices...),
	}
}

// toolRow is one tool, answered by the lookup first and the cache listing
// second: found is found, wherever it was found, and only then does it matter
// what could be downloaded.
func toolRow(r state.ResourceRow, found map[string]string) state.SetupRow {
	row := state.SetupRow{Name: r.Name, What: r.Why, Cost: costWords(r)}
	if p, ok := found[r.Name]; ok {
		row.State, row.Where = string(state.SetupReady), p
		row.Do = "found " + whereFound(p) + "."
		return row
	}
	if resource.State(r.State) == resource.Unavailable {
		row.State = string(state.SetupBlocked)
		row.What = toolPurpose(r.Name) + ", and there is no build of it here " +
			"for " + runtime.GOOS + "/" + runtime.GOARCH
		row.Do, row.Cost = r.Why, ""
		return row
	}
	row.State = string(state.SetupMissing)
	if resource.State(r.State) == resource.Needed {
		row.State = string(state.SetupNeeded)
	}
	row.Do = "fetch it: it lands in " + emulated.ToolsDir() +
		", which is where a node looks." + sourceBuildHint(r.Name)
	row.Verb = "resource.fetch"
	row.Params = map[string]any{"name": r.Name, "version": r.Version, "kind": r.Kind}
	return row
}

// sourceBuildHint is the step the application cannot take for somebody. The
// emulators are forks with a build each; radioserver is forty kilobytes and one
// command, and a source checkout already has everything it needs to make it.
func sourceBuildHint(name string) string {
	if name != "radioserver" {
		return ""
	}
	return " From a source checkout you can also build it yourself: " +
		"./build.sh radioserver out in a MeshBench/meshcore-native clone, then " +
		"copy the binary into that directory."
}

// toolPurpose says what a tool is for in the one case the catalogue's own
// sentence is not available: an unavailable row spends its reason explaining
// the refusal instead.
func toolPurpose(tool string) string {
	switch tool {
	case "radioserver":
		return "the SX1262 model both emulators reach over a socket, which " +
			"every emulated node needs whatever its MCU"
	case "renode":
		return "the emulator the nRF52 boards are started under"
	default:
		return "the emulator the ESP32 family is started under"
	}
}

func softDeviceRow(r state.ResourceRow) state.SetupRow {
	row := state.SetupRow{
		Name: r.Name + " SoftDevice", What: r.Why, Cost: costWords(r),
		Where: r.Path,
	}
	switch resource.State(r.State) {
	case resource.OnDisk, resource.InUse:
		row.State = string(state.SetupReady)
		row.Do = "nothing to do; nRF52 boards boot against it."
		return row
	case resource.Needed:
		row.State = string(state.SetupNeeded)
	default:
		row.State = string(state.SetupMissing)
	}
	row.Do = "fetched from Nordic on request, under their terms, which the " +
		"row on Resources shows before the download rather than after it."
	row.Verb = "resource.fetch"
	row.Params = map[string]any{"name": r.Name, "version": r.Version, "kind": r.Kind}
	return row
}

// whereFound says which of the four locations answered, because they do not
// mean the same thing. Only the last of them can be somebody else's build.
func whereFound(p string) string {
	if underDir(p, emulated.ToolsDir()) {
		return "in the tools directory, where a fetch puts it"
	}
	if besideBinary(p) {
		return "beside the binary, where a release bundle puts it"
	}
	return "at " + p + ", which is ours only if you built it there: a " +
		"distribution QEMU carries no SX1262 device, and a stock Renode runs " +
		"as far as the sleep the SEVONPEND fix exists for and stops"
}

// besideBinary is the bundle test, and the tools directory is deliberately
// excluded from it.
//
// One can contain the other. A binary run out of /tmp has the whole cache
// underneath it, so a freshly fetched radioserver read as bundled - which is
// the one distinction this row exists to make, because a fetch and a bundle put
// a tool in different places and say different things about the install.
func besideBinary(p string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return underDir(p, filepath.Dir(exe)) && !underDir(p, emulated.ToolsDir())
}

// underDir reports whether a path is inside a directory, with an empty
// directory matching nothing rather than everything.
func underDir(path, dir string) bool {
	if dir == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// costWords prices a row before it is spent, and marks a guess as one.
func costWords(r state.ResourceRow) string {
	if r.Bytes <= 0 {
		return ""
	}
	if r.Estimated {
		return "about " + resource.SIBytes(r.Bytes) + " to download, once"
	}
	return resource.SIBytes(r.Bytes) + " on disk"
}
