// What has been decided about one build, kept beside the build itself.
//
// A setting like "bring the coprocessors up enabled" is a property of the
// firmware being looked at, not of the board or of the machine: the same
// LilyGo T-Deck runs a stock MeshCore image that wants none of it and a Rust
// one that cannot be seen past without it. So it is stored per build.
//
// Beside the image rather than in a central file, because that is what makes
// it survive: a build deleted takes its settings with it, one renamed carries
// them along, and a cache copied to another machine arrives with them intact.
// A central index would have to be reconciled with the directory on every
// read, and the failure mode of not doing so is settings attached to a build
// that no longer exists.
package firmware

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildSettings is everything the library knows about a build that is not
// written in its name or its bytes.
//
// Every field's zero value is the behaviour of a build nobody has touched, so
// an absent file and an empty one mean the same thing, and a build imported
// before any of this existed reads correctly.
type BuildSettings struct {
	// CoprocAtReset brings the emulated coprocessors up enabled, which the
	// part does not do. See EnvCoprocAtReset for why a firmware might need it
	// and what it costs in honesty.
	CoprocAtReset bool `json:"coproc_at_reset,omitempty"`

	// SPIController is which of the part's general-purpose SPI controllers
	// this firmware drives its peripherals from, or zero to take the board's
	// own answer.
	//
	// A property of the build, not of the board. The pins are fixed in copper
	// and the GPIO matrix routes whichever controller the firmware picks onto
	// them, so two firmwares for one handheld can use different controllers
	// and both are right. MeshCore's T-Deck build uses GPSPI3; a Rust one for
	// the same board uses GPSPI2, and against a machine wired for the other
	// its radio, its card and its screen all answer nothing at all - which
	// looks exactly like a board with nothing fitted.
	SPIController int `json:"spi_controller,omitempty"`

	// Notes is whatever the person who imported it wanted the next person to
	// know: where it came from, what it is for, what is wrong with it.
	Notes string `json:"notes,omitempty"`
}

// Zero reports whether anything has been decided at all, which is when the
// file is removed rather than written empty.
func (b BuildSettings) Zero() bool { return b == BuildSettings{} }

// settingsSuffix is appended to the image's own name.
//
// Appended rather than replacing the extension: two builds in a directory may
// differ only by extension, and a settings file named from the stem alone
// would be claimed by both. It also has to be a suffix listBoard can refuse,
// or the settings would list as builds of their own.
const settingsSuffix = ".msim.json"

// SettingsPath is where a build's settings sit.
func SettingsPath(image string) string { return image + settingsSuffix }

// isSettingsFile reports whether a name in the cache is settings rather than a
// build.
func isSettingsFile(name string) bool { return strings.HasSuffix(name, settingsSuffix) }

// LoadBuildSettings reads what has been decided about a build.
//
// A missing file is not an error and neither is an unreadable one: the answer
// to "what has been decided about this build" is "nothing" in both cases, and
// a library that refused to list a build because a sidecar was corrupt would
// be unusable for the one reason nobody could see.
func LoadBuildSettings(image string) BuildSettings {
	data, err := os.ReadFile(SettingsPath(image))
	if err != nil {
		return BuildSettings{}
	}
	var s BuildSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return BuildSettings{}
	}
	return s
}

// SaveBuildSettings writes them, and removes the file when there is nothing
// left to say.
//
// Removing rather than writing "{}" so that a build returned to its defaults
// is indistinguishable from one that never had settings - otherwise the cache
// slowly fills with files recording that nothing was decided.
func SaveBuildSettings(image string, s BuildSettings) error {
	p := SettingsPath(image)
	if s.Zero() {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o644)
}

// Rename moves a build to a new role, label or board, taking its settings with
// it.
//
// The name is the identity here: a board build is stored as
// <board>/<role>@<label>.bin and nothing else records what it is, so changing
// what it is called is a move. That also means the caller has to repoint
// anything pinned to the old name, which this cannot do from inside the
// firmware package and the verb above it does.
//
// Native builds are refused. Their role is encoded in a filename this package
// composes from the host's OS and architecture and their version is the
// directory they sit in, so "rename" would mean three different things at
// once; and unlike a board image, a native build is reproducible by fetching
// it again.
func Rename(cacheDir string, in Installed, role, label, board string) (Installed, error) {
	if in.Native {
		return Installed{}, fmt.Errorf(
			"firmware: %s is a build for this machine, named after the host it "+
				"runs on rather than after anything chosen - only board images "+
				"can be renamed", in.Version)
	}
	if role == "" || label == "" || board == "" {
		return Installed{}, fmt.Errorf("firmware: a build needs a role, a label and a board")
	}
	if strings.ContainsAny(label, `/\`+labelSep) || strings.ContainsAny(role, `/\`+labelSep) ||
		strings.ContainsAny(board, `/\`+labelSep) {
		return Installed{}, fmt.Errorf(
			"firmware: %q is not usable as a name; no %s, / or \\ in a role, a "+
				"label or a board", label, labelSep)
	}
	root, err := filepath.Abs(cacheDir)
	if err != nil {
		return Installed{}, err
	}
	src, err := filepath.Abs(in.Path)
	if err != nil {
		return Installed{}, err
	}
	// The same guard Remove has, for the same reason: a move driven by a path
	// somebody handed in is a move that relocates whatever a mistake hands it.
	if !strings.HasPrefix(src, root+string(os.PathSeparator)) {
		return Installed{}, fmt.Errorf(
			"firmware: refusing to rename %s, which is outside the cache", src)
	}
	dst := filepath.Join(root, boardDir, board, role+labelSep+label+filepath.Ext(src))
	if dst == src {
		return in, nil
	}
	if _, err := os.Stat(dst); err == nil {
		return Installed{}, fmt.Errorf(
			"firmware: %s %s for %s already exists - delete it first, or choose "+
				"another name", role, label, board)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Installed{}, err
	}
	if err := os.Rename(src, dst); err != nil {
		return Installed{}, err
	}
	// The settings follow the build. Failing to move them is not worth
	// failing the rename over - the build has already moved, and the worst
	// case is a setting reverting to its default, which the window shows.
	if _, err := os.Stat(SettingsPath(src)); err == nil {
		_ = os.Rename(SettingsPath(src), SettingsPath(dst))
	}
	// Take the old board's directory with it when it empties, so moving the
	// last build off a board does not leave the board listed with nothing in
	// it.
	if entries, err := os.ReadDir(filepath.Dir(src)); err == nil && len(entries) == 0 {
		_ = os.Remove(filepath.Dir(src))
	}
	return Installed{
		Version: label, Role: role, Board: board,
		Path: dst, Bytes: sizeOf(dst),
	}, nil
}
