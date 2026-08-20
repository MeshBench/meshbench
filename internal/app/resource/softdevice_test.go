package resource

import (
	"archive/zip"
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
)

// fakeSoftDevice builds an archive shaped like Nordic's, and points the
// catalogue at it. The real one is 390 kB from Nordic's CDN; a test that
// fetched it would be measuring the weather.
func fakeSoftDevice(t *testing.T, hexBody string) (*SoftDevice, func()) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"s140_nrf52_6.1.1_API/README":            "not wanted",
		"s140_nrf52_6.1.1_softdevice.hex":        hexBody,
		"s140_nrf52_6.1.1_license-agreement.txt": "Nordic's terms, in full.",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := buf.Bytes()

	saved := softDevices
	sum := sha256.Sum256(archive)
	hsum := sha256.Sum256([]byte(hexBody))
	softDevices = []softDeviceRelease{{
		Name: "s140", Version: "6.1.1", URL: "https://nordic.test/s140.zip",
		SHA256:      hex.EncodeToString(sum[:]),
		HexName:     "s140_nrf52_6.1.1_softdevice.hex",
		LicenceName: "s140_nrf52_6.1.1_license-agreement.txt",
		HexSHA256:   hex.EncodeToString(hsum[:]),
		AppBaseHex:  0x26000, Bytes: int64(len(archive)),
	}}
	sd := &SoftDevice{CacheDir: t.TempDir(), HTTP: roundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK",
			Body:          io.NopCloser(bytes.NewReader(archive)),
			ContentLength: int64(len(archive)),
		}, nil
	})}
	return sd, func() { softDevices = saved }
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) Do(r *http.Request) (*http.Response, error) { return f(r) }

// The whole point: a person who has never opened a terminal ends up with the
// image and the terms beside it.
func TestFetchingASoftDeviceKeepsTheImageAndTheLicence(t *testing.T) {
	sd, restore := fakeSoftDevice(t, ":020000040000FA\n:00000001FF\n")
	defer restore()

	rows, err := sd.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].State != Available {
		t.Fatalf("before fetching, the row reads %+v", rows[0])
	}
	if rows[0].Auto {
		t.Fatal("a licensed binary must never be fetched without being asked for")
	}

	var lastDone int64
	if err := sd.Fetch(context.Background(), "s140", "6.1.1",
		func(done, _ int64) { lastDone = done }); err != nil {
		t.Fatal(err)
	}
	if lastDone == 0 {
		t.Error("the download reported no progress at all")
	}

	if p := sd.HexPath("s140", "6.1.1"); p == "" {
		t.Fatal("the image is not where the emulator will look for it")
	} else if b, _ := os.ReadFile(p); !strings.HasPrefix(string(b), ":") {
		t.Fatal("what landed is not Intel HEX")
	}
	if lic := sd.Licence("s140", "6.1.1"); !strings.Contains(lic, "Nordic") {
		t.Fatalf("the licence did not come with it: %q", lic)
	}
	// Nothing else from the archive is kept: the API headers are not ours to
	// scatter over somebody's cache.
	entries, _ := os.ReadDir(filepath.Join(sd.CacheDir, "softdevice", "s140-6.1.1"))
	if len(entries) != 2 {
		t.Fatalf("%d files unpacked, want the image and the licence", len(entries))
	}

	rows, _ = sd.List(context.Background())
	if rows[0].State != OnDisk || rows[0].Estimated {
		t.Fatalf("after fetching, the row still reads %+v", rows[0])
	}
	if err := sd.Remove(context.Background(), rows[0]); err != nil {
		t.Fatal(err)
	}
	if sd.HexPath("s140", "6.1.1") != "" {
		t.Fatal("remove left the image behind")
	}
}

// Bytes that are not the ones this was written against are refused before
// anything is unpacked. A licensed binary is exactly the wrong thing to take
// on trust from a URL.
func TestASoftDeviceThatIsNotTheExpectedBytesIsRefused(t *testing.T) {
	sd, restore := fakeSoftDevice(t, ":020000040000FA\n")
	defer restore()
	softDevices[0].SHA256 = strings.Repeat("00", 32)

	err := sd.Fetch(context.Background(), "s140", "6.1.1", nil)
	if err == nil {
		t.Fatal("an archive with the wrong digest was unpacked anyway")
	}
	if !strings.Contains(err.Error(), "refusing to unpack") {
		t.Fatalf("the refusal did not say what it was doing: %v", err)
	}
	if sd.HexPath("s140", "6.1.1") != "" {
		t.Fatal("a refused archive still left a file behind")
	}
}

// A scenario that needs one says so, because "available" reads as optional.
func TestASoftDeviceANodeNeedsSaysSo(t *testing.T) {
	sd, restore := fakeSoftDevice(t, ":00000001FF\n")
	defer restore()
	sd.Needed = 2

	rows, _ := sd.List(context.Background())
	if rows[0].State != Needed {
		t.Fatalf("with 2 nodes waiting on it the row reads %q", rows[0].State)
	}
	if !strings.Contains(rows[0].Why, "cannot boot") {
		t.Fatalf("the row does not say what is stuck: %q", rows[0].Why)
	}
}
