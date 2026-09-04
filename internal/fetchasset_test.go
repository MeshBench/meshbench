// The release fetcher's contract, enforced.
//
// tools/fetch-release-asset.sh is the one thing standing between a fork's
// release page and what a user downloads, and its exit codes are the whole of
// the packaging pipeline's judgement about whether a bundle is shippable. They
// were one code until a fork's newest release renamed every asset and dropped
// every platform but Linux: every pattern stopped matching, every caller's
// "|| warn" swallowed it, and every platform shipped without QEMU under a green
// pipeline.
//
// So the two states are pinned here rather than described in the script's
// comment. Exercised against a stub, because GitHub cannot be asked to answer
// 404 on demand and a test that needs the network is a test that gets skipped.
//
// It lives beside the layer and layout checks because it is the same kind of
// thing: a rule about the repository, failing on the machine of whoever broke
// it rather than eight steps into a release.
package internal_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// releaseJSON is the shape the script parses: the browser_download_url lines
// and nothing else, because that is all it reads.
func releaseJSON(base string, assets ...string) string {
	var b strings.Builder
	b.WriteString(`{"tag_name":"t","assets":[`)
	for i, a := range assets {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"name":%q,"browser_download_url":"%s/dl/%s"}`, a, base, a)
	}
	b.WriteString(`]}`)
	return b.String()
}

// stubAPI answers the three ways a release can be, at three repository names.
func stubAPI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/present/releases/tags/pinned", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, releaseJSON(srv.URL, "tool-linux-amd64.tar.gz", "tool-src.tar.gz"))
	})
	// The state this test exists for: a real release carrying names nobody
	// here expects, which is what a fork renaming its assets looks like.
	mux.HandleFunc("/repos/o/renamed/releases/tags/pinned", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, releaseJSON(srv.URL, "something-else-entirely.tar.gz"))
	})
	mux.HandleFunc("/repos/o/absent/releases/tags/pinned", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	mux.HandleFunc("/repos/o/unreachable/releases/tags/pinned", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "the bytes of %s", filepath.Base(r.URL.Path))
	})
	srv.Config.Handler = mux
	t.Cleanup(srv.Close)
	return srv
}

// run executes one of the packaging scripts and reports its exit code and what
// it said.
func run(t *testing.T, apiRoot, script string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "GITHUB_API_URL="+apiRoot, "GH_TOKEN=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("running %s: %v", script, err)
	}
	return ee.ExitCode(), string(out)
}

func fetch(t *testing.T, apiRoot, repo, pattern, dir string) (int, string) {
	t.Helper()
	return run(t, apiRoot, "../tools/fetch-release-asset.sh", repo, "pinned", pattern, dir)
}

func TestFetchReleaseAsset(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this machine")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl on this machine")
	}
	srv := stubAPI(t)

	t.Run("a release that carries the asset", func(t *testing.T) {
		dir := t.TempDir()
		code, out := fetch(t, srv.URL, "o/present", `/tool-linux-amd64\.tar\.gz$`, dir)
		if code != 0 {
			t.Fatalf("exit %d, want 0:\n%s", code, out)
		}
		if _, err := os.Stat(filepath.Join(dir, "tool-linux-amd64.tar.gz")); err != nil {
			t.Errorf("the asset is not in the directory: %v", err)
		}
	})

	// The regression. A release that exists and matches nothing means the fork
	// renamed its assets or the pin is wrong, and a bundle assembled past it
	// has no emulator in it.
	t.Run("a release that carries something else", func(t *testing.T) {
		code, out := fetch(t, srv.URL, "o/renamed", `/tool-linux-amd64\.tar\.gz$`, t.TempDir())
		if code != 3 {
			t.Fatalf("exit %d, want 3 so the caller can fail:\n%s", code, out)
		}
		if !strings.Contains(out, "something-else-entirely.tar.gz") {
			t.Errorf("the error does not say what the release does carry:\n%s", out)
		}
	})

	// The state that may legitimately be warned past: a release nobody has cut.
	t.Run("no release at all", func(t *testing.T) {
		code, out := fetch(t, srv.URL, "o/absent", `/tool-linux-amd64\.tar\.gz$`, t.TempDir())
		if code != 2 {
			t.Fatalf("exit %d, want 2 so the caller can warn:\n%s", code, out)
		}
	})

	// Not knowing is not the same as knowing there is nothing, and a runner
	// that cannot reach GitHub must never be read as an empty release.
	t.Run("the API could not be asked", func(t *testing.T) {
		code, out := fetch(t, srv.URL, "o/unreachable", `/tool-linux-amd64\.tar\.gz$`, t.TempDir())
		if code != 1 {
			t.Fatalf("exit %d, want 1:\n%s", code, out)
		}
	})
}

// TestFetchEmulatorTellsTheTwoStatesApart is the policy the pipeline runs on,
// one layer up: which of the fetcher's exit codes a bundle may be assembled
// past, and which one stops the job.
func TestFetchEmulatorTellsTheTwoStatesApart(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this machine")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("no curl on this machine")
	}
	srv := stubAPI(t)
	const script = "../packaging/fetch-emulator.sh"

	// The regression, at the level the workflow calls: a release whose assets
	// have been renamed must stop the job, not warn past it.
	t.Run("a release that carries something else fails the job", func(t *testing.T) {
		code, out := run(t, srv.URL, script,
			"o/renamed", "pinned", "tool-linux-amd64.tar.gz", "QEMU", t.TempDir())
		if code == 0 {
			t.Fatalf("exit 0, so a bundle with no QEMU in it would be published:\n%s", out)
		}
		if !strings.Contains(out, "::error::") {
			t.Errorf("no error annotation, so nobody reading the run would see it:\n%s", out)
		}
	})

	t.Run("no release at all still only warns", func(t *testing.T) {
		code, out := run(t, srv.URL, script,
			"o/absent", "pinned", "tool-linux-amd64.tar.gz", "QEMU", t.TempDir())
		if code != 0 {
			t.Fatalf("exit %d, want 0: a release nobody has cut yet is a known state:\n%s", code, out)
		}
		if !strings.Contains(out, "::warning::") {
			t.Errorf("a state worth carrying on past is still worth saying out loud:\n%s", out)
		}
	})

	// A platform the forks publish nothing for is neither of those: it is the
	// bundle being honestly without an emulator.
	t.Run("a platform with no build says so", func(t *testing.T) {
		code, out := run(t, srv.URL, script, "o/present", "pinned", "", "QEMU", t.TempDir())
		if code != 0 || !strings.Contains(out, "::notice::") {
			t.Fatalf("exit %d, want 0 and a notice:\n%s", code, out)
		}
	})
}

// TestVerifyBundleRefusesABundleWithoutItsEmulators is the last gate before a
// tarball is published: a bundle that quietly lacks an emulator is the failure
// mode being designed out, and 0.0.1 through 0.0.3 shipped one.
func TestVerifyBundleRefusesABundleWithoutItsEmulators(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this machine")
	}
	const script = "../packaging/verify-bundle.sh"

	empty := t.TempDir()
	code, out := run(t, "", script, empty, "linux-amd64")
	if code == 0 {
		t.Fatalf("an empty bundle passed:\n%s", out)
	}
	if !strings.Contains(out, "qemu-system-xtensa") || !strings.Contains(out, "renode") {
		t.Errorf("the failure does not name what is missing:\n%s", out)
	}

	// The same directory once it holds what a node would look for, in the
	// layouts the emulators actually unpack into.
	full := t.TempDir()
	for _, name := range []string{
		"libvirtualsx1262.so",
		"qemu-meshbench/bin/qemu-system-xtensa",
		"renode_1.16.1-portable/renode",
		// Renode reads these at runtime, so a bundle without them can start an
		// ESP32 board and not an nRF52 one.
		"renode-support/peripherals/VirtualSX1262.cs",
	} {
		p := filepath.Join(full, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The tree says what it is, and what is required of it follows from that.
	if err := os.WriteFile(filepath.Join(full, "VARIANT"), []byte("bundled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, "", script, full, "linux-amd64"); code != 0 {
		t.Fatalf("a complete bundle was refused, exit %d:\n%s", code, out)
	}
}

// A compact bundle is meant to carry no emulators, so holding it to the bundled
// list would refuse a correct artifact - and a compact one that *does* carry
// them is a packaging mistake worth catching, because it would ship as the
// small download at the large size.
func TestVerifyBundleJudgesEachVariantByItsOwnRules(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash on this machine")
	}
	const script = "../packaging/verify-bundle.sh"

	compact := t.TempDir()
	for _, name := range []string{
		"libvirtualsx1262.so",
		"renode-support/peripherals/VirtualSX1262.cs",
	} {
		p := filepath.Join(compact, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(compact, "VARIANT"), []byte("compact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := run(t, "", script, compact, "linux-amd64"); code != 0 {
		t.Fatalf("a correct compact bundle was refused, exit %d:\n%s", code, out)
	}

	// Now the same tree with an emulator in it, which it should not have.
	p := filepath.Join(compact, "qemu-system-xtensa")
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, out := run(t, "", script, compact, "linux-amd64")
	if code == 0 {
		t.Fatal("a compact bundle carrying an emulator was accepted")
	}
	if !strings.Contains(out, "qemu-system-xtensa") {
		t.Errorf("the refusal does not name what it found:\n%s", out)
	}
}
