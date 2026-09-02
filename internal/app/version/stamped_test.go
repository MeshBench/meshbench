package version

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The release pipeline's build paths, which are what the linker stamp above
// only ever reaches. Three platforms, three commands, in two files: two jobs in
// the workflow, and a script the macOS job shells out to because the Mac that
// runs it is ours and a person may want to run the same thing by hand.
var buildPaths = []string{
	"../../../.github/workflows/package.yml",
	"../../../packaging/macos-app.sh",
}

// The stamp itself: the release's shell variable, with the tag's leading v put
// back. The v matters because String promises a release "says its tag", and a
// binary reporting 0.1.0 for the tag v0.1.0 disagrees with the release page,
// the source archive and both client indexes at once.
var stamp = regexp.MustCompile(
	`internal/app/version\.Version=(v\$\{?[A-Za-z_][A-Za-z0-9_]*\}?)`)

// Every platform is built by a different job, so the only thing keeping the
// three stamps in agreement is that somebody remembered - and once they
// disagree the difference is invisible until a release is already out, because
// each build works perfectly and says a different thing about itself.
func TestEveryReleaseBuildStampsTheVersion(t *testing.T) {
	found := 0
	for _, path := range buildPaths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, cmd := range meshbenchBuilds(string(b)) {
			found++
			m := stamp.FindStringSubmatch(cmd)
			if m == nil {
				t.Errorf("%s builds cmd/meshbench without stamping "+
					"internal/app/version.Version=v$VERSION:\n%s", path, cmd)
				continue
			}
			if !strings.HasPrefix(m[1], "v$") {
				t.Errorf("%s stamps %q, which drops the tag's v", path, m[1])
			}
		}
	}
	if found != 3 {
		t.Errorf("found %d builds of cmd/meshbench across %v, want the three "+
			"released platforms - a new one needs its own stamp", found,
			buildPaths)
	}
}

// meshbenchBuilds returns the text of each `go build ... ./cmd/meshbench`,
// which the pipeline writes across several continued lines.
func meshbenchBuilds(text string) []string {
	var out []string
	for rest := text; ; {
		i := strings.Index(rest, "./cmd/meshbench")
		if i < 0 {
			return out
		}
		cmd := rest[:i]
		rest = rest[i+len("./cmd/meshbench"):]
		j := strings.LastIndex(cmd, "go build")
		if j < 0 {
			continue
		}
		out = append(out, cmd[j:])
	}
}
