package main

import "testing"

// A same-file copy inside the workbench can truncate a build and still
// answer firmware.import with no error: the size in the reply is the only
// thing that says whether the file that landed matches the one that was
// built. importedBytes is what cmd_dev reads to tell the two apart, so
// "the workbench says nothing went wrong" stops being read as "it worked".
func TestImportedBytesReadsWhatTheWorkbenchReports(t *testing.T) {
	got, ok := importedBytes([]byte(`{"version":"local-x","role":"simple_repeater","bytes":583768}`))
	if !ok {
		t.Fatal("a well formed reply was not read")
	}
	if got != 583768 {
		t.Errorf("got %d bytes, want 583768", got)
	}
}

// The zero-byte reply this bug actually produced: no error from the call,
// and a size firmware.import genuinely computed from the file left on disk
// after a same-file truncation. Reading it is what makes cmd_dev able to
// tell the difference, rather than printing the success line regardless.
func TestImportedBytesCatchesATruncatedReply(t *testing.T) {
	got, ok := importedBytes([]byte(`{"version":"local-x","role":"simple_repeater","bytes":0}`))
	if !ok {
		t.Fatal("a well formed reply was not read")
	}
	if got != 0 {
		t.Errorf("got %d bytes, want 0", got)
	}
	// Reading succeeds; it is the caller's comparison against what was built
	// (583768 != 0) that turns this into a reported failure rather than a
	// silent one. That comparison lives in runDev's build closure, which
	// needs a running control socket to exercise end to end.
}

func TestImportedBytesRejectsUnparseableReplies(t *testing.T) {
	if _, ok := importedBytes([]byte(`not json`)); ok {
		t.Error("unparseable reply was accepted")
	}
}
