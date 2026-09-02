#!/usr/bin/env bash
# Build the disk image a Mac user already knows how to use.
#
#   packaging/macos-dmg.sh <version> <src-dir> <out.dmg>
#
# <src-dir> is the directory MeshBench.app sits in, and it is the image's whole
# contents. A plain `hdiutil create` over that directory produces a window with
# one bundle in it and nothing to drag it onto, so people run MeshBench from
# the mounted image: it works until the image is ejected, and until then macOS
# treats it differently for quarantine and for anything written beside the
# binary. Nobody can reproduce the failures that follow, because the user does
# not know they did anything unusual.
#
# So the image carries an alias to /Applications beside the app, and opens at a
# fixed size with the two icons where the gesture expects them.
#
# hdiutil and osascript only, both part of macOS. create-dmg is the usual
# answer and does exactly this dance, but it is a Homebrew install, and a
# release job that dies on a missing tool is worse than a plain image.
#
# The window geometry is the one part that can fail: it is set by scripting
# Finder, and a runner with no GUI session, or one that has not granted
# automation access, cannot. That is a warning and not an error, because the
# alias is what makes the image installable and the alias needs no Finder.
#
# Whether the alias really made it onto the finished image is asserted by
# packaging/verify-dmg.sh, which this script runs on its own output rather than
# leaving to the caller. A dmg whose alias silently stopped being generated is
# indistinguishable from one that never had it, and a check the producer skips
# is a check somebody rewrites the producer without.
set -euo pipefail

VER=${1:?version, e.g. 0.1.0}
SRC=${2:?the directory MeshBench.app sits in}
DMG=${3:?the .dmg to write}

VOL="MeshBench $VER"
[ -d "$SRC/MeshBench.app" ] || { echo "macos-dmg: no MeshBench.app in $SRC" >&2; exit 1; }

# In place rather than into a staging copy: the bundle carries both emulators
# by the time this runs, and duplicating 400 MB to add a symlink to it is a
# minute of a release for nothing.
ln -sfn /Applications "$SRC/Applications"

# hdiutil sizes an image to its contents, which leaves no room for the
# .DS_Store Finder writes next - the layout below then fails on a full volume,
# and the image still mounts and still looks unstyled.
kb=$(du -sk "$SRC" | awk '{print $1}')
work=$(mktemp -d)
rw=$work/meshbench-rw.dmg
hdiutil create -quiet -srcfolder "$SRC" -volname "$VOL" -fs HFS+ \
  -format UDRW -size "$((kb + 51200))k" -ov "$rw"

# Finder can only be told about a volume it can see, so this one mounts under
# /Volumes like any other. -noautoopen keeps a window off the runner's screen.
attach=$(hdiutil attach "$rw" -readwrite -noverify -noautoopen)
# hdiutil's columns are tab separated, and the volume name has a space in it.
dev=$(printf '%s\n' "$attach" | grep '^/dev/' | head -1 | cut -f1 | tr -d ' ')
mnt=$(printf '%s\n' "$attach" | grep '^/dev/' | grep '/Volumes/' | head -1 | cut -f3-)
[ -n "$mnt" ] && [ -d "$mnt" ] || { echo "macos-dmg: the image did not mount" >&2; exit 1; }

detach() {
  # A just-written volume is often still busy. Give it a few goes before
  # forcing, because a forced detach can lose the .DS_Store that was the point.
  local i
  for i in 1 2 3; do
    hdiutil detach "$dev" -quiet && return 0
    sleep 2
  done
  hdiutil detach "$dev" -force -quiet
}

echo "macos-dmg: laying out $VOL"
# 740 by 480, with the app under the left third and the alias under the right,
# which is where every Mac installer has put them for twenty years. The ground
# is the brand's mist rather than an image: the identity lives in a private
# repository the release path must not reach into, and a second drawing of it
# committed here would be the copy that goes stale.
cat > "$work/layout.scpt" <<'APPLESCRIPT'
on run argv
  set volName to item 1 of argv
  with timeout of 60 seconds
    tell application "Finder"
      tell disk volName
        open
        set current view of container window to icon view
        set toolbar visible of container window to false
        set statusbar visible of container window to false
        set bounds of container window to {200, 140, 940, 620}
        set opts to icon view options of container window
        set arrangement of opts to not arranged
        set icon size of opts to 128
        set text size of opts to 13
        set background color of opts to {62965, 62451, 64250}
        set position of item "MeshBench.app" of container window to {180, 210}
        set position of item "Applications" of container window to {560, 210}
        close
        open
        update without registering applications
        delay 2
      end tell
    end tell
  end timeout
end run
APPLESCRIPT

# Watchdogged, because the ways Finder scripting goes wrong are not all
# failures. AppleScript's own timeout covers an event Finder is slow to answer;
# it does not cover an event that is never delivered, which is what a runner
# waiting on an automation-permission prompt nobody can click looks like. A
# release that hangs for the job's full 75 minutes is worse than one that
# ships a plainer image, so the layout gets two minutes and no more.
osascript "$work/layout.scpt" "$VOL" & osa=$!
waited=0
while kill -0 "$osa" 2>/dev/null && [ "$waited" -lt 120 ]; do
  sleep 1
  waited=$((waited + 1))
done
kill -9 "$osa" 2>/dev/null || true
# wait still reports the status of a job that has already ended, so this is one
# answer for all three endings: laid out, refused, or killed by the count above.
if wait "$osa"; then
  laid_out=yes
else
  laid_out=no
fi

if [ "$laid_out" = no ]; then
  echo "::warning::Finder would not set the dmg window layout, so the image" \
       "opens as a file list. The Applications alias is still there; this" \
       "runner needs a GUI session and automation access to Finder."
fi

# The .DS_Store Finder just wrote has to reach the disk before the volume goes
# away, or the layout is lost and the image still mounts and still looks
# unstyled. Nothing here chmods the tree: the bundle is signed by this point,
# and the shipped image is read-only anyway.
sync
detach

hdiutil convert "$rw" -quiet -format UDZO -imagekey zlib-level=9 -ov -o "$DMG"
rm -rf "$work"

"$(cd "$(dirname "$0")" && pwd)/verify-dmg.sh" "$DMG"
