package resource

import (
	"context"
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
