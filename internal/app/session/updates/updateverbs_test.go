package updates_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	_ "github.com/MeshBench/meshbench/internal/app/session/updates"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/app/version"
)

// The verbs, driven through the store as a client drives them, against a
// release feed that is a stand-in for the published one. The stand-in is the
// point: the repository is private, there is no published release to check
// against, and the only part of this that cannot be exercised is reaching that
// page.

const asset = "meshbench-linux-x86_64.tar.gz"

// feedFor serves a release feed, the artefact and its checksum file, and hands
// back the URL to point a session at.
func feedFor(t *testing.T, tag string, body []byte) string {
	t.Helper()
	sum := sha256.Sum256(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Built from the request's own host rather than from the server value,
		// which is not assigned until the server is already listening.
		base := "http://" + r.Host
		switch strings.TrimPrefix(r.URL.Path, "/") {
		// The cheap route: what the releases page answers, a redirect naming
		// the newest tag and nothing else.
		case "releases/latest":
			w.Header().Set("Location",
				"https://example.test/MeshBench/meshbench/releases/tag/"+tag)
			w.WriteHeader(http.StatusFound)
		// The API route, asked only once a newer release has been found.
		case "releases/tags/" + tag:
			_, _ = w.Write([]byte(`{"tag_name":"` + tag + `",` +
				`"html_url":"https://example.test/releases/` + tag + `",` +
				`"published_at":"` + time.Now().Add(-72*time.Hour).UTC().
				Format(time.RFC3339) + `",` +
				`"assets":[` +
				`{"name":"` + asset + `","browser_download_url":"` + base + "/" + asset +
				`","size":` + strconv.Itoa(len(body)) + `},` +
				`{"name":"SHA256SUMS","browser_download_url":"` + base + `/SHA256SUMS"}]}`))
		case asset:
			_, _ = w.Write(body)
		case "SHA256SUMS":
			_, _ = w.Write([]byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// released stamps this process as a release for the duration of a test, which
// is the only way to exercise any of this: a test binary is a working copy, and
// a working copy is deliberately never told it is out of date.
func released(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func running(t *testing.T, feed string) (*state.Store, context.Context) {
	t.Helper()
	st, _ := session.Boot(session.Options{
		NoPrefs: true, Headless: true, UpdateFeed: feed})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go st.Run(ctx)
	return st, ctx
}

// waitForCheck polls the status verb, because a check is a network call and
// answers on a worker rather than on the store's goroutine.
func waitForCheck(t *testing.T, st *state.Store, ctx context.Context) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := st.Do(ctx, "update.status", nil)
		if err != nil {
			t.Fatalf("update.status: %v", err)
		}
		m, _ := got.(map[string]any)
		if m["checked"] != "" {
			return m
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the check never answered")
	return nil
}

func TestACheckFindsANewerReleaseAndPricesItBeforeSpending(t *testing.T) {
	released(t, "v0.1.0")
	st, ctx := running(t, feedFor(t, "v0.2.0", []byte("a release, near enough")))

	if _, err := st.Do(ctx, "update.check", nil); err != nil {
		t.Fatalf("update.check: %v", err)
	}
	m := waitForCheck(t, st, ctx)
	if m["latest"] != "0.2.0" {
		t.Errorf("latest is %v, want 0.2.0", m["latest"])
	}
	if m["newer"] != true {
		t.Error("0.2.0 was not read as newer than 0.1.0")
	}
	if m["bytes"].(int64) <= 0 {
		t.Error("the size was not reported, and it has to be said before it is spent")
	}
	if m["feed"] == "" {
		t.Error("a check pointed at a stand-in feed did not say so")
	}
}

// A check that could not reach the release page has not said this build is
// current. Three answers, not two: there is something newer, there is nothing
// newer, and I could not find out - and the third has to be visible as itself
// or the updater is lying about the one thing it exists to say.
func TestACheckThatCouldNotAnswerIsNotReportedAsUpToDate(t *testing.T) {
	released(t, "v0.1.0")
	refusing := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
		}))
	t.Cleanup(refusing.Close)
	st, ctx := running(t, refusing.URL)

	if _, err := st.Do(ctx, "update.check", nil); err != nil {
		t.Fatalf("update.check: %v", err)
	}
	m := waitForCheck(t, st, ctx)
	if m["error"] == "" {
		t.Fatal("a refused check left no reason, so it reads as up to date")
	}
	if m["newer"] != false || m["available"] != false {
		t.Error("a refused check claimed to know whether a release is newer")
	}
	if m["latest"] != "" {
		t.Errorf("a refused check named %v as the newest release", m["latest"])
	}
	if _, err := st.Do(ctx, "update.download", nil); err == nil {
		t.Error("a download started off a check that never answered")
	}
}

// The rule the whole feature turns on.
func TestAWorkingCopyIsNeverToldItIsOutOfDate(t *testing.T) {
	st, ctx := running(t, feedFor(t, "v9.9.9", []byte("x")))

	got, err := st.Do(ctx, "update.check", nil)
	if err != nil {
		t.Fatalf("update.check: %v", err)
	}
	m, _ := got.(map[string]any)
	if m["newer"] == true {
		t.Fatal("a working copy was told a release is newer than it")
	}
	if why, _ := m["why"].(string); !strings.Contains(why, "release") {
		t.Errorf("the answer is %q, want it to say this build is not a release", why)
	}
	if _, err := st.Do(ctx, "update.download", nil); err == nil {
		t.Error("a working copy was allowed to download an update over itself")
	}
}

// Consent is three states and starts at the third: nothing has been asked, and
// nothing has been spent.
func TestUpdateChecksAreOffUntilSomebodySaysOtherwise(t *testing.T) {
	released(t, "v0.1.0")
	st, ctx := running(t, "")

	got, err := st.Do(ctx, "update.status", nil)
	if err != nil {
		t.Fatalf("update.status: %v", err)
	}
	m, _ := got.(map[string]any)
	if m["allowed"] != false || m["asked"] != false {
		t.Errorf("a fresh session reports allowed=%v asked=%v, want both false",
			m["allowed"], m["asked"])
	}
	if m["checked"] != "" {
		t.Error("something checked without being asked to")
	}
}

// Nothing to download is four different answers, and the refusal has to say
// which: only two of them are worth doing anything about.
func TestDownloadingWithNothingToTakeSaysWhy(t *testing.T) {
	released(t, "v0.1.0")
	st, ctx := running(t, "")

	_, err := st.Do(ctx, "update.download", nil)
	if err == nil {
		t.Fatal("a download started with nothing to download")
	}
	if !strings.Contains(err.Error(), "update.check") {
		t.Errorf("the refusal is %q, want it to say what would answer the "+
			"question first", err)
	}
}

// The whole path: check, download, verify, and land beside the build rather
// than on top of it.
func TestADownloadLandsBesideTheBuildAndReplacesNothing(t *testing.T) {
	released(t, "v0.1.0")
	body := []byte("the release, as a file")
	st, ctx := running(t, feedFor(t, "v0.2.0", body))
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	if _, err := st.Do(ctx, "update.check", nil); err != nil {
		t.Fatalf("update.check: %v", err)
	}
	waitForCheck(t, st, ctx)
	if _, err := st.Do(ctx, "update.download", nil); err != nil {
		t.Fatalf("update.download: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := st.Do(ctx, "update.status", nil)
		m, _ := got.(map[string]any)
		if staged, _ := m["staged"].(string); staged != "" {
			if !strings.Contains(staged, "updates") {
				t.Errorf("the download landed at %q, want it under the update "+
					"cache and nowhere near the binary", staged)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the download never finished, or never said where it went")
}
