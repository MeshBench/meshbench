//go:build linux || freebsd || openbsd || netbsd

package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

// write puts a file at a relative path under dir, making parents as needed.
func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The case that produced the bug: Plasma names a theme and no size, because
// the size is at its default and Plasma only writes what was changed. The
// theme must come from Plasma and the size from GTK, which mirrors it.
func TestPlasmaThemeAndGTKSize(t *testing.T) {
	d := t.TempDir()
	write(t, d, ".config/kcminputrc", "[Mouse]\ncursorTheme=capitaine-cursors\n")
	write(t, d, ".config/gtk-3.0/settings.ini",
		"[Settings]\ngtk-cursor-blink=true\ngtk-cursor-theme-name=capitaine-cursors\ngtk-cursor-theme-size=24\n")

	theme, size := readSettings(filepath.Join(d, ".config"), d)
	if theme != "capitaine-cursors" || size != 24 {
		t.Fatalf("got %q/%d, want capitaine-cursors/24", theme, size)
	}
}

// A size Plasma did write beats the GTK mirror, which goes stale.
func TestPlasmaSizeWins(t *testing.T) {
	d := t.TempDir()
	write(t, d, ".config/kcminputrc", "[Mouse]\ncursorTheme=breeze_cursors\ncursorSize=48\n")
	write(t, d, ".config/gtk-3.0/settings.ini", "[Settings]\ngtk-cursor-theme-size=24\n")

	theme, size := readSettings(filepath.Join(d, ".config"), d)
	if theme != "breeze_cursors" || size != 48 {
		t.Fatalf("got %q/%d, want breeze_cursors/48", theme, size)
	}
}

// GTK 4 is read before GTK 3, so a migrated desktop is not read backwards.
func TestGTK4BeatsGTK3(t *testing.T) {
	d := t.TempDir()
	write(t, d, ".config/gtk-4.0/settings.ini",
		"[Settings]\ngtk-cursor-theme-name=Adwaita\ngtk-cursor-theme-size=32\n")
	write(t, d, ".config/gtk-3.0/settings.ini",
		"[Settings]\ngtk-cursor-theme-name=old\ngtk-cursor-theme-size=16\n")

	theme, size := readSettings(filepath.Join(d, ".config"), d)
	if theme != "Adwaita" || size != 32 {
		t.Fatalf("got %q/%d, want Adwaita/32", theme, size)
	}
}

// The Xcursor convention, used by desktops that write no config of their own.
func TestInheritsFromIconTheme(t *testing.T) {
	d := t.TempDir()
	write(t, d, ".icons/default/index.theme",
		"# a comment\n[Icon Theme]\nName=Default\nName[en_GB]=Default\nInherits=Bibata-Original-Ice\n")

	theme, size := readSettings(filepath.Join(d, ".config"), d)
	if theme != "Bibata-Original-Ice" {
		t.Fatalf("got theme %q, want Bibata-Original-Ice", theme)
	}
	if size != 0 {
		t.Fatalf("got size %d, want 0 - an icon theme states no size", size)
	}
}

// A key outside the group it belongs to is not the key we asked for. This is
// the failure that would silently read a blink rate as a cursor size.
func TestKeysOutsideTheGroupAreIgnored(t *testing.T) {
	d := t.TempDir()
	write(t, d, ".config/kcminputrc",
		"[Keyboard]\ncursorSize=99\n\n[Mouse]\ncursorTheme=breeze_cursors\n")

	theme, size := readSettings(filepath.Join(d, ".config"), d)
	if theme != "breeze_cursors" {
		t.Fatalf("got theme %q", theme)
	}
	if size != 0 {
		t.Fatalf("read %d out of the wrong group", size)
	}
}

// Nothing configured anywhere: the caller gets the desktop default, not Gio's
// 32, which is the whole point of the exercise.
func TestBareSystemGetsTheCommonDefault(t *testing.T) {
	d := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(d, ".config"))
	t.Setenv("HOME", d)
	os.Unsetenv("XCURSOR_THEME")
	os.Unsetenv("XCURSOR_SIZE")

	if _, size := MatchSystemCursor(); size != defaultSize {
		t.Fatalf("got %d, want %d", size, defaultSize)
	}
	if got := os.Getenv("XCURSOR_SIZE"); got != "24" {
		t.Fatalf("XCURSOR_SIZE is %q, so Gio will not see it", got)
	}
}

// Somebody who exported the variables meant them.
func TestEnvironmentIsNotOverridden(t *testing.T) {
	d := t.TempDir()
	write(t, d, ".config/kcminputrc", "[Mouse]\ncursorTheme=capitaine-cursors\ncursorSize=24\n")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(d, ".config"))
	t.Setenv("HOME", d)
	t.Setenv("XCURSOR_THEME", "Bibata")
	t.Setenv("XCURSOR_SIZE", "48")

	theme, size := MatchSystemCursor()
	if theme != "Bibata" || size != 48 {
		t.Fatalf("got %q/%d, want Bibata/48", theme, size)
	}
}
