package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Artefact is which bundle this build came out of, because what an update can
// honestly do differs per bundle and nothing else in the application knows.
type Artefact string

const (
	// Tarball is the Linux .tar.gz, which carries the emulators.
	Tarball Artefact = "tarball"
	// AppImage is the single-file Linux build.
	AppImage Artefact = "AppImage"
	// Deb is the Debian package, and it is the one that is not ours to
	// replace: apt owns those files and fighting it leaves a machine whose
	// package manager and whose disk disagree.
	Deb Artefact = "deb"
	// Bundle is the macOS .app inside the .dmg.
	Bundle Artefact = "app bundle"
	// Zip is the Windows build.
	Zip Artefact = "zip"
	// Loose is a binary somebody built or unpacked themselves, which is what
	// a source checkout looks like.
	Loose Artefact = "unpackaged"
)

// This is the artefact this process is running out of.
func This() Artefact {
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	return Detect(runtime.GOOS, exe, os.Getenv("APPIMAGE"))
}

// Detect deduces the bundle from where the binary is and what launched it.
//
// Deduced rather than stamped at build time, so it stays true for somebody who
// unpacked half of one bundle next to another - which is the same reason the
// readiness check deduces it, and the same evidence.
func Detect(goos, exe, appImage string) Artefact {
	if appImage != "" {
		// Set by the AppImage runtime itself, and by nothing else.
		return AppImage
	}
	if strings.Contains(filepath.ToSlash(exe), ".app/Contents/MacOS/") {
		return Bundle
	}
	switch goos {
	case "windows":
		return Zip
	case "darwin":
		return Loose
	}
	// A package manager put it there. Not necessarily dpkg - it is the only
	// package this project publishes, but the reasoning holds for any of them:
	// a file under /usr that something else installed is not a file this may
	// overwrite.
	if strings.HasPrefix(filepath.ToSlash(exe), "/usr/") {
		return Deb
	}
	if exe != "" && besideBinary(exe, "radioserver") {
		return Tarball
	}
	return Loose
}

func besideBinary(exe, name string) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(exe), name))
	return err == nil
}

// SumsAsset is the checksum file every release publishes beside its artefacts.
const SumsAsset = "SHA256SUMS"

// AssetFor picks the file this machine would take, or says why there is none.
//
// Matched by shape rather than by a name with a version in it: the published
// names carry the release number in four different places and one of them
// carries no number at all, so a table of literal names would go wrong at the
// first release that spelled one differently.
func AssetFor(r Release, a Artefact, goos, goarch string) (Asset, string) {
	if a == Deb {
		return Asset{}, "this build was installed by a package manager, which " +
			"owns these files: update it the way it was installed rather than " +
			"from here"
	}
	arch, ok := archWords(goarch)
	if !ok {
		return Asset{}, "nothing is published for " + goos + "/" + goarch
	}
	want := wanted(a, goos, arch)
	if want == nil {
		return Asset{}, "no build is published for " + goos + "/" + goarch +
			", so there is nothing here to take"
	}
	for _, asset := range r.Assets {
		if want(asset.Name) {
			return asset, ""
		}
	}
	return Asset{}, "release " + r.Tag + " published nothing for " + goos + "/" +
		goarch + ", so there is nothing here to take"
}

// archWords is the spellings a published asset uses for a machine.
func archWords(goarch string) ([]string, bool) {
	switch goarch {
	case "amd64":
		return []string{"x86_64", "amd64"}, true
	case "arm64":
		return []string{"arm64", "aarch64"}, true
	default:
		return nil, false
	}
}

// wanted is the test one asset name has to pass, or nil where this platform
// takes no published artefact.
func wanted(a Artefact, goos string, arch []string) func(string) bool {
	switch {
	case a == AppImage:
		return func(n string) bool { return strings.HasSuffix(n, ".AppImage") }
	case goos == "windows":
		return func(n string) bool {
			return strings.HasSuffix(n, ".zip") && has(n, "windows") && hasAny(n, arch)
		}
	case goos == "darwin":
		return func(n string) bool { return strings.HasSuffix(n, ".dmg") && hasAny(n, arch) }
	case goos == "linux":
		// Deliberately not the source archive, which is also a .tar.gz and is
		// the one thing on the page that is not a build.
		return func(n string) bool {
			return strings.HasSuffix(n, ".tar.gz") && has(n, "linux") &&
				hasAny(n, arch) && !has(n, "source")
		}
	default:
		return nil
	}
}

func has(name, word string) bool {
	return strings.Contains(strings.ToLower(name), word)
}

func hasAny(name string, words []string) bool {
	for _, w := range words {
		if has(name, w) {
			return true
		}
	}
	return false
}
