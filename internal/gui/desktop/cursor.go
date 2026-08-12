//go:build linux || freebsd || openbsd || netbsd

// Package desktop reconciles a window with the desktop it opens on.
//
// Gio asks libwayland for a cursor theme by name and size, and takes both from
// XCURSOR_THEME and XCURSOR_SIZE. Neither is part of the Wayland protocol, so
// nothing guarantees they are set: Plasma, for one, configures the cursor
// through its own files and exports nothing into the session. Gio then falls
// back to size 32 and the unnamed default theme, and the window gets a pointer
// a third larger than every other window on the screen, in the wrong theme.
//
// So read what the desktop actually decided, from the same files the desktop
// wrote, and set the two variables before the window is made.
package desktop

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The size to assume when nothing on the system says otherwise.
//
// 24 rather than Gio's 32 because 24 is what Plasma, GNOME and the Xcursor
// spec's own examples default to. A wrong guess here is visible, so guess the
// one the neighbours use.
const defaultSize = 24

// MatchSystemCursor points Gio at the desktop's cursor theme and size.
//
// Call it before the first window is created; Gio loads the theme once, when
// the window is made, and never looks again. Variables already in the
// environment are left alone - somebody who exported XCURSOR_SIZE meant it,
// and a session that does export them is right more often than this file's
// guessing is.
func MatchSystemCursor() (theme string, size int) {
	theme, hasTheme := os.LookupEnv("XCURSOR_THEME")
	size, hasSize := 0, false
	if s, ok := os.LookupEnv("XCURSOR_SIZE"); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			size, hasSize = n, true
		}
	}
	if hasTheme && hasSize {
		return theme, size
	}

	found, foundSize := readSettings(configHome(), home())
	if !hasTheme && found != "" {
		theme = found
		_ = os.Setenv("XCURSOR_THEME", theme)
	}
	if !hasSize {
		size = foundSize
		if size <= 0 {
			size = defaultSize
		}
		_ = os.Setenv("XCURSOR_SIZE", strconv.Itoa(size))
	}
	return theme, size
}

// readSettings walks the places a desktop records its cursor, most specific
// first. A field is taken from the first source that names it, so a Plasma
// theme survives a stale GTK size and the other way round.
func readSettings(config, home string) (theme string, size int) {
	type source struct {
		path, group, theme, size string
	}
	for _, s := range []source{
		// Plasma. The group is [Mouse] for historical reasons.
		{filepath.Join(config, "kcminputrc"), "Mouse", "cursorTheme", "cursorSize"},
		// GTK 4 and 3, which GNOME writes and Plasma mirrors for GTK apps.
		{filepath.Join(config, "gtk-4.0", "settings.ini"), "Settings",
			"gtk-cursor-theme-name", "gtk-cursor-theme-size"},
		{filepath.Join(config, "gtk-3.0", "settings.ini"), "Settings",
			"gtk-cursor-theme-name", "gtk-cursor-theme-size"},
		// The Xcursor convention: a theme that inherits from the real one.
		{filepath.Join(home, ".icons", "default", "index.theme"), "Icon Theme",
			"Inherits", ""},
		{filepath.Join(home, ".local", "share", "icons", "default", "index.theme"),
			"Icon Theme", "Inherits", ""},
	} {
		vals := readGroup(s.path, s.group)
		if theme == "" {
			theme = strings.TrimSpace(vals[s.theme])
		}
		if size == 0 && s.size != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(vals[s.size])); err == nil && n > 0 {
				size = n
			}
		}
		if theme != "" && size != 0 {
			break
		}
	}
	return theme, size
}

// readGroup returns the key/value pairs of one group of an INI-shaped file.
//
// Every format read here is the same shape: bracketed groups, key=value, # or
// ; for a comment. Not a general INI parser, and it does not need to be.
func readGroup(path, group string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			in = strings.EqualFold(strings.Trim(line, "[]"), group)
			continue
		}
		if !in {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// A localised key - Name[en_GB] - is not the key we asked for.
		if strings.Contains(k, "[") {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func configHome() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	return filepath.Join(home(), ".config")
}

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}
