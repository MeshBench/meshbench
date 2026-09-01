package flagdump

import (
	"bytes"
	"encoding/json"
	"flag"
	"testing"
	"time"
)

// What a flag set says about itself has to survive being recorded, because the
// reference is built from this and nothing else looks at it again.
func TestRecordKeepsTypeDefaultAndUsage(t *testing.T) {
	reset()
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.Float64("from-lat", 56.25, "latitude of the first station")
	fs.Bool("offline", false, "never download")
	fs.String("junit", "", "write a JUnit XML report here")
	fs.Duration("quit-after", 3*time.Second, "exit after this long")
	Record("link", "link budget between two points", fs)

	got := map[string]Flag{}
	for _, f := range byName["link"].Flags {
		got[f.Name] = f
	}
	for _, want := range []Flag{
		{"from-lat", "float", "56.25", "latitude of the first station"},
		// A boolean prints no type after its name, because it takes no
		// argument. Left as the empty string it reached the document as a
		// column with nothing in it.
		{"offline", "bool", "false", "never download"},
		{"junit", "string", "", "write a JUnit XML report here"},
		{"quit-after", "duration", "3s", "exit after this long"},
	} {
		if got[want.Name] != want {
			t.Errorf("-%s recorded as %+v, want %+v", want.Name, got[want.Name], want)
		}
	}
	if len(got) != 4 {
		t.Errorf("recorded %d flags, want 4", len(got))
	}
}

// Recorded twice, a command holds what it declared once. A second walk that
// appended would double every flag in the reference.
func TestRecordingTwiceDoesNotDouble(t *testing.T) {
	reset()
	fs := flag.NewFlagSet("airtime", flag.ContinueOnError)
	fs.Int("sf", 10, "spreading factor")
	Record("airtime", "time on air", fs)
	Record("airtime", "time on air", fs)
	if n := len(byName["airtime"].Flags); n != 1 {
		t.Fatalf("recorded twice, holding %d flags", n)
	}
}

// The index and the flag set are learned at different moments, and both have
// to end up on the same command.
func TestEmitJoinsTheIndexToTheFlags(t *testing.T) {
	reset()
	Note("boards", "the hardware profiles this build knows about",
		[]Example{{Line: "meshbench boards", Why: "the only command with no flags"}})
	Record("boards", "", flag.NewFlagSet("boards", flag.ContinueOnError))

	var buf bytes.Buffer
	if err := Emit(&buf); err != nil {
		t.Fatal(err)
	}
	var out Dump
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("emitted something that is not JSON: %v", err)
	}
	if len(out.Commands) != 1 {
		t.Fatalf("emitted %d commands, want 1", len(out.Commands))
	}
	c := out.Commands[0]
	if c.Name != "boards" || c.Summary == "" || len(c.Examples) != 1 {
		t.Errorf("index lost on the way out: %+v", c)
	}
	if len(c.Flags) != 0 {
		t.Errorf("boards declares no flags, emitted %d", len(c.Flags))
	}
}

// Emit keeps the order the index was built in, which is the order a reader
// meets the commands rather than the alphabet.
func TestEmitKeepsTheIndexOrder(t *testing.T) {
	reset()
	for _, name := range []string{"link", "airtime", "boards"} {
		Note(name, name, nil)
	}
	var buf bytes.Buffer
	if err := Emit(&buf); err != nil {
		t.Fatal(err)
	}
	var out Dump
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"link", "airtime", "boards"} {
		if out.Commands[i].Name != want {
			t.Errorf("position %d is %q, want %q", i, out.Commands[i].Name, want)
		}
	}
}

func reset() {
	wanted = false
	order = nil
	byName = map[string]*Command{}
}
