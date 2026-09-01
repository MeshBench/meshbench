// Package flagdump lets the binary describe its own command line, so the CLI
// reference is generated from the flag declarations instead of written beside
// them and left to rot.
//
// It is a collector rather than a printer because a command's flags exist only
// for the instant that command is starting: each builds its own flag.FlagSet
// inside its run function, parses it and gets on with the work. So a process
// asked to describe itself walks every command, records the set each declares
// at the moment it declares it, and prints the lot at the end.
//
// Asking the binary rather than reading the source is the whole point. The
// defaults it reports are the values the flag package will actually use,
// including the ones computed at startup - a cache directory under the user's
// home, a tile zoom that comes from a constant in another package - which no
// amount of reading cmd/meshbench can know.
package flagdump

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
)

// ErrRecorded is what a command returns instead of running, once its flags
// have been taken. Named as an error rather than handled by an exit inside
// Record so that the caller keeps deciding when the process ends: the walk has
// fifteen more commands to visit after this one.
var ErrRecorded = errors.New("command line recorded; nothing was run")

// Example is one worked invocation, held beside the command it demonstrates.
//
// Extraction cannot invent these, and a reference that lists flags without
// showing one being used documents the vocabulary and not the language.
type Example struct {
	// Setup is an optional shell line that has to run first, for a command
	// that needs a file it cannot ship, and is empty for most.
	Setup string `json:"setup,omitempty"`
	Line  string `json:"line"`
	Why   string `json:"why"`
}

// Flag is one declared flag, as the flag package sees it.
type Flag struct {
	Name string `json:"name"`
	// Type is what -h prints after the name: string, int, float, duration,
	// uint, value - or bool, which -h prints as nothing at all because a
	// boolean flag takes no argument.
	Type    string `json:"type"`
	Default string `json:"default"`
	Usage   string `json:"usage"`
}

// Command is one command, from both the places that know about it: the index
// that lists it, and the flag set it builds when it starts.
type Command struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// Describe is the line the command's own -h prints above its flags, which
	// is not always the summary the index gives it. Empty where the command
	// has none, and the summary then stands for both.
	Describe string    `json:"describe"`
	Examples []Example `json:"examples"`
	Flags    []Flag    `json:"flags"`
}

var (
	wanted bool
	order  []string
	byName = map[string]*Command{}
)

// Begin marks this process as one started to describe itself rather than to
// simulate anything.
func Begin() { wanted = true }

// Wanted reports whether the commands should record their flags and stop.
func Wanted() bool { return wanted }

// Note records what the command index says about a command, before it is run.
func Note(name, summary string, examples []Example) {
	c := entry(name)
	c.Summary = summary
	c.Examples = examples
}

// Record takes the flag set a command has just finished declaring.
func Record(name, describe string, fs *flag.FlagSet) {
	c := entry(name)
	c.Describe = describe
	c.Flags = nil
	// VisitAll walks in lexical order, which is the order -h prints and the
	// order the reference needs: an alphabetical list is the one a reader can
	// search, and a declaration order is an accident of how the file grew.
	fs.VisitAll(func(f *flag.Flag) {
		kind, usage := flag.UnquoteUsage(f)
		if kind == "" {
			kind = "bool"
		}
		c.Flags = append(c.Flags, Flag{
			Name: f.Name, Type: kind, Default: f.DefValue, Usage: usage,
		})
	})
}

func entry(name string) *Command {
	if c, ok := byName[name]; ok {
		return c
	}
	c := &Command{Name: name}
	byName[name] = c
	order = append(order, name)
	return c
}

// Dump is everything one run of the binary has to say about its command line.
type Dump struct {
	// CacheDir is this machine's cache root, reported so a generated document
	// can put something portable where the default cache paths would otherwise
	// name whoever ran the generator. The flags themselves keep the real path,
	// because that is what -h has to print to be useful.
	CacheDir string     `json:"cache_dir"`
	Commands []*Command `json:"commands"`
}

// Emit writes everything recorded, in the order the index lists it.
func Emit(w io.Writer) error {
	out := Dump{Commands: make([]*Command, 0, len(order))}
	out.CacheDir, _ = os.UserCacheDir()
	for _, name := range order {
		out.Commands = append(out.Commands, byName[name])
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
