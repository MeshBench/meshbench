package control

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isPrivate checks the guarantee the way Windows expresses it.
//
// Not mode bits: Go synthesises those from the read-only attribute, so every
// file on this platform reads as 0666 and a test asserting 0600 fails while
// the file is in fact private. That failure looks exactly like a security
// hole and is not one, which is the worst way for a test to be wrong.
//
// What actually protects these files is the discretionary ACL they inherit
// from a per-user directory, so that is what is read: no entry may grant any
// access to a group everybody is in. Everyone (S-1-1-0) and the local Users
// group (S-1-5-32-545) are the two that would matter.
func isPrivate(t *testing.T, path string, _ fs.FileMode) {
	t.Helper()
	if err := privacyProblem(path); err != nil {
		t.Error(err)
	}
}

// privacyProblem returns what is wrong with the ACL, or nil.
func privacyProblem(path string) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("reading the ACL of %s: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("no DACL on %s: %w", path, err)
	}
	if dacl == nil {
		return fmt.Errorf("%s has a null DACL, which grants everyone everything", path)
	}
	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return err
	}
	users, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	if err != nil {
		return err
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return fmt.Errorf("reading ACE %d of %s: %w", i, path, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(everyone) || sid.Equals(users) {
			return fmt.Errorf("%s grants access to %s, so it is not private", path, sid)
		}
	}
	return nil
}

// A check that cannot fail is not a check. Grant a group everybody is in and
// confirm the ACL reader objects, so the passes above mean something.
func TestThePrivacyCheckCatchesAPublicFile(t *testing.T) {
	path := filepath.Join(shortSocketDir(t), "public.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := privacyProblem(path); err != nil {
		t.Fatalf("a fresh file under the user's own temp is already public: %v", err)
	}
	out, err := exec.Command("icacls", path, "/grant", "*S-1-1-0:(R)").CombinedOutput()
	if err != nil {
		t.Skipf("could not grant Everyone with icacls, so this cannot be proved here: %v: %s", err, out)
	}
	if err := privacyProblem(path); err == nil {
		t.Error("a file granting Everyone read was reported as private")
	}
}
