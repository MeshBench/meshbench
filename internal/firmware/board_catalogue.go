package firmware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// BoardImage is one published firmware for real hardware.
//
// The flasher at meshcore.io serves these; it is a browser client over the same
// GitHub release assets, and there is no separate manifest to consume. So the
// asset names are the catalogue:
//
//	Heltec_v3_repeater-v1.17.0-727fc05-merged.bin
//	└─ board ─┘└─ role ─┘└ version ┘└ sha ┘└ full flash image
type BoardImage struct {
	Board   string
	Role    string
	Version string
	Commit  string

	// Merged marks a complete flash image: bootloader, partition table and
	// application in one file. That is what an emulator wants handed to it; the
	// bare .bin is the application alone and boots nothing on its own.
	Merged bool

	// Format is the extension, without the dot. bin for ESP32 family, uf2 for
	// the nRF52 boards, and those are not interchangeable.
	Format string

	Name  string
	URL   string
	Bytes int64
}

// assetPattern splits a published asset name.
//
// Roles are lowercase words with underscores; boards are everything before
// them and may contain hyphens, dots and underscores of their own, so the split
// is anchored on the role rather than done by counting separators.
var assetPattern = regexp.MustCompile(
	`^(?i)(.+?)_(repeater|companion|room[-_]server)-v([0-9][^-]*)-([0-9a-f]+)(-merged)?\.(bin|uf2|hex|zip)$`)

// ParseAssetName reads a published asset name, or reports that it is not one.
func ParseAssetName(name string) (BoardImage, bool) {
	m := assetPattern.FindStringSubmatch(name)
	if m == nil {
		return BoardImage{}, false
	}
	role := strings.ToLower(strings.ReplaceAll(m[2], "-", "_"))
	// The tag says "repeater"; the application is called simple_repeater, and
	// that is the name the runner and the wiring both use.
	switch role {
	case "repeater":
		role = "simple_repeater"
	case "companion":
		role = "companion_radio"
	case "room_server":
		role = "simple_room_server"
	}
	return BoardImage{
		Board:   m[1],
		Role:    role,
		Version: "v" + m[3],
		Commit:  m[4],
		Merged:  m[5] != "",
		Format:  strings.ToLower(m[6]),
		Name:    name,
	}, true
}

// BoardCatalogue lists the published board images for a release.
//
// Same source as the flasher, and the same source NativeCatalogue already
// talks to, so a machine that can reach one can reach the other.
type BoardCatalogue struct {
	// Repo is owner/name; empty means MeshCore's own.
	Repo string
	HTTP Doer

	// CacheDir is where downloads land.
	CacheDir string
}

func (c *BoardCatalogue) repo() string {
	if c.Repo == "" {
		return "meshcore-dev/MeshCore"
	}
	return c.Repo
}

// List returns every image published under a release tag.
//
// Tags are per role upstream - repeater-v1.17.0 carries no companion build - so
// listing is per tag rather than per version.
func (c *BoardCatalogue) List(ctx context.Context, tag string) ([]BoardImage, error) {
	var payload struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", c.repo(), tag)
	body, err := c.get(ctx, url, "application/vnd.github+json")
	if err != nil {
		return nil, fmt.Errorf("firmware: listing %s: %w", tag, err)
	}
	defer func() { _ = body.Close() }()
	if err := json.NewDecoder(io.LimitReader(body, 32<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("firmware: unparseable release %s: %w", tag, err)
	}

	var out []BoardImage
	for _, a := range payload.Assets {
		img, ok := ParseAssetName(a.Name)
		if !ok {
			continue
		}
		img.URL, img.Bytes = a.URL, a.Size
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Board != out[j].Board {
			return out[i].Board < out[j].Board
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Runnable narrows a listing to what an emulator could actually boot.
//
// Two conditions, and both are about honesty rather than tidiness. A bare .bin
// is the application without a bootloader, so it starts nothing; and a board we
// have no wiring for cannot be pointed at a radio, which produces a driver
// reporting no chip and looks like a broken emulator.
func Runnable(images []BoardImage, wired func(board string) bool) []BoardImage {
	var out []BoardImage
	for _, img := range images {
		if !img.Merged || img.Format != "bin" {
			continue
		}
		if wired != nil && !wired(img.Board) {
			continue
		}
		out = append(out, img)
	}
	return out
}

// BoardImagePath is where a downloaded image lives, and where the runner looks.
func BoardImagePath(cacheDir string, img BoardImage) string {
	return filepath.Join(cacheDir, boardDir, img.Board,
		img.Role+"-"+img.Version+"."+img.Format)
}

// Ensure downloads an image if it is not already cached, and returns its path.
func (c *BoardCatalogue) Ensure(ctx context.Context, img BoardImage) (string, error) {
	dst := BoardImagePath(c.CacheDir, img)
	if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
		return dst, nil
	}
	if img.URL == "" {
		return "", fmt.Errorf("firmware: %s has no download URL", img.Name)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	body, err := c.get(ctx, img.URL, "application/octet-stream")
	if err != nil {
		return "", fmt.Errorf("firmware: downloading %s: %w", img.Name, err)
	}
	defer func() { _ = body.Close() }()

	// Not executable: a flash image is data for an emulator, not a program for
	// this machine, and marking it runnable invites exactly that mistake.
	//
	// Written to a temporary name and renamed, so an interrupted download
	// cannot leave a half-file that looks cached and boots nothing.
	tmp := dst + ".part"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// get is the package's usual request shape: authenticated when the environment
// allows, because sixty unauthenticated calls an hour is one bad afternoon.
func (c *BoardCatalogue) get(ctx context.Context, url, accept string) (io.ReadCloser, error) {
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return resp.Body, nil
}
