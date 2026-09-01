package main

import (
	"flag"
	"strings"
	"testing"
)

// Zero is a latitude and a longitude like any other, and the two together are a
// real place in the Gulf of Guinea. Six commands inferred "not given" from the
// value, so the equator and the prime meridian were refused as missing flags -
// and a study area that straddles either line could not be asked for at all.
func TestAFlagGivenAsZeroCountsAsGiven(t *testing.T) {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	lat := fs.Float64("lat", 0, "")
	lon := fs.Float64("lon", 0, "")
	if err := fs.Parse([]string{"-lat", "0", "-lon", "0"}); err != nil {
		t.Fatal(err)
	}
	if *lat != 0 || *lon != 0 {
		t.Fatalf("parsed %v, %v", *lat, *lon)
	}
	if err := requireAll(notGiven(fs, "lat", "lon")); err != nil {
		t.Fatalf("the equator at the prime meridian was refused: %v", err)
	}
}

// And a flag genuinely left off is still named, one message for all of them.
func TestFlagsLeftOffAreAllNamedAtOnce(t *testing.T) {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.Float64("from-lat", 0, "")
	fs.Float64("from-lon", 0, "")
	fs.Float64("to-lat", 0, "")
	fs.Float64("to-lon", 0, "")
	if err := fs.Parse([]string{"-from-lat", "56.25"}); err != nil {
		t.Fatal(err)
	}
	err := requireAll(notGiven(fs, "from-lat", "from-lon", "to-lat", "to-lon"))
	if err == nil {
		t.Fatal("three missing flags were accepted")
	}
	for _, want := range []string{"-from-lon", "-to-lat", "-to-lon"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not name %s", err, want)
		}
	}
	if strings.Contains(err.Error(), "-from-lat") {
		t.Errorf("%q names a flag that was given", err)
	}
}
