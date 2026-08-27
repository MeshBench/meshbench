package session_test

// Blank-imports the domain packages split out of session, so the session test
// binary registers their verbs from init. The full-surface tests here - the
// verb manifest and the parity checks - then see every verb, not only the core
// ones, exactly as a running binary does once it imports the domains.
import (
	_ "github.com/MeshBench/meshbench/internal/app/session/study"
)
