// Getting a build onto this machine: the release list, the download, and the
// cache it lands in.
package firmware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

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
		if err := Verify(b, img.SHA256); err == nil {
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
	if err := Verify(body, img.SHA256); err != nil {
		return "", fmt.Errorf("firmware: %s: %w", img.Name(), err)
	}
	// The digest proves these are the published bytes, not that they are a
	// program this machine can run: a release built for the wrong platform
	// matches its own digest perfectly.
	if err := checkNativeExecutable(img.Name(), body); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("firmware: cache directory: %w", err)
	}
	// Executable, because unlike a .uf2 this is a program this machine runs.
	if err := os.WriteFile(dest, body, 0o755); err != nil {
		return "", fmt.Errorf("firmware: write %s: %w", dest, err)
	}
	// Recorded so a later cache hit, from either this path or fetchDirect, has
	// something on-disk corruption is checked against without keeping the
	// published digest around separately.
	if err := recordChecksum(dest, body); err != nil {
		return "", fmt.Errorf("firmware: recording %s's checksum: %w", dest, err)
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

	// Three things worth knowing when this fails, and the old message gave
	// none of them: whether the version exists at all, which platforms it was
	// built for, and which versions of this role *do* run here. It used to
	// answer with the roles published for the version while ignoring the
	// platform - so a Mac was told "has no simple_repeater build for
	// darwin-arm64; it publishes simple_repeater", which names the one thing
	// that was not missing.
	var roles, platforms, hereVersions []string
	for _, img := range images {
		if img.Role == role && img.ForThisMachine() {
			if img.Version == version {
				return c.Fetch(ctx, img)
			}
			if !contains(hereVersions, img.Version) {
				hereVersions = append(hereVersions, img.Version)
			}
		}
		if img.Version != version {
			continue
		}
		if !contains(roles, img.Role) {
			roles = append(roles, img.Role)
		}
		if img.Role == role {
			if p := img.OS + "-" + img.Arch; !contains(platforms, p) {
				platforms = append(platforms, p)
			}
		}
	}
	if len(roles) == 0 {
		return "", fmt.Errorf("firmware: no native builds published for MeshCore %s", version)
	}
	sort.Strings(roles)
	sort.Strings(platforms)
	sort.Strings(hereVersions)

	here := runtime.GOOS + "-" + runtime.GOARCH
	if len(platforms) == 0 {
		return "", fmt.Errorf(
			"firmware: MeshCore %s publishes no %s build at all (it has %s); "+
				"this machine is %s", version, role, strings.Join(roles, ", "), here)
	}
	msg := fmt.Sprintf("firmware: MeshCore %s has a %s build for %s, but not for %s",
		version, role, strings.Join(platforms, ", "), here)
	if len(hereVersions) > 0 {
		msg += fmt.Sprintf("; %s does run here as %s",
			role, strings.Join(hereVersions, ", "))
	} else {
		msg += fmt.Sprintf("; no version of %s is published for %s yet", role, here)
	}
	return "", errors.New(msg)
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
	// Checked before it is written, because this path has no published digest
	// to compare against and the file is about to be marked executable. A body
	// that is complete and wrong - an error page served with 200, an asset for
	// another platform - would otherwise be cached, run, and reported as every
	// node exiting 1 at once.
	if err := checkNativeExecutable(name, body); err != nil {
		return "", err
	}
	dest := filepath.Join(c.CacheDir, "native", version, name)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, body, 0o755); err != nil {
		return "", err
	}
	// No published digest to check this against - that is the whole point of
	// this path - but what it hands back can still be checked against itself
	// later: cachedBinary refuses to serve this file again once its bytes stop
	// matching what was written here.
	if err := recordChecksum(dest, body); err != nil {
		return "", err
	}
	return dest, nil
}

// listTTL is how long a memoised listing answers for. Releases change on the
// timescale of upstream merges, not seconds; anyone who needs fresher can make
// a new catalogue.
const listTTL = 10 * time.Minute
