package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
)

// The real download, against the real releases, into a temporary directory.
//
// Off by default because it is over 100 MB across the network and this suite
// has to run offline. It is the only thing that proves the catalogue: the
// digests pinned there are the whole verification, and a release asset
// replaced under its own tag would be caught by nothing else in the tree.
//
//	MESHBENCH_LIVE=1 go test ./internal/app/resource -run TestFetchingTheRealToolchain -v
func TestFetchingTheRealToolchain(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1 to fetch the real toolchain")
	}
	dir := t.TempDir()
	tc := &Toolchain{Dir: dir}
	for _, rel := range toolReleases {
		if _, ok := rel.asset(); !ok {
			t.Logf("%s: %s", rel.Name, rel.unavailableBecause())
			continue
		}
		var last int64
		err := tc.Fetch(context.Background(), rel.Name, rel.Version,
			func(done, _ int64) { last = done })
		if err != nil {
			t.Errorf("%s: %v", rel.Name, err)
			continue
		}
		row := tc.row(rel)
		if row.State != OnDisk {
			t.Errorf("%s fetched and is not on disk", rel.Name)
		}
		t.Logf("%s %s: %d bytes downloaded, %d on disk at %s",
			rel.Name, rel.Version, last, row.Bytes, row.Path)
	}
	t.Logf("on %s/%s", runtime.GOOS, runtime.GOARCH)
}

// Every asset in the catalogue, on every platform, against what is published.
//
// TestFetchingTheRealToolchain only exercises the platform it runs on, so a
// wrong digest or a renamed asset for another platform reaches somebody else's
// first run rather than CI. These pins are hand-copied from release pages, and
// a hand-copied digest is exactly the thing that is right until it is not.
//
// Downloads rather than HEADs, because a digest is the point: a URL that
// resolves and serves different bytes is the failure the pins exist to stop.
// About 200 MB, which is why this is off by default.
//
//	MESHBENCH_LIVE=1 go test ./internal/app/resource -run TestEveryPinnedAsset -v
func TestEveryPinnedAssetIsWhatTheCatalogueSaysItIs(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1 to check every pin against its release")
	}
	for _, rel := range toolReleases {
		for platform, a := range rel.Assets {
			t.Run(rel.Name+"/"+platform, func(t *testing.T) {
				req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, a.URL, nil)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("fetching %s: %v", a.URL, err)
				}
				defer func() { _ = resp.Body.Close() }()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("%s answered %s", a.URL, resp.Status)
				}
				sum := sha256.New()
				n, err := io.Copy(sum, resp.Body)
				if err != nil {
					t.Fatalf("reading %s: %v", a.URL, err)
				}
				if n != a.Bytes {
					t.Errorf("%s is %d bytes, the catalogue says %d", a.URL, n, a.Bytes)
				}
				if got := hex.EncodeToString(sum.Sum(nil)); got != a.SHA256 {
					t.Errorf("%s hashes to %s, the catalogue says %s", a.URL, got, a.SHA256)
				}
			})
		}
	}
}
