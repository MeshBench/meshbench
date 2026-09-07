package shell

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/version"
)

// A development build looks exactly like a release in a screenshot, and a
// screenshot is what reaches an issue.
func TestADevelopmentBuildSaysSoInTheStatusBar(t *testing.T) {
	was := version.Version
	defer func() { version.Version = was }()

	version.Version = "v0.0.11-dev.3"
	if got := versionMark(); !strings.Contains(got, "development build") {
		t.Errorf("a development build's mark is %q", got)
	}
	version.Version = "v0.0.11"
	if got := versionMark(); strings.Contains(got, "development") {
		t.Errorf("a stable build's mark is %q", got)
	}
	// A working copy names its commit already, and is not a release of any
	// channel to be marked as one.
	version.Version = ""
	if got := versionMark(); strings.Contains(got, "development build") {
		t.Errorf("a working copy's mark is %q", got)
	}
}
