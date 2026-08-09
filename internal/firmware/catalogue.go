package firmware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Image is one prebuilt firmware a user can run.
type Image struct {
	// Board is the hardware, Role is repeater / companion / room-server, and
	// Variant separates builds of the same thing — usb versus ble companions,
	// most often.
	Board   string
	Role    string
	Variant string

	Version string
	URL     string
	Size    int64

	// Asset is the published filename. Kept because it is what the cache is
	// keyed on: deriving a name from the URL loses it — release URLs are
	// frequently opaque — and an offline catalogue then cannot tell what it
	// already has.
	Asset string

	// SHA256 as published, if the release carries one. Empty means the release
	// did not publish a digest, which is worth knowing rather than papering
	// over: an unverified image is still runnable, but it is not the same claim.
	SHA256 string
}

// Name is how an image is referred to on the command line and in a scenario.
func (i Image) Name() string {
	parts := []string{i.Board, i.Role}
	if i.Variant != "" {
		parts = append(parts, i.Variant)
	}
	return strings.Join(parts, "/")
}

// Catalogue is the set of prebuilt images, and the local store of downloaded
// ones.
//
// This is the default path on purpose. The user is an RF engineer, not a
// software engineer: "pick your board from a list" is the experience, and
// building firmware from source is the exception rather than the entry point.
type Catalogue struct {
	// ReleasesURL lists releases. Defaults to the MeshCore repository.
	ReleasesURL string
	// CacheDir holds downloaded images.
	CacheDir string
	HTTP     Doer

	// Offline serves only what is already downloaded.
	Offline bool
}

// Doer is the slice of http.Client used here.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// DefaultReleasesURL is the MeshCore firmware release index.
const DefaultReleasesURL = "https://api.github.com/repos/meshcore-dev/MeshCore/releases"

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
		// GitHub publishes a digest on newer releases; older ones have none.
		Digest string `json:"digest"`
	} `json:"assets"`
}

// List fetches the catalogue.
//
// Only .uf2 and .bin assets, because those are the things that can be flashed
// or emulated. A release also carries source archives and merged images for
// factory programming, and offering those in a "pick your board" list is how
// someone ends up trying to run a zip.
func (c *Catalogue) List(ctx context.Context) ([]Image, error) {
	url := c.ReleasesURL
	if url == "" {
		url = DefaultReleasesURL
	}
	if c.Offline {
		return c.listCached()
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firmware: fetch catalogue: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firmware: catalogue returned %s", resp.Status)
	}

	var releases []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("firmware: unparseable catalogue: %w", err)
	}

	var out []Image
	for _, rel := range releases {
		for _, a := range rel.Assets {
			ext := strings.ToLower(filepath.Ext(a.Name))
			if ext != ".uf2" && ext != ".bin" {
				continue
			}
			img := parseAssetName(a.Name)
			if img.Board == "" {
				continue
			}
			img.Version = rel.TagName
			img.Asset = a.Name
			img.URL = a.URL
			img.Size = a.Size
			img.SHA256 = strings.TrimPrefix(a.Digest, "sha256:")
			out = append(out, img)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Board != out[j].Board {
			return out[i].Board < out[j].Board
		}
		return out[i].Name() < out[j].Name()
	})
	return out, nil
}

// parseAssetName splits a MeshCore release asset name.
//
// The shape is <Board>_<role>[_<variant>]-v<version>-<sha>.uf2 — for example
// RAK_4631_repeater-v1.17.0-727fc05.uf2, or
// Heltec_t114_companion_radio_ble-v1.17.0-727fc05.uf2. Parsed rather than
// pattern-matched against a hard-coded board list, because the list of
// supported boards changes with every release and a hard-coded one silently
// hides new hardware.
func parseAssetName(name string) Image {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	stem := base
	if i := strings.Index(base, "-v"); i > 0 {
		stem = base[:i]
	}

	roles := []string{"companion_radio_ble", "companion_radio_usb", "companion_radio",
		"room_server", "repeater", "companion", "sensor"}
	for _, role := range roles {
		marker := "_" + role
		i := strings.LastIndex(stem, marker)
		if i <= 0 {
			continue
		}
		img := Image{Board: stem[:i]}
		switch {
		case strings.HasSuffix(role, "_ble"):
			img.Role, img.Variant = "companion", "ble"
		case strings.HasSuffix(role, "_usb"):
			img.Role, img.Variant = "companion", "usb"
		case role == "companion_radio":
			img.Role = "companion"
		default:
			img.Role = strings.ReplaceAll(role, "_", "-")
		}
		return img
	}
	return Image{}
}

// Fetch downloads an image and returns the path to it, verifying the digest.
//
// A wrong image is not a crash. It boots, or does not, on hardware it was never
// built for, and the resulting behaviour is indistinguishable from a firmware
// bug — so verification is not optional when a digest exists.
func (c *Catalogue) Fetch(ctx context.Context, img Image) (string, error) {
	if c.CacheDir == "" {
		return "", fmt.Errorf("firmware: catalogue needs a cache directory")
	}
	asset := img.Asset
	if asset == "" {
		asset = filepath.Base(img.URL)
	}
	dest := filepath.Join(c.CacheDir, img.Version, asset)

	if b, err := os.ReadFile(dest); err == nil {
		if err := verify(b, img.SHA256); err != nil {
			// A cached file that no longer matches is corruption or a moved
			// tag. Removing and refetching is right; serving it is not.
			_ = os.Remove(dest)
		} else {
			return dest, nil
		}
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", fmt.Errorf("firmware: read %s: %w", img.Name(), err)
	}
	if err := verify(body, img.SHA256); err != nil {
		return "", fmt.Errorf("firmware: %s: %w", img.Name(), err)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("firmware: cache directory: %w", err)
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return "", fmt.Errorf("firmware: write %s: %w", dest, err)
	}
	return dest, nil
}

// verify checks a digest when one was published, and says so when none was.
func verify(body []byte, want string) error {
	if want == "" {
		return nil // nothing published to check against
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: got %s, published %s", got, want)
	}
	return nil
}

// Verified reports whether an image carries a published digest. Surfaced rather
// than hidden: running an unverified image is a decision, not a detail.
func (i Image) Verified() bool { return i.SHA256 != "" }

// Import takes a user's own .bin or .uf2 into the cache.
//
// Supported deliberately. The catalogue is the default, not the only way: an
// engineer testing a build of their own should not have to publish a release
// first. Imported images have no digest and report as unverified, which is
// accurate rather than pessimistic.
func (c *Catalogue) Import(path, board, role string) (Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Image{}, fmt.Errorf("firmware: import %s: %w", path, err)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".uf2" && ext != ".bin" && ext != ".elf" {
		return Image{}, fmt.Errorf("firmware: %s is not a firmware image (.uf2, .bin or .elf)", path)
	}
	if board == "" {
		return Image{}, fmt.Errorf("firmware: an imported image needs a board; nothing in the file says which")
	}

	dest := filepath.Join(c.CacheDir, "imported", filepath.Base(path))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return Image{}, fmt.Errorf("firmware: cache directory: %w", err)
	}
	if err := os.WriteFile(dest, b, 0o644); err != nil {
		return Image{}, fmt.Errorf("firmware: store import: %w", err)
	}
	return Image{
		Board: board, Role: role, Version: "imported",
		Asset: filepath.Base(path), URL: dest, Size: int64(len(b)),
	}, nil
}

func (c *Catalogue) listCached() ([]Image, error) {
	var out []Image
	err := filepath.WalkDir(c.CacheDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".uf2" && ext != ".bin" {
			return nil
		}
		img := parseAssetName(filepath.Base(p))
		if img.Board == "" {
			return nil
		}
		img.Asset = filepath.Base(p)
		img.URL = p
		img.Version = filepath.Base(filepath.Dir(p))
		out = append(out, img)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("firmware: scan cache: %w", err)
	}
	return out, nil
}

// Boards lists the distinct boards in a catalogue, which is what a picker shows
// first.
func Boards(images []Image) []string {
	seen := map[string]bool{}
	var out []string
	for _, i := range images {
		if !seen[i.Board] {
			seen[i.Board] = true
			out = append(out, i.Board)
		}
	}
	sort.Strings(out)
	return out
}
