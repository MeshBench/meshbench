package updates

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/version"
)

// A feed where the newest thing is a pre-release. The redirect names it too,
// which is the hand-pointed-feed case the stable channel has to refuse.
func preReleaseFeed(t *testing.T) string {
	t.Helper()
	const tag = "v0.0.12-dev.2"
	// The artefact this platform would take, so the pre-release is a real
	// offer and not refused two lines later for publishing no build. The
	// same names updateverbs_test uses, which sits in the external package
	// where this file, needing ask(), cannot.
	asset := "meshbench-linux-x86_64.tar.gz"
	switch runtime.GOOS {
	case "windows":
		asset = "meshbench-0.2.0-windows-x86_64.zip"
	case "darwin":
		asset = "meshbench-0.2.0-macos-arm64.dmg"
	}
	rel := func(tag string, pre bool) string {
		p := "false"
		if pre {
			p = "true"
		}
		return `{"tag_name":"` + tag + `","html_url":"https://example.test/r/` + tag +
			`","published_at":"2026-09-07T00:00:00Z","draft":false,"prerelease":` + p +
			`,"assets":[{"name":"` + asset + `","browser_download_url":"https://example.test/` + asset + `","size":1}]}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/releases/latest":
			w.Header().Set("Location", "https://example.test/releases/tag/"+tag)
			w.WriteHeader(http.StatusFound)
		case r.URL.Path == "/releases":
			_, _ = w.Write([]byte("[" + rel(tag, true) + "," + rel("v0.0.11", false) + "]"))
		case strings.HasPrefix(r.URL.Path, "/releases/tags/"):
			t := strings.TrimPrefix(r.URL.Path, "/releases/tags/")
			_, _ = w.Write([]byte(rel(t, t == tag)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// The development channel exists to be offered pre-releases. detail() used to
// refuse every pre-release it was handed, so on that channel the check found
// the build and then declined it.
func TestTheDevelopmentChannelIsOfferedAPreRelease(t *testing.T) {
	was := version.Version
	defer func() { version.Version = was }()
	version.Version = "v0.0.11"
	feed := preReleaseFeed(t)

	u := ask(context.Background(), feed, "development")
	if u.Err != "" {
		t.Fatalf("the check errored: %s", u.Err)
	}
	if u.Latest != "0.0.12-dev.2" {
		t.Errorf("the development channel found %q, want 0.0.12-dev.2", u.Latest)
	}
	if u.Why != "" {
		t.Errorf("the development channel declined the build it found: %s", u.Why)
	}
}

// The stable channel must not be offered one, even from a feed pointed at it
// by hand whose redirect names a pre-release.
func TestTheStableChannelRefusesAPreRelease(t *testing.T) {
	was := version.Version
	defer func() { version.Version = was }()
	version.Version = "v0.0.11"
	feed := preReleaseFeed(t)

	u := ask(context.Background(), feed, "stable")
	if u.Latest != "" {
		t.Errorf("the stable channel was offered %q", u.Latest)
	}
	if !strings.Contains(u.Why, "pre-release") || !strings.Contains(u.Why, "stable channel") {
		t.Errorf("the refusal does not say why: %q", u.Why)
	}
}
