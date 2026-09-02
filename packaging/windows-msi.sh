#!/usr/bin/env bash
# Build the Windows installer from an assembled bundle, on Linux.
#
#   packaging/windows-msi.sh <bundle-dir> <version> <output.msi>
#
# <bundle-dir> is the directory meshbench.exe sits in, the same one the zip is
# made from, and everything in it goes into the installer. That is the point:
# the emulators and radioserver.exe are found beside the binary, so an
# installer that carried the binary alone would produce a build that cannot
# emulate a board and cannot say why.
#
# No Windows runner. The WiX toolset is a .NET tool and runs here, beside the
# mingw cross-build that produced the .exe.
#
# It was wixl until the installer was asked to say it had finished and to offer
# a folder to install into. wixl builds no dialogs - GNOME/msitools#3 - so
# neither was possible and every answer had to be a switch on the msiexec
# command line. Those switches still work and are still documented; there is
# now also a wizard for the two that people expect to click.
#
# msitools has not gone: msiinfo reads the built package for verify-msi.sh, and
# msibuild puts back the one row WiX will not author. See the ALLUSERS note
# further down.
set -euo pipefail

stage=${1:?the directory meshbench.exe sits in}
version=${2:?the release version}
out=${3:?the .msi to write}

here=$(cd "$(dirname "$0")" && pwd)

missing=""
for t in wix msiinfo msibuild icotool magick python3; do
  command -v "$t" >/dev/null || missing="$missing $t"
done
if [ -n "$missing" ]; then
  echo "::error::windows-msi: missing:$missing - wix is the WiX toolset," \
       "installed with 'dotnet tool install --global wix'; msiinfo and" \
       "msibuild are 'msitools'; icotool is 'icoutils'; magick is ImageMagick" >&2
  exit 1
fi

# The dialogs come from an extension, and a missing one fails inside the build
# with a message about an unknown element rather than about a missing package.
wix extension list -g 2>/dev/null | grep -q WixToolset.UI.wixext || {
  echo "::error::windows-msi: the WiX UI extension is not installed: run" \
       "'wix extension add -g WixToolset.UI.wixext'" >&2
  exit 1
}

# Windows Installer compares three numeric fields and ignores anything after
# them, so a development version has no version to compare and is given the
# lowest one. Every real release is a plain X.Y.Z and passes through.
msiversion=$version
grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' <<<"$version" || msiversion=0.0.0

# The ProductCode identifies this exact version and the UpgradeCode identifies
# MeshBench. Derived from the version rather than drawn fresh each build, so
# rebuilding a version produces the same product: a random one would let the
# same version install twice, side by side, which is the failure an installer
# is supposed to remove rather than introduce.
upgrade=6f2a1c84-7f3b-4c9e-9a6d-0d8f5b1e2c30
product=$(python3 -c "
import uuid, sys
print(uuid.uuid5(uuid.UUID('$upgrade'), sys.argv[1]).urn[9:].upper())
" "$msiversion")

# A scratch directory for the two things built rather than harvested: the icon
# and the dialog bitmaps. Absolute paths are handed to the build, which wixl
# could not take - it fed them to g_file_get_child, which rejected them with a
# GLib assertion and then said it could not find the file, which is why this
# used to symlink both source trees into one directory and name them
# relatively.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
extra=$work/extra
mkdir -p "$extra"
out=$(cd "$(dirname "$out")" && pwd)/$(basename "$out")

# Windows wants one icon file holding every size, the way macOS wants one
# .icns. Built here from the committed PNGs for the same reason macos-app.sh
# builds the .icns there: one set of source images, and no second copy of the
# mark to go stale.
icotool -c -o "$extra/meshbench.ico" \
  "$here/icons/meshbench-16.png" "$here/icons/meshbench-32.png" \
  "$here/icons/meshbench-48.png" "$here/icons/meshbench-64.png" \
  "$here/icons/meshbench-128.png" "$here/icons/meshbench-256.png"

# The two bitmaps the dialogs are drawn on, at the sizes WixUI asks for: the
# dialog fills the left of the first and last pages, the banner runs along the
# top of the ones between. Built from the committed artwork rather than
# committed themselves, for the same reason as the icon - and as bitmaps
# because that is what WixUI takes, which is also why they cannot be the PNGs.
#
# The card is docs/brand's copy, pointed at rather than duplicated here.
#
# The card is 1200x630 and the panel is 493x312, a different shape, so it is
# covered and cropped rather than squashed. The alpha is removed onto the
# brand's own ground: a bitmap has none to lose it to, and what is left
# otherwise is black.
magick "$here/../docs/brand/meshbench-card.png" \
  -background '#0B0A12' -resize 493x312^ -gravity center -extent 493x312 \
  -alpha remove -alpha off -type truecolor BMP3:"$extra/dialog.bmp"
magick -size 493x58 canvas:'#0B0A12' \
  \( "$here/icons/meshbench-64.png" -resize 40x40 \) \
  -gravity west -geometry +14+0 -composite \
  -alpha remove -alpha off -type truecolor BMP3:"$extra/banner.bmp"

sed "s/<version>/$version/" "$here/installed-by-msi.txt" \
  > "$extra/installed-by-msi.txt"

files=$(find "$stage" -type f | wc -l)
echo "windows-msi: $files files from $stage"

# One command, and no harvest step. wixl needed a separate tool to turn the
# tree into components and then a pass over its output to make the ids stable,
# because it named every directory afresh on each run and derived the component
# GUIDs from those names - so the same tree built twice produced two different
# installers. WiX walks the tree itself, in its own order, and does not.
wix build -arch x64 \
  -d "ProductCode=$product" -d "Version=$msiversion" \
  -d "Stage=$(cd "$stage" && pwd)" -d "Extra=$extra" \
  -ext WixToolset.UI.wixext \
  -o "$out" "$here/meshbench.wxs"

# ALLUSERS back to 2, which is the one thing the toolset will not author.
#
# This package has always been dual-purpose: per-machine by default, and
# per-user with MSIINSTALLPERUSER=1, which is what lets somebody without
# administrator rights install into their own profile. WiX 4 removed the
# option - Scope takes perMachine or perUser and nothing else - and emits
# ALLUSERS=1 itself, so the row is corrected here rather than a documented
# install route being dropped to suit a schema. verify-msi.sh checks it.
msibuild "$out" -q "UPDATE \`Property\` SET \`Value\`='2' WHERE \`Property\`='ALLUSERS'"

# Plus the note, which is the one file the installer adds to the bundle.
"$here/verify-msi.sh" "$out" "$msiversion" "$upgrade" "$((files + 1))"
