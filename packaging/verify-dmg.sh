#!/usr/bin/env bash
# Assert that a built disk image installs the way a Mac app installs.
#
#   packaging/verify-dmg.sh <path.dmg>
#
# It mounts the finished file and looks, rather than trusting that the tool
# that wrote it did what it was asked. The failure this exists to stop is the
# quiet one: an image whose Applications alias stopped being generated looks
# exactly like an image that never had one, the job stays green, and the first
# person to notice is a user who ran MeshBench off the mounted volume and lost
# it when they ejected.
#
# The alias is the hard requirement and fails the build. The window layout is
# reported and does not, because it is set by scripting Finder and a runner
# without a GUI session cannot: an image that opens as a file list still
# installs correctly, and refusing to ship one would trade a real release for a
# cosmetic one.
set -uo pipefail

dmg=${1:?the .dmg to check}
[ -f "$dmg" ] || { echo "verify-dmg: no such image: $dmg" >&2; exit 1; }

attach=$(hdiutil attach "$dmg" -readonly -nobrowse -noautoopen) || {
  echo "::error::$dmg does not mount, so nobody can install from it" >&2
  exit 1
}
# hdiutil's columns are tab separated, and the volume name has a space in it.
dev=$(printf '%s\n' "$attach" | grep '^/dev/' | head -1 | cut -f1 | tr -d ' ')
mnt=$(printf '%s\n' "$attach" | grep '^/dev/' | grep '/Volumes/' | head -1 | cut -f3-)

fail=0
note() { echo "verify-dmg: $*"; }
bad() { echo "::error::$dmg $*" >&2; fail=1; }

if [ -z "$mnt" ] || [ ! -d "$mnt" ]; then
  echo "::error::$dmg mounted no volume" >&2
  hdiutil detach "$dev" -quiet 2>/dev/null || true
  exit 1
fi
note "mounted $mnt"

if [ ! -d "$mnt/MeshBench.app" ]; then
  bad "has no MeshBench.app in it"
elif [ ! -x "$mnt/MeshBench.app/Contents/MacOS/meshbench" ]; then
  bad "carries a MeshBench.app with nothing to launch"
fi

# -L before readlink: a real directory named Applications would satisfy a
# plain -e and would copy the whole of /Applications into the image, which is
# the other way this goes wrong.
if [ ! -L "$mnt/Applications" ]; then
  bad "has no Applications symlink, so there is nothing to drag MeshBench onto"
else
  target=$(readlink "$mnt/Applications")
  if [ "$target" != "/Applications" ]; then
    bad "points Applications at '$target' rather than /Applications"
  elif [ ! -d "$mnt/Applications/" ]; then
    bad "has an Applications symlink that resolves to nothing"
  else
    note "Applications -> $target, and it resolves"
  fi
fi

# Finder's record of the window: its size, the icon positions and the ground.
# Absent means the image opens as a plain file list, which installs correctly
# and looks like nobody laid it out.
if [ -f "$mnt/.DS_Store" ]; then
  note "the window layout is set ($(wc -c <"$mnt/.DS_Store" | tr -d ' ') bytes of .DS_Store)"
else
  echo "::warning::$dmg has no .DS_Store, so it opens as a file list rather" \
       "than at a set size with the icons placed. Finder scripting did not run."
fi

hdiutil detach "$dev" -quiet || hdiutil detach "$dev" -force -quiet

if [ "$fail" -ne 0 ]; then
  echo "verify-dmg: $dmg is not shippable" >&2
  exit 1
fi
echo "verify-dmg: $dmg installs the way a Mac app installs"
