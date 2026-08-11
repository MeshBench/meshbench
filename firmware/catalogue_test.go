package firmware_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/firmware"
)

type catServer struct {
	byPath map[string][]byte
	status int
	hits   map[string]int
}

func (s *catServer) Do(req *http.Request) (*http.Response, error) {
	if s.hits == nil {
		s.hits = map[string]int{}
	}
	s.hits[req.URL.Path]++
	body, ok := s.byPath[req.URL.Path]
	code := s.status
	if code == 0 {
		if ok {
			code = 200
		} else {
			code = 404
		}
	}
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, nil
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Real MeshCore release asset names, verbatim. Parsed rather than matched
// against a hard-coded board list, because the board list changes every release
// and a hard-coded one silently hides new hardware.
func TestParsesRealAssetNames(t *testing.T) {
	image := []byte("firmware bytes")
	releases := `[{"tag_name":"repeater-v1.17.0","assets":[
		{"name":"RAK_4631_repeater-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/rak.uf2","size":14,"digest":"sha256:` + digest(image) + `"},
		{"name":"Heltec_t114_companion_radio_ble-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/heltec-ble.uf2","size":14},
		{"name":"Heltec_t114_companion_radio_usb-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/heltec-usb.uf2","size":14},
		{"name":"Xiao_S3_WIO_room_server-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/xiao.uf2","size":14},
		{"name":"RAK_3112_repeater-v1.17.0-727fc05-merged.bin","browser_download_url":"http://fw.test/merged.bin","size":14},
		{"name":"Source code (zip)","browser_download_url":"http://fw.test/src.zip","size":9}
	]}]`

	srv := &catServer{byPath: map[string][]byte{"/releases": []byte(releases)}}
	c := &firmware.Catalogue{ReleasesURL: "http://fw.test/releases", CacheDir: t.TempDir(), HTTP: srv}

	imgs, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]firmware.Image{}
	for _, i := range imgs {
		got[i.Name()] = i
	}
	for _, want := range []string{
		"RAK_4631/repeater",
		"Heltec_t114/companion/ble",
		"Heltec_t114/companion/usb",
		"Xiao_S3_WIO/room-server",
	} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %q; got %v", want, keys(got))
		}
	}
	// The zip is not a firmware image and must not be offered in a picker.
	for name := range got {
		if strings.Contains(name, "Source") {
			t.Errorf("a source archive was offered as firmware: %q", name)
		}
	}

	if b := firmware.Boards(imgs); len(b) < 4 {
		t.Errorf("boards: %v", b)
	}
}

// A wrong image is not a crash. It boots, or does not, on hardware it was never
// built for, and the result is indistinguishable from a firmware bug.
func TestChecksumIsEnforced(t *testing.T) {
	image := []byte("the real firmware")
	releases := `[{"tag_name":"repeater-v1.17.0","assets":[
		{"name":"RAK_4631_repeater-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/rak.uf2","size":17,"digest":"sha256:` + digest(image) + `"}
	]}]`
	srv := &catServer{byPath: map[string][]byte{
		"/releases": []byte(releases),
		"/rak.uf2":  []byte("a completely different firmware"),
	}}
	dir := t.TempDir()
	c := &firmware.Catalogue{ReleasesURL: "http://fw.test/releases", CacheDir: dir, HTTP: srv}

	imgs, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !imgs[0].Verified() {
		t.Fatal("the published digest was not recorded")
	}
	_, err = c.Fetch(context.Background(), imgs[0])
	if err == nil {
		t.Fatal("an image with the wrong contents was accepted")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error should name the problem: %v", err)
	}
	// And nothing should have been written.
	matches, _ := filepath.Glob(filepath.Join(dir, "*", "*"))
	if len(matches) != 0 {
		t.Errorf("a failed download left %d files behind", len(matches))
	}
}

func TestFetchCachesAndReuses(t *testing.T) {
	image := []byte("the real firmware")
	releases := `[{"tag_name":"repeater-v1.17.0","assets":[
		{"name":"RAK_4631_repeater-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/rak.uf2","size":17,"digest":"sha256:` + digest(image) + `"}
	]}]`
	srv := &catServer{byPath: map[string][]byte{
		"/releases": []byte(releases),
		"/rak.uf2":  image,
	}}
	c := &firmware.Catalogue{ReleasesURL: "http://fw.test/releases", CacheDir: t.TempDir(), HTTP: srv}

	imgs, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.Fetch(context.Background(), imgs[0])
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Fetch(context.Background(), imgs[0])
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("two fetches gave different paths: %s and %s", first, second)
	}
	if srv.hits["/rak.uf2"] != 1 {
		t.Errorf("downloaded %d times; the cache is not being used", srv.hits["/rak.uf2"])
	}

	b, err := os.ReadFile(first)
	if err != nil || !bytes.Equal(b, image) {
		t.Errorf("cached file does not match what was published")
	}
}

// A cached file that no longer matches its digest is corruption or a moved tag.
// Serving it is the one thing that must not happen.
func TestCorruptCacheIsRefetched(t *testing.T) {
	image := []byte("the real firmware")
	releases := `[{"tag_name":"repeater-v1.17.0","assets":[
		{"name":"RAK_4631_repeater-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/rak.uf2","size":17,"digest":"sha256:` + digest(image) + `"}
	]}]`
	srv := &catServer{byPath: map[string][]byte{
		"/releases": []byte(releases),
		"/rak.uf2":  image,
	}}
	dir := t.TempDir()
	c := &firmware.Catalogue{ReleasesURL: "http://fw.test/releases", CacheDir: dir, HTTP: srv}
	imgs, _ := c.List(context.Background())

	path, err := c.Fetch(context.Background(), imgs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}

	again, err := c.Fetch(context.Background(), imgs[0])
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(again)
	if !bytes.Equal(b, image) {
		t.Error("a corrupted cache entry was served")
	}
	if srv.hits["/rak.uf2"] != 2 {
		t.Errorf("corrupted entry was not refetched (%d downloads)", srv.hits["/rak.uf2"])
	}
}

// Importing your own build is supported on purpose: the catalogue is the
// default, not the only way. An imported image reports as unverified, which is
// accurate rather than pessimistic.
func TestImportOwnBuild(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "my-debug-build.uf2")
	if err := os.WriteFile(src, []byte("my own firmware"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &firmware.Catalogue{CacheDir: filepath.Join(dir, "cache")}
	img, err := c.Import(src, "RAK_4631", "repeater")
	if err != nil {
		t.Fatal(err)
	}
	if img.Verified() {
		t.Error("an imported image claimed a published digest")
	}
	if img.Board != "RAK_4631" || img.Version != "imported" {
		t.Errorf("unexpected image: %+v", img)
	}
	if _, err := os.Stat(img.URL); err != nil {
		t.Errorf("imported file was not stored: %v", err)
	}

	// Nothing in a bare .bin says which board it is for, so it has to be told.
	if _, err := c.Import(src, "", "repeater"); err == nil {
		t.Error("an import with no board was accepted")
	}
	if _, err := c.Import(filepath.Join(dir, "notes.txt"), "RAK_4631", "repeater"); err == nil {
		t.Error("a text file was accepted as firmware")
	}
}

// A release without a digest is still usable. Refusing it would make the
// default path fail on exactly the older releases someone is most likely to
// want for comparison.
func TestUnverifiedImagesStillRun(t *testing.T) {
	image := []byte("older firmware, no digest published")
	releases := `[{"tag_name":"repeater-v1.10.0","assets":[
		{"name":"RAK_4631_repeater-v1.10.0-abc1234.uf2","browser_download_url":"http://fw.test/old.uf2","size":35}
	]}]`
	srv := &catServer{byPath: map[string][]byte{
		"/releases": []byte(releases),
		"/old.uf2":  image,
	}}
	c := &firmware.Catalogue{ReleasesURL: "http://fw.test/releases", CacheDir: t.TempDir(), HTTP: srv}

	imgs, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if imgs[0].Verified() {
		t.Error("an image with no published digest reported as verified")
	}
	if _, err := c.Fetch(context.Background(), imgs[0]); err != nil {
		t.Errorf("an unverified image would not download: %v", err)
	}
}

func TestOfflineListsWhatIsCached(t *testing.T) {
	image := []byte("the real firmware")
	releases := `[{"tag_name":"repeater-v1.17.0","assets":[
		{"name":"RAK_4631_repeater-v1.17.0-727fc05.uf2","browser_download_url":"http://fw.test/rak.uf2","size":17,"digest":"sha256:` + digest(image) + `"}
	]}]`
	srv := &catServer{byPath: map[string][]byte{
		"/releases": []byte(releases),
		"/rak.uf2":  image,
	}}
	dir := t.TempDir()
	c := &firmware.Catalogue{ReleasesURL: "http://fw.test/releases", CacheDir: dir, HTTP: srv}
	imgs, _ := c.List(context.Background())
	if _, err := c.Fetch(context.Background(), imgs[0]); err != nil {
		t.Fatal(err)
	}

	c.Offline = true
	cached, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 1 || cached[0].Board != "RAK_4631" {
		t.Errorf("offline catalogue: %+v", cached)
	}

	// And an image that was never downloaded must fail rather than block.
	missing := firmware.Image{Board: "Other", Role: "repeater", Version: "v1", URL: "http://fw.test/never.uf2"}
	if _, err := c.Fetch(context.Background(), missing); err == nil {
		t.Error("an undownloaded image was served while offline")
	}
}

func keys(m map[string]firmware.Image) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
