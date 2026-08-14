package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// runDev is the firmware development loop as one command.
//
//	meshbench dev -from ~/src/MeshCore
//
// It builds the checkout, hands the result to a running workbench, and assigns
// it. Nothing is added to the MeshCore tree and nothing in it is modified: the
// build happens in a temporary directory and the checkout is read only as far
// as this command is concerned.
func runDev(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dev", flag.ExitOnError)
	from := fs.String("from", ".", "a MeshCore checkout to build")
	role := fs.String("role", "simple_repeater",
		"which application: simple_repeater, companion_radio or simple_room_server")
	name := fs.String("name", "", "what to call the build; the git branch by default")
	watch := fs.Bool("watch", false, "rebuild and reassign whenever a source file changes")
	assign := fs.Bool("assign", true, "assign the build to every node of that role")
	if err := parse(fs, args, "build a MeshCore checkout and give it to the workbench"); err != nil {
		return err
	}

	src, err := filepath.Abs(*from)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(src, "examples", *role)); err != nil {
		return fmt.Errorf("%s does not look like a MeshCore checkout: no examples/%s\n\n"+
			"Point -from at the top of the tree, the directory holding src/ and examples/", src, *role)
	}

	build := func() error {
		label := *name
		if label == "" {
			label = "local-" + gitRef(src)
		}
		bin, err := buildNative(ctx, src, *role)
		if err != nil {
			return err
		}
		in, err := firmware.Import(firmware.DefaultCacheDir(), bin, label, *role, "")
		if err != nil {
			return err
		}
		fmt.Printf("%s  %s  %.1f MB\n", in.Label(), in.Path, float64(in.Bytes)/1e6)

		// Handing it over is best effort: a workbench that is not running is a
		// perfectly normal state, and the build is still in the cache for the
		// next time one starts.
		if err := tell("firmware.import", map[string]any{
			"path": bin, "role": *role, "version": label}); err != nil {
			fmt.Println("  workbench not running, so it is cached but not loaded")
			return nil
		}
		fmt.Println("  in the workbench's firmware library")
		if *assign {
			if err := tell("firmware.set", map[string]any{"role": *role, "version": label}); err == nil {
				fmt.Printf("  assigned to every %s node\n", *role)
			}
		}
		return nil
	}

	if err := build(); err != nil {
		return err
	}
	if !*watch {
		return nil
	}

	fmt.Println("\nwatching for changes; press ctrl-c to stop")
	last := newestSource(src)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(2 * time.Second):
		}
		if n := newestSource(src); n.After(last) {
			last = n
			fmt.Printf("\n%s  change detected, rebuilding\n", time.Now().Format("15:04:05"))
			if err := build(); err != nil {
				fmt.Println("  build failed:", err)
			}
		}
	}
}

// buildNative compiles a checkout for this machine using the published build
// script, into a temporary directory.
func buildNative(ctx context.Context, src, role string) (string, error) {
	script, err := findBuildScript()
	if err != nil {
		return "", err
	}
	out, err := os.MkdirTemp("", "meshbench-build-")
	if err != nil {
		return "", err
	}
	fmt.Printf("building %s from %s\n", role, src)
	crypto, err := findCrypto(ctx)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "bash", script, role, out)
	cmd.Env = append(os.Environ(), "MESHCORE="+src, "CRYPTO="+crypto)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build failed: %w", err)
	}
	bin := filepath.Join(out, firmware.NativeBinaryName(role))
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("the build produced no %s", filepath.Base(bin))
	}
	return bin, nil
}

// findBuildScript locates meshcore-native's build.sh: beside the binary, in the
// usual checkout, or wherever MESHCORE_NATIVE says.
func findBuildScript() (string, error) {
	var candidates []string
	if p := os.Getenv("MESHCORE_NATIVE"); p != "" {
		candidates = append(candidates, filepath.Join(p, "build.sh"))
	}
	if self, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(self), "meshcore-native", "build.sh"))
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

// findCrypto locates the Crypto library MeshCore builds against, and fetches it
// if it is not already here.
//
// It is one of MeshCore's own lib_deps rather than anything of ours, and
// requiring somebody to clone it by hand before their first build is exactly
// the kind of step this command exists to remove.
func findCrypto(ctx context.Context) (string, error) {
	if p := os.Getenv("MESHCORE_CRYPTO"); p != "" {
		return p, nil
	}
	home, _ := os.UserHomeDir()
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = filepath.Join(home, ".cache")
	}
	deps := filepath.Join(cache, "meshcoresim", "deps", "arduinolibs")
	for _, c := range []string{
		filepath.Join(home, "msim", "arduinolibs", "libraries", "Crypto"),
		filepath.Join(home, "src", "arduinolibs", "libraries", "Crypto"),
		filepath.Join(deps, "libraries", "Crypto"),
	} {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	fmt.Println("fetching the Crypto library MeshCore builds against, once")
	if err := os.MkdirAll(filepath.Dir(deps), 0o755); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1",
		"https://github.com/rweather/arduinolibs", deps)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not fetch the Crypto library: %w\n\n"+
			"Clone https://github.com/rweather/arduinolibs and set MESHCORE_CRYPTO to "+
			"its libraries/Crypto", err)
	}
	return filepath.Join(deps, "libraries", "Crypto"), nil
}

// newestSource is the most recent modification time under a checkout's own
// sources, which is enough to notice an edit without watching every file.
func newestSource(root string) time.Time {
	var newest time.Time
	for _, dir := range []string{"src", "examples"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() {
				return nil
			}
			switch filepath.Ext(p) {
			case ".cpp", ".h", ".hpp", ".c":
				if fi.ModTime().After(newest) {
					newest = fi.ModTime()
				}
			}
			return nil
		})
	}
	return newest
}

func gitRef(dir string) string {
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
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			return r
		}
		return '-'
	}, ref)
}

// tell sends one verb to a running workbench.
func tell(method string, params map[string]any) error {
	sock := filepath.Join(runtimeDir(), "meshcoresim.sock")
	c, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	body, _ := json.Marshal(map[string]any{"id": 1, "method": method, "params": params})
	if _, err := c.Write(append(body, '\n')); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return err
	}
	var reply struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal([]byte(line), &reply)
	if reply.Error != "" {
		return fmt.Errorf("%s", reply.Error)
	}
	return nil
}

func runtimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return fmt.Sprintf("/run/user/%d", os.Getuid())
}
