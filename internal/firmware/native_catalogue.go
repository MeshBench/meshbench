package firmware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

// NativeReleasesURL indexes the host builds of MeshCore.
//
// A separate repository from the emulated images, and from this one: those
// binaries link MeshCore, so they are published under MeshCore's own MIT licence
// rather than from a project that has not chosen one (ADR-0020).
const NativeReleasesURL = "https://api.github.com/repos/MeshBench/meshcore-native/releases"

// NativeImage is one host build: an application, a MeshCore version, and a
// machine to run it on.
//
// There is no board here, and that is the whole distinction from Image. A native
// node is not pretending to be a RAK4631 — it is MeshCore's own application
// compiled for the operator's machine, and what makes it a repeater rather than
// a companion is only which application was linked.
type NativeImage struct {
	// Role is the MeshCore application, named as upstream names its example
	// directory: simple_repeater, companion_radio, and whatever it ships next.
	Role string

	// Version is the upstream ref: a tag, or main or dev.
	Version string

	OS, Arch string

	Asset  string
	URL    string
	Size   int64
	SHA256 string
}

// Name is how an image is referred to on the command line and in a scenario.
func (i NativeImage) Name() string {
	return fmt.Sprintf("%s@%s (%s-%s)", i.Role, i.Version, i.OS, i.Arch)
}

// ForThisMachine reports whether the image will run here.
func (i NativeImage) ForThisMachine() bool {
	return i.OS == runtime.GOOS && i.Arch == runtime.GOARCH
}

// NativeCatalogue lists and downloads host builds.
type NativeCatalogue struct {
	// ReleasesURL defaults to NativeReleasesURL.
	ReleasesURL string
	// CacheDir holds downloaded binaries.
	CacheDir string
	HTTP     Doer
	// Offline serves only what is already downloaded.
	Offline bool

	// The release listing, memoised. One GitHub API call answers for every
	// node and every question for the next while — the unauthenticated rate
	// limit is sixty calls an hour, and a 613-node scenario that listed per
	// node burned the whole budget on its first attach and got 403s for the
	// rest (which read as "firmware missing", the worst possible spelling of
	// "slow down").
	mu       sync.Mutex
	listing  []NativeImage
	listedAt time.Time
	listErr  error
}

// CachedImages lists every build already downloaded for this machine.
//
// The catalogue's disk half. It answers with no network at all, which is what
// lets a picker show something real — "these will run right now" — while the
// listing is rate-limited, offline, or slow.
func (c *NativeCatalogue) CachedImages() []NativeImage {
	if c.CacheDir == "" {
		return nil
	}
	root := filepath.Join(c.CacheDir, "native")
	versions, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []NativeImage
	for _, v := range versions {
		if !v.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, v.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			img, ok := parseNativeAsset(f.Name())
			if !ok || !img.ForThisMachine() {
				continue
			}
			img.Version = v.Name()
			img.Asset = f.Name()
			out = append(out, img)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		return out[i].Role < out[j].Role
	})
	return out
}

// cachedBinary reports whether a verified download for (role, version) is
// already on disk.
func (c *NativeCatalogue) cachedBinary(role, version string) (string, bool) {
	if c.CacheDir == "" {
		return "", false
	}
	name := fmt.Sprintf("meshcore-%s-%s-%s", role, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(c.CacheDir, "native", version, name)
	if st, err := os.Stat(p); err == nil && st.Size() > 0 {
		return p, true
	}
	return "", false
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// Resolve finds a runnable build of a role, downloading one if there is none.
//
// Local first. A build sitting beside the simulator or named by the environment
// is almost always someone's own, and silently preferring a published binary
// over the one they just compiled is how an afternoon disappears.
//
// Only then the catalogue, because "run real firmware" should run real firmware
// rather than explain how to obtain some. The download is one file and it is
// cached, so the cost is paid once.
func Resolve(ctx context.Context, explicit, role, version, cacheDir string) (string, error) {
	if role == "" {
		role = DefaultRole
	}
	if path, err := FindNative(explicit, role); err == nil {
		return path, nil
	} else if explicit != "" {
		return "", err
	}
	if cacheDir == "" {
		return "", fmt.Errorf("firmware: no %s build found and nowhere to cache one", role)
	}
	c := sharedCatalogue(cacheDir)
	path, err := c.Ensure(ctx, role, version)
	if err != nil {
		return "", fmt.Errorf("%w\n\nBuild one with meshcore-native's build.sh, or set %s to your own",
			err, EnvNativeBinary)
	}
	return path, nil
}

// sharedCatalogue hands out one catalogue per cache directory, so the memoised
// release listing is shared by everything in the process. Without this each
// Resolve built a fresh catalogue and the memoisation protected nobody.
var (
	catMu    sync.Mutex
	catByDir = map[string]*NativeCatalogue{}
)

func sharedCatalogue(cacheDir string) *NativeCatalogue {
	catMu.Lock()
	defer catMu.Unlock()
	c, ok := catByDir[cacheDir]
	if !ok {
		c = &NativeCatalogue{CacheDir: cacheDir}
		catByDir[cacheDir] = c
	}
	return c
}

// DefaultCacheDir is where downloaded firmware is kept.
func DefaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "firmware"
	}
	return filepath.Join(dir, "meshcoresim", "firmware")
}
