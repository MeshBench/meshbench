package update_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/update"
)

// A stand-in release: the artefact, and the checksum file the pipeline
// publishes beside it. Everything about a download except reaching the real
// release page is exercised here, because the real release page does not exist
// yet - the repository is private - and a download nobody checked is the thing
// this code exists to refuse.

const assetName = "meshbench-linux-x86_64.tar.gz"

// fakeRelease serves an asset and a SHA256SUMS, and hands back the release as
// the feed would describe it. sums is what the checksum file claims, so a test
// can publish a digest that does not match what it serves.
func fakeRelease(t *testing.T, body []byte, sums string) update.Release {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/") {
		case assetName:
			_, _ = w.Write(body)
		case update.SumsAsset:
			_, _ = w.Write([]byte(sums))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return update.Release{
		Tag: "v0.2.0", Version: "0.2.0",
		Notes: "https://github.com/MeshBench/meshbench/releases/tag/v0.2.0",
		Assets: []update.Asset{
			{Name: assetName, URL: srv.URL + "/" + assetName, Bytes: int64(len(body))},
			{Name: update.SumsAsset, URL: srv.URL + "/" + update.SumsAsset},
		},
	}
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func stageInto(t *testing.T) update.Stage {
	t.Helper()
	// Redirected, because this is not github.com: a test feed is exactly the
	// case the host check exists to allow deliberately and refuse silently.
	return update.Stage{Dir: t.TempDir(), Redirected: true}
}

func TestAVerifiedDownloadLandsBesideTheBuild(t *testing.T) {
	body := []byte("not really a tarball, but it hashes like one")
	rel := fakeRelease(t, body, digestOf(body)+"  "+assetName+"\n")
	st := stageInto(t)

	var last int64
	path, err := st.Get(context.Background(), rel, rel.Assets[0],
		func(done, _ int64) { last = done })
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if filepath.Base(path) != assetName {
		t.Errorf("landed as %q, want the published name", filepath.Base(path))
	}
	if filepath.Base(filepath.Dir(path)) != "v0.2.0" {
		t.Errorf("landed in %q, want a directory named for the release",
			filepath.Dir(path))
	}
	got, err := os.ReadFile(path) //nolint:gosec // a file this test just wrote
	if err != nil || string(got) != string(body) {
		t.Fatalf("what landed is not what was served: %v", err)
	}
	if last == 0 {
		t.Error("no progress was reported, so a long download would show nothing")
	}
}

// The refusal that matters. A file whose digest is not the published one is not
// kept: a truncated or substituted download left on the disk is one a later run
// can mistake for a finished one.
func TestADownloadThatDoesNotMatchIsRefusedAndNotKept(t *testing.T) {
	body := []byte("what actually arrived")
	rel := fakeRelease(t, body, digestOf([]byte("what was promised"))+"  "+assetName+"\n")
	st := stageInto(t)

	_, err := st.Get(context.Background(), rel, rel.Assets[0], nil)
	if err == nil {
		t.Fatal("a download whose digest did not match was accepted")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("the refusal is %q, want it to name the digest", err)
	}
	if left, _ := filepath.Glob(filepath.Join(st.Dir, "v0.2.0", "*")); len(left) > 0 {
		t.Errorf("a refused download left %v behind, which a later run could "+
			"mistake for a finished one", left)
	}
}

// A release with no checksum file cannot be checked, so nothing is taken from
// it. Refused rather than downloaded anyway: verified before it is offered is
// the rule, and a release that cannot be verified has to fail it loudly.
func TestAReleaseWithoutChecksumsIsNotDownloaded(t *testing.T) {
	body := []byte("anything")
	rel := fakeRelease(t, body, "")
	rel.Assets = rel.Assets[:1]
	st := stageInto(t)

	_, err := st.Get(context.Background(), rel, rel.Assets[0], nil)
	if err == nil {
		t.Fatal("a release with no checksum file was downloaded from anyway")
	}
	if !strings.Contains(err.Error(), update.SumsAsset) {
		t.Errorf("the refusal is %q, want it to name what is missing", err)
	}
}

// A checksum file that does not list this asset is the same problem wearing a
// different hat.
func TestAnAssetMissingFromTheChecksumsIsNotDownloaded(t *testing.T) {
	body := []byte("anything")
	rel := fakeRelease(t, body, digestOf(body)+"  something-else.zip\n")
	st := stageInto(t)

	if _, err := st.Get(context.Background(), rel, rel.Assets[0], nil); err == nil {
		t.Fatal("an asset with no published digest was downloaded")
	}
}

// The digest and the file come from the same release, so what the digest proves
// is that the download arrived intact. What says the release is ours is the
// connection, which is why an asset served from anywhere else is refused before
// a byte of it is read.
func TestAnAssetServedFromSomewhereElseIsRefused(t *testing.T) {
	body := []byte("anything")
	rel := fakeRelease(t, body, digestOf(body)+"  "+assetName+"\n")
	st := update.Stage{Dir: t.TempDir()}

	for _, u := range []string{
		"http://github.com/MeshBench/meshbench/x.tar.gz",
		"https://example.invalid/MeshBench/meshbench/x.tar.gz",
	} {
		a := rel.Assets[0]
		a.URL = u
		_, err := st.Get(context.Background(), rel, a, nil)
		if err == nil {
			t.Errorf("%s was accepted as a place a release is served from", u)
			continue
		}
		if !strings.Contains(err.Error(), "refusing") {
			t.Errorf("the refusal for %s is %q, want it to say it is refusing", u, err)
		}
	}
}
