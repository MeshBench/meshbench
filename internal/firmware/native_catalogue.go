package firmware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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

// listTTL is how long a memoised listing answers for. Releases change on the
// timescale of upstream merges, not seconds; anyone who needs fresher can make
// a new catalogue.
const listTTL = 10 * time.Minute

// List fetches every published host build, memoised for listTTL.
func (c *NativeCatalogue) List(ctx context.Context) ([]NativeImage, error) {
	c.mu.Lock()
	if time.Since(c.listedAt) < listTTL && (c.listing != nil || c.listErr != nil) {
		imgs, err := c.listing, c.listErr
		c.mu.Unlock()
		return imgs, err
	}
	c.mu.Unlock()

	imgs, err := c.fetchList(ctx)

	c.mu.Lock()
	c.listing, c.listErr, c.listedAt = imgs, err, time.Now()
	c.mu.Unlock()
	return imgs, err
}

func (c *NativeCatalogue) fetchList(ctx context.Context) ([]NativeImage, error) {
	url := c.ReleasesURL
	if url == "" {
		url = NativeReleasesURL
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("firmware: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// Authenticated when the environment allows: sixty unauthenticated calls
	// an hour is one bad afternoon, five thousand is not. The token is only
	// ever sent to the API host the catalogue points at.
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firmware: fetch native catalogue: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firmware: native catalogue returned %s", resp.Status)
	}

	var releases []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("firmware: unparseable native catalogue: %w", err)
	}

	var out []NativeImage
	for _, rel := range releases {
		for _, a := range rel.Assets {
			img, ok := parseNativeAsset(a.Name)
			if !ok {
				continue
			}
			img.Version = rel.TagName
			img.URL = a.URL
			img.Size = a.Size
			img.SHA256 = strings.TrimPrefix(a.Digest, "sha256:")
			out = append(out, img)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return out[i].Version < out[j].Version
		}
		return out[i].Role < out[j].Role
	})
	return out, nil
}

// parseNativeAsset splits meshcore-<role>-<os>-<arch>[.exe].
//
// The role is whatever is between the prefix and the platform, however many
// underscores it contains, because it is an upstream directory name and not one
// of ours. Matching against a list of known roles is how a node type MeshCore
// ships next year becomes invisible.
func parseNativeAsset(name string) (NativeImage, bool) {
	base := strings.TrimSuffix(name, ".exe")
	rest, ok := strings.CutPrefix(base, "meshcore-")
	if !ok {
		return NativeImage{}, false
	}
	dash := strings.LastIndex(rest, "-")
	if dash <= 0 {
		return NativeImage{}, false
	}
	arch := rest[dash+1:]
	rest = rest[:dash]
	dash = strings.LastIndex(rest, "-")
	if dash <= 0 {
		return NativeImage{}, false
	}
	return NativeImage{Role: rest[:dash], OS: rest[dash+1:], Arch: arch}, true
}

// Fetch downloads a host build and returns a path to it, ready to run.
func (c *NativeCatalogue) Fetch(ctx context.Context, img NativeImage) (string, error) {
	if c.CacheDir == "" {
		return "", fmt.Errorf("firmware: native catalogue needs a cache directory")
	}
	asset := img.Asset
	if asset == "" {
		asset = filepath.Base(img.URL)
	}
	dest := filepath.Join(c.CacheDir, "native", img.Version, asset)

	if b, err := os.ReadFile(dest); err == nil {
		if err := verify(b, img.SHA256); err == nil {
			return dest, nil
		}
		// A cached binary that no longer matches its digest is corruption or a
		// moved branch release. Refetching is right; running it is not.
		_ = os.Remove(dest)
	}
	if c.Offline {
		return "", fmt.Errorf("firmware: %s is not downloaded and downloads are off", img.Name())
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, img.URL, nil)
	if err != nil {
		return "", fmt.Errorf("firmware: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("firmware: download %s: %w", img.Name(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("firmware: download %s returned %s", img.Name(), resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return "", fmt.Errorf("firmware: read %s: %w", img.Name(), err)
	}
	if err := verify(body, img.SHA256); err != nil {
		return "", fmt.Errorf("firmware: %s: %w", img.Name(), err)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("firmware: cache directory: %w", err)
	}
	// Executable, because unlike a .uf2 this is a program this machine runs.
	if err := os.WriteFile(dest, body, 0o755); err != nil {
		return "", fmt.Errorf("firmware: write %s: %w", dest, err)
	}
	return dest, nil
}

// Ensure returns a runnable binary for a role, downloading it if necessary.
//
// Version may be a tag, "main", "dev", or empty for the newest stable — which is
// main, not dev: a simulation run against upstream's development branch is a
// legitimate thing to ask for and a poor thing to get by default.
func (c *NativeCatalogue) Ensure(ctx context.Context, role, version string) (string, error) {
	if version == "" {
		version = "main"
	}
	// Disk first for tagged versions: a tag is immutable, so a binary already
	// downloaded and verified is the answer, with no network involved at all.
	// Branches move, so main and dev do ask the network — but fall back to the
	// cached copy rather than failing, because a slightly stale dev build beats
	// a node that cannot start.
	tagged := version != "main" && version != "dev"
	if p, ok := c.cachedBinary(role, version); ok && tagged {
		return p, nil
	}

	// Straight at the asset, no API. Release downloads are plain web URLs and
	// are not metered like api.github.com — whose sixty-an-hour anonymous
	// limit is what turned a big scenario into a wall of 403s. The listing is
	// only needed when this guess misses and someone has to be told what does
	// exist.
	if p, err := c.fetchDirect(ctx, role, version); err == nil {
		return p, nil
	}

	images, err := c.List(ctx)
	if err != nil {
		if p, ok := c.cachedBinary(role, version); ok {
			return p, nil
		}
		return "", fmt.Errorf("%w\n\nThe GitHub API is also how the catalogue lists builds; if this is "+
			"a rate limit, set GITHUB_TOKEN (e.g. export GITHUB_TOKEN=$(gh auth token)) or wait an hour. "+
			"Already-downloaded firmware keeps working without it", err)
	}

	var roles []string
	for _, img := range images {
		if img.Version != version {
			continue
		}
		if !contains(roles, img.Role) {
			roles = append(roles, img.Role)
		}
		if img.Role == role && img.ForThisMachine() {
			return c.Fetch(ctx, img)
		}
	}
	if len(roles) == 0 {
		return "", fmt.Errorf("firmware: no native builds published for MeshCore %s", version)
	}
	sort.Strings(roles)
	return "", fmt.Errorf("firmware: MeshCore %s has no %s build for %s-%s; it publishes %s",
		version, role, runtime.GOOS, runtime.GOARCH, strings.Join(roles, ", "))
}

// fetchDirect downloads a release asset by its predictable URL.
//
// No digest check, because the digest lives in the API listing this exists to
// avoid. The trade is explicit: a file served over TLS from the release page
// itself, unverified, against a scenario that cannot start at all. The API
// path still verifies when it is the one used.
func (c *NativeCatalogue) fetchDirect(ctx context.Context, role, version string) (string, error) {
	if c.CacheDir == "" || c.Offline {
		return "", fmt.Errorf("firmware: no direct fetch without a cache directory")
	}
	name := fmt.Sprintf("meshcore-%s-%s-%s", role, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	// The releases URL is api.github.com/repos/<owner>/<repo>/releases; the
	// download host is github.com with the same owner and repo.
	base := c.ReleasesURL
	if base == "" {
		base = NativeReleasesURL
	}
	repoPath := strings.TrimSuffix(strings.TrimPrefix(base, "https://api.github.com/repos/"), "/releases")
	if strings.Contains(repoPath, "://") {
		// A non-GitHub catalogue (tests, mirrors) has no guessable asset URL.
		return "", fmt.Errorf("firmware: no direct-download form for %s", base)
	}
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repoPath, version, name)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("firmware: download %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("firmware: %s@%s is not published at %s (%s)", role, version, url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
	if err != nil {
		return "", err
	}
	dest := filepath.Join(c.CacheDir, "native", version, name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, body, 0o755); err != nil {
		return "", err
	}
	return dest, nil
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
