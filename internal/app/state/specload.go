package state

// Reading the descriptions back: one file's worth at a time, merged into the
// single map every generated document is written from.
//
// Merging rather than concatenating means the split into sibling files is a
// convenience for whoever edits them and invisible to everything downstream,
// which is the only reason it is safe to keep them small.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// specFileSuffix marks a file as the descriptions of the verbs registered by
// the Go file of the same name.
const specFileSuffix = ".verbs.json"

// LoadSpecs reads every *.verbs.json under root and merges them, keyed by verb.
//
// Two files describing one verb is refused rather than resolved: a last one
// wins would drop a description out of the reference with nothing anywhere
// saying it had gone, which is the failure this whole arrangement exists to
// make impossible.
func LoadSpecs(root string) (map[string]Spec, error) {
	out := map[string]Spec{}
	from := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), specFileSuffix) {
			return err
		}
		file, err := readSpecFile(path)
		if err != nil {
			return err
		}
		names := make([]string, 0, len(file))
		for verb := range file {
			names = append(names, verb)
		}
		// Sorted, so the file that gets named in a duplicate does not depend on
		// map iteration and the error reads the same on every machine.
		sort.Strings(names)
		for _, verb := range names {
			if prev, ok := from[verb]; ok {
				return fmt.Errorf("%s is described in both %s and %s", verb, prev, path)
			}
			from[verb] = path
			out[verb] = file[verb]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// readSpecFile reads one sibling file and checks what it holds. Unknown fields
// are refused: a misspelt key would otherwise be accepted and then be missing
// from the reference, which is the same silence as having written nothing.
func readSpecFile(path string) (map[string]Spec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var specs map[string]Spec
	if err := dec.Decode(&specs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for verb, sp := range specs {
		if err := sp.validate(verb); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return specs, nil
}

// validate is the little that can be checked without the tree in front of it:
// that something was said, and that a parameter's type is one a client can bind.
func (s Spec) validate(verb string) error {
	if verb == "" {
		return fmt.Errorf("a description with no verb name")
	}
	if s.What == "" {
		return fmt.Errorf("%s has an entry that says nothing", verb)
	}
	for _, p := range s.Params {
		if p.Name == "" {
			return fmt.Errorf("%s has a parameter with no name", verb)
		}
		switch p.Type {
		case ParamString, ParamNumber, ParamBool, ParamObject, ParamArray:
		default:
			return fmt.Errorf("%s parameter %s has type %q, which is not one a client can bind",
				verb, p.Name, p.Type)
		}
		if p.What == "" {
			return fmt.Errorf("%s parameter %s says nothing", verb, p.Name)
		}
	}
	return nil
}
