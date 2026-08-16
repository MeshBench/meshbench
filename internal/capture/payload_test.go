package capture_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/MeshBench/meshbench/internal/capture"
)

// field finds a named field, so a test says what it means rather than
// indexing into a slice whose order is not the point.
func field(t *testing.T, fs []capture.Field, name string) capture.Field {
	t.Helper()
	for _, f := range fs {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no field named %q in %v", name, names(fs))
	return capture.Field{}
}

func names(fs []capture.Field) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return out
}

// advertWith builds an advert carrying the given app_data, the way
// Mesh::createAdvert lays one out: key, timestamp, signature, app data.
func advertWith(appData []byte) []byte {
	f := []byte{0x01 | (0x04 << 2), 0x00} // flood, advert, no path
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i)
	}
	f = append(f, pub...)
	ts := make([]byte, 4)
	binary.LittleEndian.PutUint32(ts, 1788220800)
	f = append(f, ts...)
	f = append(f, make([]byte, 64)...)
	return append(f, appData...)
}

// An advert is the one payload that is entirely in clear, and the part we
// never read was the part that says what the node is, where it is and what it
// calls itself. All of it comes out of one flags byte and a walk.
func TestAdvertAppDataIsRead(t *testing.T) {
	app := []byte{0x02 | 0x10 | 0x80} // repeater, with lat/lon, with name
	lat := make([]byte, 4)
	lon := make([]byte, 4)
	var latV, lonV int32 = 56242100, -3278183
	binary.LittleEndian.PutUint32(lat, uint32(latV))
	binary.LittleEndian.PutUint32(lon, uint32(lonV))
	app = append(app, lat...)
	app = append(app, lon...)
	app = append(app, []byte("West Lomond")...)

	d := capture.Dissect(advertWith(app))
	if d.Truncated {
		t.Fatalf("a well-formed advert was called malformed: %s", d.Problem)
	}
	if got := field(t, d.PayloadFields, "latitude").Decoded; got != "56.242100°" {
		t.Errorf("latitude decoded as %q, want 56.242100°", got)
	}
	if got := field(t, d.PayloadFields, "longitude").Decoded; got != "-3.278183°" {
		t.Errorf("longitude decoded as %q, want -3.278183°", got)
	}
	if got := field(t, d.PayloadFields, "name").Value; got != "West Lomond" {
		t.Errorf("name = %q, want West Lomond", got)
	}
	// The flags byte is the thing that made all three readable, so it says so.
	if got := field(t, d.PayloadFields, "flags").Decoded; got != "repeater, with position, name" {
		t.Errorf("flags decoded as %q", got)
	}
	// A timestamp is both the number on the wire and the date it means.
	ts := field(t, d.PayloadFields, "timestamp")
	if ts.Value != "1788220800" || ts.Decoded != "2026-09-01 00:00:00Z" {
		t.Errorf("timestamp = %q / %q", ts.Value, ts.Decoded)
	}
}

// Each advert field is present only if its own bit is set. Reading it as a
// fixed struct puts the name where the latitude should be the moment a node
// adverts without a position.
func TestAdvertWithoutLocationDoesNotInventOne(t *testing.T) {
	app := append([]byte{0x01 | 0x80}, []byte("Jazzy")...) // chat, name, no lat/lon
	d := capture.Dissect(advertWith(app))
	for _, f := range d.PayloadFields {
		if f.Name == "latitude" || f.Name == "longitude" {
			t.Errorf("read a %s from an advert that carries no position", f.Name)
		}
	}
	if got := field(t, d.PayloadFields, "name").Value; got != "Jazzy" {
		t.Errorf("name = %q, want Jazzy - the name must not be read at the wrong offset", got)
	}
}

// A trace collects the SNR each hop measured in the packet's *path* area, one
// signed byte scaled by four - not hashes. Reading it as hashes prints the
// measurement the packet exists to carry as meaningless hex.
func TestATracesPathAreaIsSNRNotHashes(t *testing.T) {
	f := []byte{0x02 | (0x09 << 2), 0x02, 0x14, 0xEC} // direct trace, 2 SNR bytes
	tag := make([]byte, 4)
	binary.LittleEndian.PutUint32(tag, 0xDEADBEEF)
	f = append(f, tag...)
	f = append(f, 0, 0, 0, 0) // auth
	f = append(f, 0x00)       // flags: 1-byte path hashes

	d := capture.Dissect(f)
	if len(d.PathFields) != 2 {
		t.Fatalf("got %d path entries, want 2: %v", len(d.PathFields), names(d.PathFields))
	}
	if got := d.PathFields[0].Decoded; got != "+5.00 dB" {
		t.Errorf("first hop SNR = %q, want +5.00 dB (0x14 = 20, ÷4)", got)
	}
	if got := d.PathFields[1].Decoded; got != "-5.00 dB" {
		t.Errorf("second hop SNR = %q, want -5.00 dB (0xEC = -20, ÷4)", got)
	}
}

// Every other type reads the path area as relay hashes, as before.
func TestANonTracePathAreaIsStillHashes(t *testing.T) {
	d := capture.Dissect([]byte{0x01 | (0x04 << 2), 0x02, 0xAB, 0xCD})
	if len(d.PathFields) != 2 {
		t.Fatalf("got %d path entries, want 2", len(d.PathFields))
	}
	if d.PathFields[0].Value != "AB" || d.PathFields[1].Value != "CD" {
		t.Errorf("path read as %q, %q; want AB, CD", d.PathFields[0].Value, d.PathFields[1].Value)
	}
}

// An anonymous request hands over a whole public key in clear - the one
// encrypted-body type whose sender can be pinned down exactly.
func TestAnonymousRequestNamesItsSenderKey(t *testing.T) {
	f := []byte{0x01 | (0x07 << 2), 0x00, 0xA7} // flood anon-req, no path, dest hash
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i + 1)
	}
	f = append(f, pub...)
	f = append(f, 0xDE, 0xAD)       // MAC
	f = append(f, 1, 2, 3, 4, 5, 6) // ciphertext

	d := capture.Dissect(f)
	key := field(t, d.PayloadFields, "sender public key")
	if key.Length != 32 {
		t.Errorf("sender key is %d bytes, want 32", key.Length)
	}
	enc := field(t, d.PayloadFields, "encrypted")
	if enc.Value != "6 bytes" {
		t.Errorf("encrypted body = %q, want 6 bytes", enc.Value)
	}
}

// Only PAYLOAD_VER_1 defines the hash and MAC widths. Parsing an undefined
// version with version 1's widths would emit a full set of confidently
// mislabelled fields at wrong offsets, so it must decline instead.
func TestAnUndefinedPayloadVersionIsNotParsed(t *testing.T) {
	// Version 1 in the top two bits: defined nowhere in v1.17.0.
	f := []byte{0x01 | (0x02 << 2) | (0x01 << 6), 0x00, 0xAA, 0xBB, 0xCC, 0xDD}
	d := capture.Dissect(f)
	if d.Version != 1 {
		t.Fatalf("version = %d, want 1", d.Version)
	}
	for _, f := range d.PayloadFields {
		if f.Name == "destination hash" || f.Name == "MAC" {
			t.Errorf("parsed %q out of a payload version whose widths are undefined", f.Name)
		}
	}
	if len(d.PayloadFields) != 1 || d.PayloadFields[0].Decoded != "not parsed" {
		t.Errorf("expected one field saying it was not parsed, got %v", names(d.PayloadFields))
	}
}

// A multipart packet names the type it is carrying, so it can be read without
// having to find its siblings first.
func TestMultipartNamesWhatItCarries(t *testing.T) {
	// remaining=3 in the high nibble, ack (0x03) in the low.
	f := []byte{0x01 | (0x0A << 2), 0x00, (3 << 4) | 0x03, 0x11, 0x22}
	d := capture.Dissect(f)
	got := field(t, d.PayloadFields, "part header").Decoded
	if got != "3 more to come, carrying ack" {
		t.Errorf("part header decoded as %q", got)
	}
}

// The frame's shape, before any of its detail. The path length byte is its
// own span: folding it into the transport codes - as the first mock did -
// puts every offset after it one byte out.
func TestSpansCoverTheFrameInOrder(t *testing.T) {
	f := []byte{0x00 | (0x04 << 2), 0x11, 0x22, 0x33, 0x44, 0x01, 0xAB, 0x05}
	d := capture.Dissect(f)
	var got []string
	at := 0
	for _, s := range d.Spans {
		got = append(got, s.Name)
		if s.Offset != at {
			t.Errorf("span %q starts at %d, want %d - spans must tile the frame", s.Name, s.Offset, at)
		}
		at += s.Length
	}
	if at != len(f) {
		t.Errorf("spans cover %d bytes of a %d byte frame", at, len(f))
	}
	want := []string{"header", "transport codes", "path length", "path", "payload — advert"}
	if len(got) != len(want) {
		t.Fatalf("spans = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("span %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// The two transport codes are a region scope code and a reserved word. The
// pair {0,0} has its own meaning in the firmware and must not read as a scope.
func TestTransportCodesAreDescribedAsScopeNotAddresses(t *testing.T) {
	zero := capture.Dissect([]byte{0x00 | (0x04 << 2), 0, 0, 0, 0, 0x00})
	var detail string
	for _, s := range zero.Spans {
		if s.Name == "transport codes" {
			detail = s.Detail
		}
	}
	if detail != "0000 0000 — addressed to no region" {
		t.Errorf("all-zero codes described as %q", detail)
	}
}

// A payload that ends before its type's fields do must say so, not carry on
// reading from an un-advanced cursor.
//
// rd.field declines to read past the end and leaves the cursor where it was,
// so a caller that ignores that resumes at the wrong offset. On a short
// advert that meant reading the flags, position and name out of *signature
// bytes* and reporting them as fact - a node type, a claimed position and a
// name, none of which were in the packet, with nothing marked truncated.
func TestATruncatedAdvertDoesNotInventFieldsFromTheSignature(t *testing.T) {
	// Long enough for the key and timestamp, far too short for the signature.
	short := []byte{0x01 | (0x04 << 2), 0x00}
	body := make([]byte, 40)
	body[36] = 0x92 // would read as "repeater, with position, name"
	body[37], body[38], body[39] = 'A', 'B', 'C'
	short = append(short, body...)

	d := capture.Dissect(short)
	for _, f := range d.PayloadFields {
		switch f.Name {
		case "flags", "latitude", "longitude", "name", "node type":
			t.Errorf("read %q out of a payload too short to contain it: %q / %q",
				f.Name, f.Value, f.Decoded)
		}
	}
	if _, err := lastNamed(d.PayloadFields, "truncated"); err != nil {
		t.Errorf("a short payload was read as a whole one: %v", names(d.PayloadFields))
	}
}

// The same guard on an addressed type, whose prefix is version-dependent.
func TestATruncatedAddressedPayloadSaysSoRatherThanMislabelling(t *testing.T) {
	// Claims a text message, carries one byte of the three-byte prefix.
	d := capture.Dissect([]byte{0x01 | (0x02 << 2), 0x00, 0xAA})
	for _, f := range d.PayloadFields {
		if f.Name == "MAC" {
			t.Error("labelled a MAC in a payload with no room for one")
		}
	}
	if _, err := lastNamed(d.PayloadFields, "truncated"); err != nil {
		t.Errorf("a short addressed payload was read as whole: %v", names(d.PayloadFields))
	}
}

func lastNamed(fs []capture.Field, name string) (capture.Field, error) {
	for _, f := range fs {
		if f.Name == name {
			return f, nil
		}
	}
	return capture.Field{}, errNotFound
}

var errNotFound = fmt.Errorf("not found")
