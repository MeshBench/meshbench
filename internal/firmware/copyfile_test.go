package firmware

import (
	"os"
	"path/filepath"
	"testing"
)

// A copy whose source and destination are the same file has nothing to
// move, and O_TRUNC on the destination truncates the source before a byte is
// read: the file ends at zero length and copyFile still reports success. This
// is what cmd_dev hit whenever a running workbench turned firmware.Build's own
// cache path back into firmware.import's destination.
func TestCopyFileDoesNotTruncateItsOwnSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meshcore-simple_repeater-linux-amd64")
	want := []byte("a built firmware binary, not actually one")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(path, path, true); err != nil {
		t.Fatalf("copying a file onto itself returned an error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("content after a same-file copy: got %d bytes, want %d bytes (%q)",
			len(got), len(want), got)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0o111 == 0 {
		t.Error("an executable same-file copy left the file without its execute bit")
	}
}

// The same identity check by a relative path, since a truncation bug hiding
// behind string comparison would still trigger here: "./x" and "x" name one
// file and do not read as equal strings.
func TestCopyFileDoesNotTruncateItsOwnSourceByARelativePath(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "build")
	want := []byte("built again")
	if err := os.WriteFile(abs, want, 0o644); err != nil {
		t.Fatal(err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	if err := copyFile("build", "./build", false); err != nil {
		t.Fatalf("copying via a relative path onto itself returned an error: %v", err)
	}
	got, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("content after a same-file copy by relative path: got %q, want %q", got, want)
	}
}

// The ordinary case: two different files, one of them starting out absent.
// The guard must not turn a real copy into a no-op.
func TestCopyFileStillCopiesTwoDifferentFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	want := []byte("the real content")
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("copied content: got %q, want %q", got, want)
	}
	// The source is untouched: only the destination is meant to end up at
	// O_TRUNC's mercy.
	srcGot, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(srcGot) != string(want) {
		t.Fatalf("source content after copying elsewhere: got %q, want %q", srcGot, want)
	}
}
