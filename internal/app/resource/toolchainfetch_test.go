package resource

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

// A fetch is a download plus three refusals, and the refusals are the part
// worth testing: a wrong digest, a build for another architecture, and an
// archive that tries to write outside the directory it was unpacked into. All
// three arrive looking exactly like a good download.

// stubHTTP answers one URL with fixed bytes.
type stubHTTP struct {
	body   []byte
	status int
}

func (s stubHTTP) Do(req *http.Request) (*http.Response, error) {
	code := s.status
	if code == 0 {
		code = http.StatusOK
	}
	return &http.Response{
		StatusCode:    code,
		Status:        http.StatusText(code),
		Body:          io.NopCloser(bytes.NewReader(s.body)),
		ContentLength: int64(len(s.body)),
		Request:       req,
	}, nil
}

// writeFake writes something with the right executable header and the given
// size, which is all the verification step reads.
func writeFake(t *testing.T, path string, want execFormat, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fakeExec(want, size), 0o755); err != nil { //nolint:gosec // a stand-in emulator in a temporary directory
		t.Fatal(err)
	}
}

func fakeExec(want execFormat, size int) []byte {
	b := make([]byte, size)
	switch want {
	case machARM64:
		binary.LittleEndian.PutUint32(b[0:4], machO64LE)
		binary.LittleEndian.PutUint32(b[4:8], machCPUARM64)
	default:
		copy(b, "\x7fELF")
		machine := uint16(elfMachineAMD64)
		if want == elfARM64 {
			machine = elfMachineARM64
		}
		binary.LittleEndian.PutUint16(b[18:20], machine)
	}
	return b
}

// tarGz builds an archive of plain files from a map of path to content.
func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for name, body := range files {
		writeHdr(t, tw, &tar.Header{Name: name, Mode: 0o755,
			Size: int64(len(body)), Typeflag: tar.TypeReg}, body)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return gzipped(t, raw.Bytes())
}

func writeHdr(t *testing.T, tw *tar.Writer, h *tar.Header, body []byte) {
	t.Helper()
	if err := tw.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
}

func gzipped(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// tarXZBytes is the same archive under the compressor the QEMU fork publishes.
func tarXZBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for name, body := range files {
		writeHdr(t, tw, &tar.Header{Name: name, Mode: 0o755,
			Size: int64(len(body)), Typeflag: tar.TypeReg}, body)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw, err := xz.NewWriter(&out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fetchOne runs a fetch against a catalogue entry made for the test, so the
// real digests stay pinned to the real assets.
func fetchOne(t *testing.T, dir string, rel toolRelease, body []byte) (
	[]int64, error) {
	t.Helper()
	saved := toolReleases
	toolReleases = []toolRelease{rel}
	t.Cleanup(func() { toolReleases = saved })

	var seen []int64
	tc := &Toolchain{Dir: dir, HTTP: stubHTTP{body: body}}
	err := tc.Fetch(context.Background(), rel.Name, rel.Version,
		func(done, _ int64) { seen = append(seen, done) })
	return seen, err
}

// A plain binary lands under the name the lookup asks for, runnable, and the
// job hears about the bytes on the way.
func TestFetchingAPlainToolLandsWhereTheLookupSearches(t *testing.T) {
	dir := t.TempDir()
	body := fakeExec(elfAMD64, 5000)
	rel := toolRelease{
		Name: "a-tool", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/a-tool", SHA256: digest(body),
			Bytes: int64(len(body)), Kind: plainFile, Magic: elfAMD64,
		}},
	}
	seen, err := fetchOne(t, dir, rel, body)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	st, err := os.Stat(filepath.Join(dir, "a-tool"))
	if err != nil {
		t.Fatalf("nothing landed: %v", err)
	}
	// Windows has no executable bit: what may be run is decided by the name.
	// The bytes landing is the part that means something there, and the
	// progress check below covers it.
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0 {
		t.Error("the tool landed without an executable bit, so nothing can run it")
	}
	if len(seen) == 0 || seen[len(seen)-1] != int64(len(body)) {
		t.Errorf("progress reported %v, want it to reach %d bytes", seen, len(body))
	}
	// Nothing left behind: a .part beside the tool is the download this page
	// exists to account for, counted twice.
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("the partial download %s was left on disk", e.Name())
		}
	}
}

// An archive keeps its layout and gains a link, because both emulators resolve
// their own path to find what sits beside them.
func TestFetchingAnArchiveKeepsItsLayoutAndLinksTheBinary(t *testing.T) {
	dir := t.TempDir()
	body := tarGz(t, map[string][]byte{
		"qemu/bin/qemu-system-xtensa": fakeExec(elfAMD64, 2048),
		"qemu/share/qemu/esp32-rom.bin": []byte(
			"the ROM the emulator finds beside itself"),
	})
	rel := toolRelease{
		Name: "qemu-system-xtensa", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/q.tar", SHA256: digest(body),
			Bytes: int64(len(body)), Kind: tarGzip, Magic: elfAMD64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		}},
	}
	if _, err := fetchOne(t, dir, rel, body); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "qemu/share/qemu/esp32-rom.bin")); err != nil {
		t.Errorf("the archive's layout was not kept: %v", err)
	}
	link := filepath.Join(dir, "qemu-system-xtensa")
	if !fileExists(link) {
		t.Fatal("nothing landed under the name the lookup asks for")
	}
	// A link, not a copy: a bare copy starts and then cannot find its own ROM
	// images.
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the binary was copied rather than linked into place")
	}
}

// The same, over xz. Every QEMU asset the fork publishes is a .tar.xz, and a
// compressor the fetcher cannot open is a refusal that reads as a broken
// release: "not an archive this knows how to open" says nothing about which
// half is wrong.
func TestAnXZArchiveUnpacksLikeAGzippedOne(t *testing.T) {
	dir := t.TempDir()
	body := tarXZBytes(t, map[string][]byte{
		"qemu/bin/qemu-system-xtensa": fakeExec(elfAMD64, 2048),
		"qemu/share/qemu/esp32-rom.bin": []byte(
			"the ROM the emulator finds beside itself"),
	})
	rel := toolRelease{
		Name: "qemu-system-xtensa", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/q.tar.xz", SHA256: digest(body),
			Bytes: int64(len(body)), Kind: tarXZ, Magic: elfAMD64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		}},
	}
	if _, err := fetchOne(t, dir, rel, body); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "qemu/share/qemu/esp32-rom.bin")); err != nil {
		t.Errorf("the archive's layout was not kept: %v", err)
	}
	if !fileExists(filepath.Join(dir, "qemu-system-xtensa")) {
		t.Error("nothing landed under the name the lookup asks for")
	}
}

// The link inside an archive is kept, because Renode's portable package has
// one and its plugins are behind it.
func TestAnArchivesOwnLinksAreKept(t *testing.T) {
	dir := t.TempDir()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	body := fakeExec(elfAMD64, 2048)
	writeHdr(t, tw, &tar.Header{Name: "r/renode", Mode: 0o755,
		Size: int64(len(body)), Typeflag: tar.TypeReg}, body)
	writeHdr(t, tw, &tar.Header{Name: "r/libs/here", Mode: 0o644,
		Size: 2, Typeflag: tar.TypeReg}, []byte("hi"))
	writeHdr(t, tw, &tar.Header{Name: "r/plugins/libs", Mode: 0o777,
		Typeflag: tar.TypeSymlink, Linkname: "../libs"}, nil)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := gzipped(t, raw.Bytes())
	rel := toolRelease{
		Name: "renode", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/r.tar", SHA256: digest(archive),
			Bytes: int64(len(archive)), Kind: tarGzip, Magic: elfAMD64,
			Root: "r", Binary: "r/renode",
		}},
	}
	if _, err := fetchOne(t, dir, rel, archive); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "r/plugins/libs/here")); err != nil {
		t.Errorf("the archive's own link did not survive: %v", err)
	}
}

// A link pointing out of the tree is refused, however ordinary it looks.
func TestALinkOutOfTheTreeIsRefused(t *testing.T) {
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	writeHdr(t, tw, &tar.Header{Name: "r/escape", Mode: 0o777,
		Typeflag: tar.TypeSymlink, Linkname: "../../../etc/passwd"}, nil)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := gzipped(t, raw.Bytes())
	rel := toolRelease{
		Name: "renode", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/r.tar", SHA256: digest(archive),
			Bytes: int64(len(archive)), Kind: tarGzip, Magic: elfAMD64,
			Root: "r", Binary: "r/renode",
		}},
	}
	_, err := fetchOne(t, t.TempDir(), rel, archive)
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("a link out of the tools directory was accepted: %v", err)
	}
}

// A truncated or replaced download is refused, and leaves nothing behind.
func TestAWrongDigestIsRefusedAndNothingIsKept(t *testing.T) {
	dir := t.TempDir()
	body := fakeExec(elfAMD64, 5000)
	rel := toolRelease{
		Name: "a-tool", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL:    "https://example.invalid/a-tool",
			SHA256: strings.Repeat("00", 32),
			Bytes:  int64(len(body)), Kind: plainFile, Magic: elfAMD64,
		}},
	}
	_, err := fetchOne(t, dir, rel, body)
	if err == nil {
		t.Fatal("a download whose digest did not match was accepted")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Errorf("the refusal does not say what was wrong: %v", err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Errorf("the refused download left %d entries behind", len(ents))
	}
}

// A build for another architecture downloads and unpacks perfectly, and is the
// one failure that would otherwise surface as "the board did not come up".
func TestAWrongArchitectureBuildIsRefusedAtFetchTime(t *testing.T) {
	dir := t.TempDir()
	body := fakeExec(machARM64, 5000)
	rel := toolRelease{
		Name: "a-tool", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/a-tool", SHA256: digest(body),
			Bytes: int64(len(body)), Kind: plainFile, Magic: elfAMD64,
		}},
	}
	_, err := fetchOne(t, dir, rel, body)
	if err == nil {
		t.Fatal("a build for the wrong architecture was installed")
	}
	if !strings.Contains(err.Error(), "Mach-O") {
		t.Errorf("the refusal does not say what arrived: %v", err)
	}
	if fileExists(filepath.Join(dir, "a-tool")) {
		t.Error("an unrunnable tool was left where the lookup would find it")
	}
}

// An archive that unpacks without the binary it declared is not an
// installation, however cleanly it extracted.
func TestAnArchiveWithoutItsBinaryIsRefused(t *testing.T) {
	dir := t.TempDir()
	body := tarGz(t, map[string][]byte{"qemu/share/qemu/rom.bin": []byte("only data")})
	rel := toolRelease{
		Name: "qemu-system-xtensa", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/q.tar", SHA256: digest(body),
			Bytes: int64(len(body)), Kind: tarGzip, Magic: elfAMD64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		}},
	}
	if _, err := fetchOne(t, dir, rel, body); err == nil {
		t.Fatal("an archive with no emulator in it was accepted")
	}
}

// An archive is somebody else's data even when it is our own fork's release.
func TestAnArchiveCannotWriteOutsideTheToolsDirectory(t *testing.T) {
	dir := t.TempDir()
	body := tarGz(t, map[string][]byte{"qemu/../../escaped": []byte("no")})
	rel := toolRelease{
		Name: "qemu-system-xtensa", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{platform(): {
			URL: "https://example.invalid/q.tar", SHA256: digest(body),
			Bytes: int64(len(body)), Kind: tarGzip, Magic: elfAMD64,
			Root: "qemu", Binary: "qemu/bin/qemu-system-xtensa",
		}},
	}
	_, err := fetchOne(t, dir, rel, body)
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("an archive escaping its directory was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escaped")); err == nil {
		t.Fatal("the archive wrote outside the tools directory")
	}
}

// A platform with no build gets a refusal that names the platform, rather than
// a download that could not have worked.
func TestFetchingWhatThisPlatformCannotHaveIsRefused(t *testing.T) {
	rel := toolRelease{
		Name: "renode", Version: "test", Why: "w", Terms: "t",
		Assets: map[string]toolAsset{"plan9/mips": {}},
	}
	_, err := fetchOne(t, t.TempDir(), rel, nil)
	if err == nil || !strings.Contains(err.Error(), "renode") {
		t.Fatalf("a fetch with no build for this machine was accepted: %v", err)
	}
}
