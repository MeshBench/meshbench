package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// One download, checked before anything is allowed to use it.
//
// Written once and shared, because there is more than one thing on this
// machine that fetches a large file from a release page: the emulator
// toolchain, and the update that replaces this build. Two copies of "stream it,
// hash it, refuse it if the digest is wrong" would be two places for the
// refusal to be forgotten, and the refusal is the whole point.

// HTTPDoer is the one method a download needs, so a test can answer without a
// network.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Verified is a file to fetch and the digest it has to have.
type Verified struct {
	// URL is where it is, and SHA256 is what it has to hash to. An empty
	// digest is refused rather than skipped: this type exists to check.
	URL    string
	SHA256 string
	// Bytes is what the publisher says the size is, used for progress when
	// the server declares none - a redirect often arrives with no
	// Content-Length, and a job reading "0 of 0" on a 61 MB download reads as
	// a stall.
	Bytes int64
	// Name is what to call this in an error, in the words the operator saw it
	// offered under.
	Name string
	// Dir is where the part file is written. The caller moves it into place,
	// because where "in place" is differs per caller and only they know
	// whether what landed is what they wanted.
	Dir  string
	HTTP HTTPDoer
}

// To streams the file into Dir, hashing as it goes, and hands back the
// temporary file's path.
//
// Streamed rather than held in memory: these are tens to hundreds of megabytes,
// and the machine is usually about to spend its memory on something else.
// Nothing is left behind on a failure, including a digest mismatch: a truncated
// or substituted file that stays on the disk is one a later run can mistake for
// a finished download.
func (v Verified) To(ctx context.Context, progress func(done, total int64)) (string, error) {
	if v.SHA256 == "" {
		return "", fmt.Errorf("resource: refusing to download %s with no digest "+
			"to check it against", v.Name)
	}
	resp, err := v.get(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	f, err := os.CreateTemp(v.Dir, "."+safeName(v.Name)+"-*.part")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	total := v.Bytes
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
		return "", fmt.Errorf("resource: reading %s: %w", v.Name, err)
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != v.SHA256 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("resource: %s is not the file this was written "+
			"against (sha256 %s, expected %s) - the download was truncated or the "+
			"asset was replaced, and either way it is not being unpacked",
			v.Name, got, v.SHA256)
	}
	return tmp, nil
}

func (v Verified) get(ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.URL, nil)
	if err != nil {
		return nil, err
	}
	client := v.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resource: fetching %s: %w", v.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("resource: %s answered %s for %s",
			hostOf(v.URL), resp.Status, v.Name)
	}
	return resp, nil
}

// safeName keeps a part file's name from carrying a path separator into the
// directory it is created in. The names are ours, but they arrive from a
// release feed, and a release feed is somebody else's data.
func safeName(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "download"
	}
	return string(out)
}

// hostOf names where an answer came from, without printing a URL that may
// carry a token on the end of it.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
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
