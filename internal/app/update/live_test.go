package update_test

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/update"
)

// The real release page, the real API and a real asset.
//
// Off by default because it goes to the network and this suite has to run
// offline; the stand-in tests beside it are what CI proves. What this proves is
// the half no stand-in can: that the redirect names the tag GitHub actually
// publishes, that the API describes the assets under the names AssetFor
// matches, and that a real artefact's bytes hash to what the real SHA256SUMS
// says they do.
//
//	MESHBENCH_LIVE=1 go test ./internal/app/update -run TestTheRealReleaseFeed -v
func TestTheRealReleaseFeed(t *testing.T) {
	if os.Getenv("MESHBENCH_LIVE") == "" {
		t.Skip("set MESHBENCH_LIVE=1 to ask the real release page")
	}
	ctx := context.Background()
	var c update.Checker

	newest, err := c.Latest(ctx)
	if err != nil {
		t.Fatalf("the redirect route: %v", err)
	}
	t.Logf("the releases page redirects to %s (%s), no API call spent",
		newest.Tag, newest.Notes)
	if newest.Version == "" {
		t.Errorf("%s is not a plain X.Y.Z tag, so nothing can compare it",
			newest.Tag)
	}

	rel, err := c.Detail(ctx, newest.Tag)
	if err != nil {
		t.Fatalf("the API route: %v", err)
	}
	t.Logf("%s published %s with %d assets", rel.Tag,
		rel.Published.Format("2006-01-02"), len(rel.Assets))
	for _, a := range rel.Assets {
		t.Logf("  %-40s %d", a.Name, a.Bytes)
	}
	if _, ok := rel.Find(update.SumsAsset); !ok {
		t.Fatalf("%s publishes no %s, so nothing could be verified", rel.Tag,
			update.SumsAsset)
	}

	// What this machine would take, for each bundle it could have been.
	for _, art := range []update.Artefact{
		update.Tarball, update.AppImage, update.Bundle, update.Zip, update.Loose,
	} {
		goos, goarch := goosFor(art)
		a, why := update.AssetFor(rel, art, goos, goarch)
		if why != "" {
			t.Errorf("%s on %s/%s: %s", art, goos, goarch, why)
			continue
		}
		t.Logf("%s on %s/%s takes %s (%d bytes)", art, goos, goarch, a.Name, a.Bytes)
	}

	// One real download, digest checked against the real checksum file. The
	// source archive rather than a build: it is seven megabytes rather than a
	// hundred, and it is verified by exactly the same path.
	src, ok := findSource(rel)
	if !ok {
		t.Skip("this release publishes no source archive to check the path with")
	}
	dir := t.TempDir()
	st := update.Stage{Dir: dir}
	path, err := st.Get(ctx, rel, src, nil)
	if err != nil {
		t.Fatalf("downloading %s: %v", src.Name, err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("what landed cannot be read: %v", err)
	}
	t.Logf("downloaded and verified %s: %d bytes at %s", src.Name, fi.Size(), path)
	if fi.Size() != src.Bytes {
		t.Errorf("%d bytes landed, the release says %d", fi.Size(), src.Bytes)
	}
	t.Logf("this machine is %s/%s and reads itself as a %s",
		runtime.GOOS, runtime.GOARCH, update.This())
}

// goosFor is the machine each bundle only ever runs on, so one run can check
// every platform's asset choice against the real names.
func goosFor(a update.Artefact) (goos, goarch string) {
	switch a {
	case update.Bundle:
		return "darwin", "arm64"
	case update.Zip:
		return "windows", "amd64"
	default:
		return "linux", "amd64"
	}
}

func findSource(rel update.Release) (update.Asset, bool) {
	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, "source.tar.gz") {
			return a, true
		}
	}
	return update.Asset{}, false
}
