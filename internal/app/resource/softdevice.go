// The Nordic SoftDevice, fetched from Nordic at runtime.
//
// It cannot be bundled and does not need to be. Whether emulating it for
// firmware testing is permitted at all was asked and answered - DevZone case
// 362437, recorded in docs/licence.md: it is, provided the end product runs on
// real Nordic hardware and the binary is neither reverse-engineered nor
// modified. How a copy reaches this machine is the same shape as everything
// else the application downloads: fetched from the source and cached, so
// MeshBench never becomes the one distributing it.
//
// Until now this was a manual step. tools/renode/rak4631-softdevice.resc
// expects /tmp/s140.bin and the instructions above it tell a person to find
// the .hex themselves, convert it, and put it there - which is why no nRF52
// board has ever been probed by anybody who had not already done that by hand.
package resource

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SoftDeviceKind is its row group.
const SoftDeviceKind Kind = "softdevice"

// softDeviceRelease is one published SoftDevice, as data rather than as code.
//
// AppBaseHex is the part worth keeping as a field: the SoftDevice version is
// decided by where the application starts, and pairing a 0x26000 image with
// v7.x overlaps the two. The failure is not subtle - the app makes 119 SVC
// calls into the SoftDevice and without a matching one it executes the
// stack-fill pattern - but it is silent, so the number lives beside the
// download that produces it.
type softDeviceRelease struct {
	Name, Version string
	URL           string
	// SHA256 is the archive's digest, observed from the real download rather
	// than taken on trust. A mismatch means the bytes are not the ones this
	// was written against, whatever the URL says.
	SHA256 string
	// HexName and LicenceName are the entries wanted out of the archive; the
	// licence is fetched with it because a copy nobody can read is worse than
	// no copy at all.
	HexName, LicenceName string
	HexSHA256            string
	AppBaseHex           uint32
	Bytes                int64
}

// softDevices is every release this knows how to fetch. One today, and the
// list is the place a second goes.
var softDevices = []softDeviceRelease{{
	Name: "s140", Version: "6.1.1",
	URL:         "https://nsscprodmedia.blob.core.windows.net/prod/software-and-other-downloads/softdevices/s140/s140nrf52611.zip",
	SHA256:      "6c82006927dfe8bf29e2b3ba19d23fe6e2a4fc70f0d9055101789d78756c425c",
	HexName:     "s140_nrf52_6.1.1_softdevice.hex",
	LicenceName: "s140_nrf52_6.1.1_license-agreement.txt",
	HexSHA256:   "b7666763e1b909d746ad69d2b7296c677ca86fc32fba0c5291d0535cc36335f0",
	AppBaseHex:  0x26000,
	Bytes:       389907,
}}

// SoftDevice provides the Nordic SoftDevice rows.
type SoftDevice struct {
	// CacheDir is the resource cache root; releases land under it by name and
	// version, so two can coexist.
	CacheDir string
	// HTTP is the client, so a test answers without a network.
	HTTP interface {
		Do(*http.Request) (*http.Response, error)
	}
	// Needed is how many nodes in this scenario cannot boot without one, so a
	// missing row can say that rather than sit there looking optional.
	Needed int
}

func (s *SoftDevice) Kind() Kind { return SoftDeviceKind }

// dir is where one release lives once fetched.
func (s *SoftDevice) dir(r softDeviceRelease) string {
	return filepath.Join(s.CacheDir, "softdevice", r.Name+"-"+r.Version)
}

// HexPath is the converted-from location for a release, or "" when it is not
// cached. Exported because the emulator bring-up needs the file, not the row.
func (s *SoftDevice) HexPath(name, version string) string {
	for _, r := range softDevices {
		if r.Name == name && r.Version == version {
			p := filepath.Join(s.dir(r), r.HexName)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

func (s *SoftDevice) List(_ context.Context) ([]Row, error) {
	out := make([]Row, 0, len(softDevices))
	for _, r := range softDevices {
		row := Row{
			Kind: SoftDeviceKind, Name: r.Name, Version: r.Version,
			Bytes: r.Bytes, Estimated: true, State: Available,
			// Never automatic. It is somebody else's licensed binary, and a
			// person should see the terms arrive rather than find it already
			// on their disk.
			Auto: false,
			Why: fmt.Sprintf("nRF52 boards boot MBR then SoftDevice then MeshCore; "+
				"this one pairs with an application based at 0x%X", r.AppBaseHex),
		}
		hexPath := filepath.Join(s.dir(r), r.HexName)
		if fi, err := os.Stat(hexPath); err == nil {
			row.State, row.Path, row.Bytes, row.Estimated = OnDisk, s.dir(r), fi.Size(), false
		} else if s.Needed > 0 {
			row.State = Needed
			row.Why = fmt.Sprintf("%d node(s) here cannot boot without it", s.Needed)
		}
		out = append(out, row)
	}
	return out, nil
}

func (s *SoftDevice) Remove(_ context.Context, row Row) error {
	for _, r := range softDevices {
		if r.Name == row.Name && r.Version == row.Version {
			return os.RemoveAll(s.dir(r))
		}
	}
	return fmt.Errorf("resource: no SoftDevice called %s %s", row.Name, row.Version)
}

// Licence is Nordic's own text for a fetched release, for the interface to
// show. Empty when nothing is cached: there is no licence to display for a
// file this machine does not have.
func (s *SoftDevice) Licence(name, version string) string {
	for _, r := range softDevices {
		if r.Name != name || r.Version != version {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir(r), r.LicenceName))
		if err != nil {
			return ""
		}
		return string(b)
	}
	return ""
}

// Fetch downloads one release, verifies it, and unpacks the two files worth
// keeping. progress is called with bytes received and the total where the
// server declares one.
func (s *SoftDevice) Fetch(ctx context.Context, name, version string,
	progress func(done, total int64)) error {

	var rel softDeviceRelease
	for _, r := range softDevices {
		if r.Name == name && r.Version == version {
			rel = r
		}
	}
	if rel.URL == "" {
		return fmt.Errorf("resource: no SoftDevice called %s %s is known", name, version)
	}
	if s.CacheDir == "" {
		return fmt.Errorf("resource: no cache directory to put a SoftDevice in")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.URL, nil)
	if err != nil {
		return err
	}
	client := s.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resource: fetching the SoftDevice: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resource: Nordic answered %s for the SoftDevice", resp.Status)
	}

	// Held in memory rather than streamed to disk: it is under half a
	// megabyte, and the digest has to be checked before any of it is kept.
	var buf bytes.Buffer
	sum := sha256.New()
	if _, err := io.Copy(io.MultiWriter(&buf, sum), &countingReader{
		r: resp.Body, total: resp.ContentLength, report: progress,
	}); err != nil {
		return fmt.Errorf("resource: reading the SoftDevice: %w", err)
	}
	if got := hex.EncodeToString(sum.Sum(nil)); got != rel.SHA256 {
		return fmt.Errorf("resource: the SoftDevice archive is not the one this "+
			"was written against (sha256 %s, expected %s) - refusing to unpack it",
			got, rel.SHA256)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return fmt.Errorf("resource: the SoftDevice archive will not open: %w", err)
	}
	dir := s.dir(rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	want := map[string]bool{rel.HexName: true, rel.LicenceName: true}
	found := 0
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if !want[base] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		if base == rel.HexName {
			if got := sha256.Sum256(b); hex.EncodeToString(got[:]) != rel.HexSHA256 {
				return fmt.Errorf("resource: the SoftDevice image inside the archive " +
					"is not the one this was written against")
			}
			if !strings.HasPrefix(string(b), ":") {
				return fmt.Errorf("resource: the SoftDevice image is not Intel HEX")
			}
		}
		if err := os.WriteFile(filepath.Join(dir, base), b, 0o644); err != nil {
			return err
		}
		found++
	}
	if found != len(want) {
		return fmt.Errorf("resource: the archive held %d of the %d files wanted",
			found, len(want))
	}
	return nil
}

// countingReader reports progress as bytes arrive.
type countingReader struct {
	r      io.Reader
	total  int64
	done   int64
	report func(done, total int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.done += int64(n)
	if c.report != nil {
		c.report(c.done, c.total)
	}
	return n, err
}
