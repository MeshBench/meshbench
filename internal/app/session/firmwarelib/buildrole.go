package firmwarelib

import (
	"sort"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/firmware"
)

// assetRoles is what the published catalogue's short names mean, the same
// normalisation an import applies to a feed's role names: a build offered as
// repeater-v1.17.1 is simple_repeater's.
var assetRoles = map[string]string{
	"repeater":    "simple_repeater",
	"companion":   "companion_radio",
	"room-server": "simple_room_server",
}

// rolesFor is the set of roles a version is a build for, or empty when that
// cannot be known.
//
// Three places know: the cache, for a build on disk; the catalogue's own
// cache, for one published and not yet fetched; and the name itself, which
// the catalogue spells role-first. A local label like main or a developer's
// branch name answers to none of them, and a pin nobody can check is made as
// it always was, so an unknown build is never refused for being unknown.
func rolesFor(s *session.Sim, version string) map[string]bool {
	out := map[string]bool{}
	for _, b := range firmware.ListInstalled(firmware.DefaultCacheDir()) {
		if b.Version == version {
			out[baseRole(b.Role)] = true
		}
	}
	for _, b := range publishedBuilds(s) {
		if b.version == version {
			out[baseRole(b.role)] = true
		}
	}
	if len(out) == 0 {
		if r, _ := firmware.RoleVersionFromImageName(version); assetRoles[r] != "" {
			out[assetRoles[r]] = true
		}
	}
	return out
}

// baseRole is a board image's role without its transport: companion_radio_usb
// is companion_radio's build for that board.
func baseRole(role string) string {
	for _, t := range []string{"_usb", "_ble"} {
		role = strings.TrimSuffix(role, t)
	}
	return role
}

func joinRoles(roles map[string]bool) string {
	names := make([]string, 0, len(roles))
	for r := range roles {
		names = append(names, r)
	}
	sort.Strings(names)
	return strings.Join(names, " or ")
}
