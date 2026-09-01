package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// httpDoer is the one method a fetch needs, so a test can answer without a
// network.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Fetch downloads one tool, verifies it, unpacks it and leaves it under the
// name the emulator lookup asks for.
//
// Streamed to a temporary file rather than held in memory: Renode's package is
// 61 MB packed and several times that unpacked, and holding it would put a
// spike on a machine that is about to run an emulator on it.
func (t *Toolchain) Fetch(ctx context.Context, name, _ string,
	progress func(done, total int64)) error {

	rel, ok := releaseNamed(name)
	if !ok {
		return fmt.Errorf("resource: no emulator tool called %s is known", name)
	}
	a, ok := rel.asset()
	if !ok {
		return fmt.Errorf("resource: %s", rel.unavailableBecause())
	}
	if t.Dir == "" {
		return fmt.Errorf("resource: no tools directory to put %s in", name)
	}
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}

	tmp, err := t.download(ctx, rel, a, progress)
	if err != nil {
		return err
	}
	// The archive is worth nothing once unpacked, and 61 MB of it is worth
	// less than nothing on the disk this page exists to account for.
	defer func() { _ = os.Remove(tmp) }()

	return t.install(rel, a, tmp)
}

func releaseNamed(name string) (toolRelease, bool) {
	for _, r := range toolReleases {
		if r.Name == name {
			return r, true
		}
	}
	return toolRelease{}, false
}

// download streams the asset to a temporary file, hashing as it goes, and
// refuses anything whose digest is not the one this was written against.
func (t *Toolchain) download(ctx context.Context, rel toolRelease, a toolAsset,
	progress func(done, total int64)) (string, error) {

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}
	client := t.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resource: fetching %s: %w", rel.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resource: %s answered %s for %s",
			"github.com", resp.Status, rel.Name)
	}

	f, err := os.CreateTemp(t.Dir, "."+rel.Name+"-*.part")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	// Reported by the asset's own size rather than the server's, because a
	// redirect can arrive with no Content-Length and a job that says "0 of 0"
	// on a 61 MB download reads as a stall.
	total := a.Bytes
	if resp.ContentLength > 0 {
		total = resp.ContentLength
	}
	sum := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, sum), &countingReader{
		r: resp.Body, total: total, report: everyPercent(total, progress),
	})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("resource: reading %s: %w", rel.Name, err)
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != a.SHA256 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("resource: %s is not the file this was written "+
			"against (sha256 %s, expected %s) - the download was truncated or the "+
			"asset was replaced, and either way it is not being unpacked",
			rel.Name, got, a.SHA256)
	}
	return tmp, nil
}

// install puts a verified download where the lookup will find it, and checks
// that what landed is an executable this machine can run before saying so.
func (t *Toolchain) install(rel toolRelease, a toolAsset, tmp string) error {
	link := t.installedAt(rel)
	if a.Kind == plainFile {
		// 0755 rather than something tighter: this is an executable, and a
		// downloaded emulator that cannot be executed is not an installation.
		if err := os.Chmod(tmp, 0o755); err != nil { //nolint:gosec // a tool has to be runnable
			return err
		}
		if err := os.Rename(tmp, link); err != nil {
			return err
		}
		return t.verify(link, rel, a)
	}
	// Replaced whole. An unpack over a half-written tree from an interrupted
	// fetch is how a tool ends up with one file from each of two releases.
	root := filepath.Join(t.Dir, a.Root)
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	if err := extractTar(tmp, t.Dir, a.Kind); err != nil {
		return err
	}
	inner := filepath.Join(t.Dir, filepath.FromSlash(a.Binary))
	if !fileExists(inner) {
		return fmt.Errorf("resource: %s unpacked without %s in it", rel.Name, a.Binary)
	}
	// A link rather than a copy, because both emulators resolve their own path
	// to find what sits beside them: QEMU's ROM images are in share/ next to
	// its bin/, and Renode's runtime walks its own directory. A bare copy of
	// the binary into the tools directory starts and then cannot find any of
	// it.
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(filepath.FromSlash(a.Binary), link); err != nil {
		return err
	}
	return t.verify(inner, rel, a)
}

// verify is the last check, and it deliberately runs after the file is in
// place: what has to be true is that the thing the lookup will pick up is
// runnable here, not that some file in a temporary directory was.
func (t *Toolchain) verify(path string, rel toolRelease, a toolAsset) error {
	if err := checkExecutable(path, a.Magic); err != nil {
		// Taken back out again. A tool that is present and unrunnable is worse
		// than one that is absent: absent has an error that says what to do.
		_ = os.Remove(t.installedAt(rel))
		if a.Root != "" {
			_ = os.RemoveAll(filepath.Join(t.Dir, a.Root))
		}
		return err
	}
	return nil
}

// everyPercent thins progress down to whole percentages.
//
// countingReader reports on every read, which on a 61 MB download is thousands
// of calls, and each one here becomes a command posted to the store's single
// goroutine. The progress bar cannot show more than a percent anyway.
func everyPercent(total int64, report func(done, total int64)) func(done, total int64) {
	if report == nil {
		return nil
	}
	last := int64(-1)
	return func(done, declared int64) {
		if total <= 0 {
			report(done, declared)
			return
		}
		pct := done * 100 / total
		if pct == last && done < total {
			return
		}
		last = pct
		report(done, total)
	}
}
