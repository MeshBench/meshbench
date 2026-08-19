package firmware

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// CachedRoles lists the roles a downloaded version has a runnable build for.
//
// Exists so a mistyped role fails where it was typed rather than where it is
// used. Setting a node to role "companion" - which is the node's *kind*, not the
// name of any MeshCore application - wrote through happily, and only surfaced
// four minutes later as "AngusOutlaw1 runs no firmware" partway into a
// forty-eight run sweep. The build is called meshcore-companion_radio-…, and
// nothing between the two said so.
//
// Cache only: this answers "can this run right now", not "does it exist
// somewhere", so it never reaches the network to answer a validation question.
func CachedRoles(cacheDir, version string) []string {
	if cacheDir == "" || version == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(cacheDir, "native", version))
	if err != nil {
		return nil
	}
	suffix := fmt.Sprintf("-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		suffix += ".exe"
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "meshcore-") || !strings.HasSuffix(name, suffix) {
			continue
		}
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(name, "meshcore-"), suffix))
	}
	sort.Strings(out)
	return out
}

// HasCachedBuild reports whether (role, version) can run from the cache.
func HasCachedBuild(cacheDir, role, version string) bool {
	if role == "" {
		role = DefaultRole
	}
	for _, r := range CachedRoles(cacheDir, version) {
		if r == role {
			return true
		}
	}
	return false
}
