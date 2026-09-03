package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The real download, from Nordic, into a temporary directory.
//
// Off by default for the same reason as the toolchain's: this suite has to run
// offline. It is the only thing that proves these rows, and the two digests in
// each are the whole verification - Nordic serves these over a URL pattern
// rather than a versioned release, so an archive replaced in place would be
// caught by nothing else here.
//
// Worth having beyond that. A board's image says only which SoftDevice family
// it was linked above, so a family with no fetchable release is a board that
// cannot boot at all: Xiao_nrf52's published image starts at 0x27000, and until
// v7 was added to softDevices its whole capability report read "untested" while
// its row in the board matrix carried a measured cross. A row nobody can fetch
// fails silently, and this is what says so out loud.
//
//	MESHBENCH_LIVE=1 go test ./internal/app/resource -run TestFetchingTheRealSoftDevices -v
func TestFetchingTheRealSoftDevices(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1 to fetch the real SoftDevices")
	}
	for _, rel := range softDevices {
		t.Run(rel.Name+"-"+rel.Version, func(t *testing.T) {
			dir := t.TempDir()
			sd := &SoftDevice{CacheDir: dir}
			var last int64
			if err := sd.Fetch(context.Background(), rel.Name, rel.Version,
				func(done, _ int64) { last = done }); err != nil {
				t.Fatalf("fetching: %v", err)
			}
			// Both files, because a SoftDevice whose licence did not arrive is
			// one nobody may read the terms of, and Fetch takes them together.
			hex := sd.HexPath(rel.Name, rel.Version)
			if hex == "" {
				t.Fatal("fetched and the hex is not on disk")
			}
			licence := filepath.Join(filepath.Dir(hex), rel.LicenceName)
			if _, err := os.Stat(licence); err != nil {
				t.Errorf("the licence did not arrive: %v", err)
			}
			t.Logf("%s %s: %d bytes downloaded, hex at %s", rel.Name, rel.Version, last, hex)
		})
	}
}
