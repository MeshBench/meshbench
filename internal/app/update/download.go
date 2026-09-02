package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/resource"
)

// Getting a release onto the disk, and what "verified" honestly means here.
//
// The emulator toolchain is pinned: its digests are in the source, written
// against a file somebody unpacked and ran, so a mismatch means the asset was
// replaced under its tag and the fetch refuses. A release that does not exist
// yet cannot be pinned that way. What it does have is SHA256SUMS, published
// beside the artefacts by the same pipeline that built them, and that is what
// this checks against.
//
// The difference is worth being plain about, because it is the difference
// between two things that both get called "verified": a pinned digest says the
// file is the one this build was written against, and a digest fetched from the
// same release says the download arrived intact. It catches a truncated
// download, a proxy that mangled it, and a mirror serving something else. It
// cannot catch a release page that is not ours, so what authenticates the
// release is the TLS connection to github.com - which is why an asset served
// over anything else is refused outright below.

// maxSums bounds the checksum file. The real one is a few hundred bytes.
const maxSums = 1 << 20

// Stage downloads a release asset and leaves it beside this build rather than
// on top of it.
type Stage struct {
	// Dir is where staged downloads land, one directory per release tag.
	Dir  string
	HTTP resource.HTTPDoer
	// Redirected relaxes the host check, and is only ever true when somebody
	// pointed this at a feed of their own. The verb that sets it says so in
	// every answer, because a check that quietly asked somewhere else is worse
	// than one that failed.
	Redirected bool
}

// Get fetches the asset, checks it against the release's own SHA256SUMS, and
// returns where it landed.
func (s Stage) Get(ctx context.Context, rel Release, a Asset,
	progress func(done, total int64)) (string, error) {

	if s.Dir == "" {
		return "", fmt.Errorf("update: nowhere to put %s", a.Name)
	}
	if err := s.check(a.URL); err != nil {
		return "", err
	}
	sums, ok := rel.Find(SumsAsset)
	if !ok {
		return "", fmt.Errorf("update: release %s publishes no %s, so nothing "+
			"here can check what it downloads: take it from %s by hand",
			rel.Tag, SumsAsset, rel.Notes)
	}
	if err := s.check(sums.URL); err != nil {
		return "", err
	}
	want, err := s.digestOf(ctx, sums, a.Name)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(s.Dir, safeTag(rel.Tag))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := resource.Verified{
		URL: a.URL, SHA256: want, Bytes: a.Bytes,
		Name: a.Name, Dir: dir, HTTP: s.HTTP,
	}.To(ctx, progress)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(a.Name))
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	// An AppImage is the application itself rather than an archive holding
	// one, and a downloaded application nobody can execute is not one anybody
	// can take. The archives are left alone: what comes out of them carries
	// its own modes.
	if strings.HasSuffix(dst, ".AppImage") {
		if err := os.Chmod(dst, 0o755); err != nil { //nolint:gosec // an application has to be runnable
			return "", err
		}
	}
	return dst, nil
}

// digestOf reads the release's checksum file and finds the line for one asset.
func (s Stage) digestOf(ctx context.Context, sums Asset, name string) (string, error) {
	body, err := s.text(ctx, sums.URL)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(body, "\n") {
		// "<hex>  <name>", which is what sha256sum writes and what the release
		// pipeline publishes.
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("update: %s does not list %s, so there is no digest "+
		"to check it against and it is not being downloaded", SumsAsset, name)
}

// text fetches a small file whole. Only the checksum list comes this way, and
// it is the one file here that cannot itself be checked against a digest.
func (s Stage) text(ctx context.Context, raw string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("update: fetching %s: %w", SumsAsset, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update: %s answered %s for %s",
			hostOf(raw), resp.Status, SumsAsset)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSums))
	if err != nil {
		return "", fmt.Errorf("update: reading %s: %w", SumsAsset, err)
	}
	return string(b), nil
}

// releaseHosts are where a published asset is actually served from: the
// release page redirects to GitHub's object storage, and both ends have to be
// on the list or the redirect looks like an attack.
var releaseHosts = []string{
	"github.com", "api.github.com",
	"objects.githubusercontent.com", "release-assets.githubusercontent.com",
}

// check refuses to fetch from anywhere the release feed does not serve from.
//
// The digest below comes from the same place as the file, so it proves the
// download is intact rather than that the release is ours. The thing that says
// the release is ours is the TLS connection, and it only says it about these
// hosts.
func (s Stage) check(raw string) error {
	if s.Redirected {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("update: %q is not a URL", raw)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("update: refusing to download over %s: a release is "+
			"only as trustworthy as the connection it arrived on", u.Scheme)
	}
	for _, h := range releaseHosts {
		if u.Host == h {
			return nil
		}
	}
	return fmt.Errorf("update: refusing to download from %s, which is not where "+
		"MeshBench releases are served from", u.Host)
}

// safeTag keeps a tag from naming a directory anywhere but under Dir. Tags are
// ours, but they arrive from a feed.
func safeTag(tag string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, tag)
	if out == "" || strings.Trim(out, ".") == "" {
		return "release"
	}
	return out
}
