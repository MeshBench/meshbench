package update_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/update"
)

// Both routes, against a stand-in that answers the two shapes GitHub answers:
// a redirect naming the newest tag, and the API's release JSON for one tag.

// releaseJSON is the half of GitHub's release object this reads, spelled the
// way the API spells it.
const releaseJSON = `{
  "tag_name": "v0.2.0",
  "html_url": "https://github.com/MeshBench/meshbench/releases/tag/v0.2.0",
  "published_at": "2026-08-30T09:00:00Z",
  "draft": false,
  "prerelease": false,
  "assets": [
    {"name": "meshbench-linux-x86_64.tar.gz",
     "browser_download_url": "https://github.com/x/meshbench-linux-x86_64.tar.gz",
     "size": 44000000},
    {"name": "SHA256SUMS",
     "browser_download_url": "https://github.com/x/SHA256SUMS", "size": 400}
  ]
}`

// standIn serves the release page's redirect and the API's release object.
func standIn(t *testing.T, tag, detail string, detailStatus int,
	headers map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			if tag == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Location",
				"https://example.test/MeshBench/meshbench/releases/tag/"+tag)
			w.WriteHeader(http.StatusFound)
		case strings.Contains(r.URL.Path, "/releases/tags/"):
			for k, v := range headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(detailStatus)
			_, _ = w.Write([]byte(detail))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// The routine question is answered by a redirect: no API call, and so nothing
// spent from the 60-an-hour budget every unauthenticated caller on one address
// shares.
func TestTheNewestReleaseComesFromTheRedirect(t *testing.T) {
	c := update.Checker{Feed: standIn(t, "v0.2.0", releaseJSON, http.StatusOK, nil)}
	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if rel.Tag != "v0.2.0" || rel.Version != "0.2.0" {
		t.Errorf("tag %q version %q, want v0.2.0 and 0.2.0 - the tag carries the "+
			"v and every other artefact of a release does not", rel.Tag, rel.Version)
	}
	if len(rel.Assets) != 0 {
		t.Error("the cheap route claimed to know the assets; only the API does")
	}
}

// The API is asked once a release is found, because it is the only route that
// knows what is published and how big it is.
func TestTheAssetsAndTheDateComeFromTheAPI(t *testing.T) {
	c := update.Checker{Feed: standIn(t, "v0.2.0", releaseJSON, http.StatusOK, nil)}
	rel, err := c.Detail(context.Background(), "v0.2.0")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if rel.Published.IsZero() {
		t.Error("the release carries no date, so nothing can say how long ago it was")
	}
	if len(rel.Assets) != 2 {
		t.Fatalf("%d assets, want 2", len(rel.Assets))
	}
	if _, ok := rel.Find(update.SumsAsset); !ok {
		t.Error("the checksum file was not found among the assets, and it is the " +
			"only thing that can check what is downloaded")
	}
}

// The sharpest form of "silence is a bug". A rate limit is not an answer about
// this build, and reporting it as one would be the updater lying about the one
// thing it exists to say.
func TestARateLimitIsAnErrorThatNamesItself(t *testing.T) {
	reset := strconv.FormatInt(time.Now().Add(37*time.Minute).Unix(), 10)
	c := update.Checker{Feed: standIn(t, "v0.2.0", `{"message":"rate limited"}`,
		http.StatusForbidden, map[string]string{
			"X-RateLimit-Limit": "60", "X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset": reset,
		})}
	_, err := c.Detail(context.Background(), "v0.2.0")
	if err == nil {
		t.Fatal("a rate-limited answer was read as a release")
	}
	for _, want := range []string{"60 an hour", "resets at"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal is %q, want it to say %q", err, want)
		}
	}
}

// A releases page that does not redirect has not said there is nothing newer;
// it has said nothing at all, and the two must not be confused.
func TestAPageThatDoesNotRedirectIsNotAnAnswer(t *testing.T) {
	c := update.Checker{Feed: standIn(t, "", "", http.StatusOK, nil)}
	_, err := c.Latest(context.Background())
	if err == nil {
		t.Fatal("a 404 from the releases page was read as an answer")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("the error is %q, want it to say the answer is unknown", err)
	}
}

// A feed pointed somewhere else has to be visible as such: every answer from
// one says where it asked.
func TestARedirectedFeedKnowsItIsOne(t *testing.T) {
	plain := update.Checker{}
	if plain.Redirected() {
		t.Error("the default checker thinks it is redirected")
	}
	elsewhere := update.Checker{Feed: "http://127.0.0.1:1"}
	if !elsewhere.Redirected() {
		t.Error("a feed somewhere else does not report itself as one")
	}
}

// The comparison, including the case the whole feature turns on: a working copy
// has no release and is never behind.
func TestNewerOnlyComparesTwoRealReleases(t *testing.T) {
	cases := []struct {
		build, latest string
		want          bool
		why           string
	}{
		{"0.1.0", "0.2.0", true, "a higher minor is newer"},
		{"0.2.0", "0.2.0", false, "the same release is not newer than itself"},
		{"0.3.0", "0.2.0", false, "a lower release is not newer"},
		{"0.9.0", "0.10.0", true, "ten is after nine, and a string compare would say otherwise"},
		{"", "0.2.0", false, "a working copy is unreleased, not behind"},
		{"0.1.0", "", false, "a feed that named no version says nothing"},
		{"0.1.0", "v0.2.0", true, "the leading v is the tag's spelling, not a different number"},
		{"0.1.0", "0.2.0-rc1", false, "a release candidate is not a release"},
	}
	for _, c := range cases {
		if got := update.Newer(c.build, c.latest); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v: %s",
				c.build, c.latest, got, c.want, c.why)
		}
	}
}
