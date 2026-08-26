// Building a MeshCore checkout into something a node can run.
//
// This lived in cmd/meshbench, which meant `meshbench dev` could build a
// checkout and nothing else could - so a script comparing a stock build
// against a locally changed one had to shell out to another copy of this
// binary. It is a firmware concern, so it lives with the rest of them and the
// command and the verb both call it.
//
// Nothing here compiles anything itself. meshcore-native's build.sh does that,
// and the whole point of MeshBench's claim to run real firmware is that
// MeshCore is compiled as it stands, by its own toolchain, with nothing of
// ours patched into it.
package firmware

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildOptions is one build of one role.
type BuildOptions struct {
	// Source is the top of a MeshCore checkout - the directory holding src/
	// and examples/.
	Source string
	// Role is the application, named as upstream names its example directory:
	// simple_repeater, companion_radio, simple_room_server.
	Role string
	// Label is what to call the result. Empty takes the checkout's git ref,
	// which is what somebody comparing branches wants without asking.
	Label string
	// Log receives the toolchain's own output. Nil discards it - a verb
	// driving this has nowhere to put a compiler's stderr, and swallowing it
	// is better than writing it to the workbench's.
	Log io.Writer
}

// Built is what came out.
type Built struct {
	Label string
	Role  string
	Path  string
	Bytes int64
}

// Build compiles a checkout and puts the result in the cache.
//
// The whole sequence, because the parts are useless separately: find the build
// script, find the Crypto library MeshCore needs, run it, and import what came
// out under a name somebody can pin a node to.
func Build(ctx context.Context, o BuildOptions) (Built, error) {
	src, err := filepath.Abs(o.Source)
	if err != nil {
		return Built{}, err
	}
	if o.Role == "" {
		o.Role = "simple_repeater"
	}
	// Checked before anything is fetched or compiled, because the usual
	// mistake is pointing at src/ or at examples/ rather than at the top, and
	// finding that out after a git clone and a toolchain hunt is a poor way to
	// learn it.
	if _, err := os.Stat(filepath.Join(src, "examples", o.Role)); err != nil {
		return Built{}, fmt.Errorf(
			"%s does not look like a MeshCore checkout: no examples/%s\n\n"+
				"Point at the top of the tree, the directory holding src/ and examples/",
			src, o.Role)
	}
	label := o.Label
	if label == "" {
		label = "local-" + GitRef(src)
	}
	bin, err := buildNative(ctx, src, o.Role, o.Log)
	if err != nil {
		return Built{}, err
	}
	in, err := Import(DefaultCacheDir(), bin, label, o.Role, "")
	if err != nil {
		return Built{}, err
	}
	return Built{Label: label, Role: o.Role, Path: in.Path, Bytes: in.Bytes}, nil
}

// buildNative runs meshcore-native's build.sh and returns what it produced.
func buildNative(ctx context.Context, src, role string, log io.Writer) (string, error) {
	script, err := FindBuildScript()
	if err != nil {
		return "", err
	}
	out, err := os.MkdirTemp("", "meshbench-build-")
	if err != nil {
		return "", err
	}
	crypto, err := FindCrypto(ctx, log)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "bash", script, role, out)
	cmd.Env = append(os.Environ(), "MESHCORE="+src, "CRYPTO="+crypto)
	if log != nil {
		cmd.Stderr, cmd.Stdout = log, log
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}
	bin := filepath.Join(out, NativeBinaryName(role))
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("the build produced no %s", filepath.Base(bin))
	}
	return bin, nil
}

// FindBuildScript locates meshcore-native's build.sh: where MESHCORE_NATIVE
// says, beside the binary, or in the usual checkout.
func FindBuildScript() (string, error) {
	var candidates []string
	if p := os.Getenv("MESHCORE_NATIVE"); p != "" {
		candidates = append(candidates, filepath.Join(p, "build.sh"))
	}
	if self, err := os.Executable(); err == nil {
		candidates = append(candidates,
			filepath.Join(filepath.Dir(self), "meshcore-native", "build.sh"))
	}
	home, _ := os.UserHomeDir()
	candidates = append(candidates,
		filepath.Join(home, "msim", "meshcore-native", "build.sh"),
		filepath.Join(home, "src", "meshcore-native", "build.sh"))
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("cannot find meshcore-native's build.sh\n\n"+
		"Clone https://github.com/MeshBench/meshcore-native and set MESHCORE_NATIVE to it.\n"+
		"Looked in: %s", strings.Join(candidates, ", "))
}

// FindCrypto locates the Crypto library MeshCore builds against, fetching it
// once if it is not already here.
//
// It is one of MeshCore's own lib_deps rather than anything of ours, and
// requiring somebody to clone it by hand before their first build is exactly
// the step this exists to remove.
func FindCrypto(ctx context.Context, log io.Writer) (string, error) {
	if p := os.Getenv("MESHCORE_CRYPTO"); p != "" {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = filepath.Join(home, ".cache")
	}
	deps := filepath.Join(cache, "meshbench", "deps", "arduinolibs")
	for _, c := range []string{
		filepath.Join(home, "msim", "arduinolibs", "libraries", "Crypto"),
		filepath.Join(home, "src", "arduinolibs", "libraries", "Crypto"),
		filepath.Join(deps, "libraries", "Crypto"),
	} {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	if log != nil {
		_, _ = fmt.Fprintln(log, "fetching the Crypto library MeshCore builds against, once")
	}
	if err := os.MkdirAll(filepath.Dir(deps), 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1",
		"https://github.com/rweather/arduinolibs", deps)
	if log != nil {
		cmd.Stderr = log
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not fetch the Crypto library: %w\n\n"+
			"Clone https://github.com/rweather/arduinolibs and set MESHCORE_CRYPTO to "+
			"its libraries/Crypto", err)
	}
	return filepath.Join(deps, "libraries", "Crypto"), nil
}

// GitRef names a checkout by its branch, or its short hash when it is not on
// one, reduced to characters a build label can hold.
func GitRef(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	ref := strings.TrimSpace(string(out))
	if err != nil || ref == "" || ref == "HEAD" {
		out, err = exec.Command("git", "-C", dir, "rev-parse", "--short", "HEAD").Output()
		ref = strings.TrimSpace(string(out))
		if err != nil {
			return "build"
		}
	}
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return '-'
	}, ref)
}
