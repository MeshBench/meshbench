package update

import (
	"path/filepath"
	"strings"
)

// What to do with a download, per bundle, in words.
//
// Nothing here swaps anything, and that is the decision rather than an
// unfinished part of one. A run holds unsaved state - there is no autosave -
// and replacing the binary underneath it is a way to lose somebody's work in
// exchange for saving them one command. So the application fetches, checks, and
// says where it put it; the person decides when their session is finished.
//
// The per-platform differences below are real and they are the reason this is
// prose rather than a button. A running executable on Linux can be replaced by
// a rename while it runs, because the process holds the inode and the directory
// entry is what moves. On Windows it cannot: the file is locked for as long as
// it is mapped, which is what the usual wait-and-swap helper exists to work
// around. macOS sits between the two: the swap works, and the trap is the
// quarantine attribute that arrives with any downloaded file, on a bundle that
// is unsigned to begin with.

// Swap is the instruction for this bundle: what to do with staged, given that
// the running binary is exe.
func Swap(a Artefact, staged, exe string) string {
	where := filepath.Dir(staged)
	switch a {
	case Deb:
		return "This build came from the package manager, so the package " +
			"manager updates it: `sudo apt update && sudo apt install " +
			"--only-upgrade meshbench`. Nothing here will write over files apt " +
			"owns."
	case AppImage:
		return "Close MeshBench, then replace the AppImage with the one in " +
			where + ": `mv " + filepath.Base(staged) + " " + orTheOne(exe) +
			"`. A rename over a running binary is allowed on Linux - the " +
			"running process keeps the old file - but the new one only starts " +
			"the next time you do."
	case Tarball, Loose:
		return "Close MeshBench, unpack " + filepath.Base(staged) + " in " +
			where + ", and move what comes out over " + orTheDir(exe) +
			". Unpack rather than copy a single file: the tarball carries the " +
			"emulators and the fixtures with the binary, and taking only the " +
			"binary leaves a build whose parts came from two releases."
	case Bundle:
		return "Close MeshBench, open " + filepath.Base(staged) + ", and drag " +
			"MeshBench.app over the one in Applications. The build is " +
			"unsigned, so the first launch of the new one needs a right-click " +
			"and Open rather than a double-click, the same as the first one " +
			"did: macOS marks anything downloaded as quarantined and will " +
			"otherwise refuse it without saying why."
	case Zip:
		return "Close MeshBench first, which Windows requires rather than " +
			"prefers: a running .exe cannot be replaced while it is running. " +
			"Then unzip " + filepath.Base(staged) + " in " + where +
			" and move the meshbench folder over " + orTheDir(exe) + "."
	case Msi:
		return "Close MeshBench, then run " + filepath.Base(staged) + " in " +
			where + ". It replaces this installation in place - same " +
			"location, same Start menu entry, one entry in Apps and Features - " +
			"so there is nothing to uninstall first and no second copy left " +
			"behind. Do not unzip a build over " + orTheDir(exe) +
			" instead: the installer's record of what is there would then " +
			"describe files that are no longer the ones on disk."
	default:
		return "The download is in " + where + "; replace this build with it " +
			"when the session is finished."
	}
}

// CanSwapItself is whether the application could, in principle, replace its own
// binary on this platform - which is not the same as whether it does.
//
// Reported rather than acted on, because the interface has to be able to say
// which platforms get a real install and which get an instruction, and a page
// that claimed "restart to finish" while doing nothing would be worse than one
// that told the truth.
//
// Msi is false with the rest of Windows, and deliberately. The installer can
// replace an installed build and this application still cannot replace itself:
// the running .exe is mapped and locked either way, and what the .msi does is
// wait for it to close. The question here is what MeshBench can do to its own
// binary, not what somebody could do from outside it, and answering yes would
// promise a restart that finishes an update nobody started.
func CanSwapItself(a Artefact) bool {
	switch a {
	case AppImage, Tarball, Loose, Bundle:
		return true
	default:
		return false
	}
}

func orTheOne(exe string) string {
	if exe == "" {
		return "the one you are running"
	}
	return exe
}

func orTheDir(exe string) string {
	if exe == "" {
		return "the directory this one is in"
	}
	return strings.TrimSuffix(filepath.Dir(exe), string(filepath.Separator))
}
