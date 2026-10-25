#!/usr/bin/env bash
# Get a Windows installer ready to build: everything the WiX step needs that is
# made from the repository rather than harvested from the staged tree.
#
#   packaging/windows-msi.sh <stage-dir> <version> <extra-dir>
#
# The installer itself is built by the WiX toolset, on Windows, by
# packaging/windows-msi.ps1. WiX cannot build an MSI anywhere else: it needs
# Windows' own msi.dll to write the database, and on Linux it dies on
# "Unable to load shared library 'msi.dll'" after refusing every
# Directory/@Name (#508). wixl, which does run here, builds no dialogs. So the
# work is split: this half runs on Linux with the tools that only Linux has
# (icotool, Pillow), the build runs on a Windows runner, and verify-msi.sh
# reads the result back on Linux with msitools.
#
# What lands in <extra-dir>: the icon, the two dialog bitmaps, the note the
# installer adds beside the binary, and msi.env, which carries the version the
# MSI is stamped with, the product and upgrade codes, and how many files the
# stage holds here. The check uses the count windows-msi.ps1 takes of the tree
# WiX actually harvested instead, because the tree crosses to the Windows
# runner as an artifact and the emulators' symlinks arrive as the files they
# point at - a count taken on this side came out short.
set -euo pipefail

stage=${1:?the directory meshbench.exe sits in}
version=${2:?the release version}
extra=${3:?the directory to write into}

here=$(cd "$(dirname "$0")" && pwd)

missing=""
for t in icotool python3; do
  command -v "$t" >/dev/null || missing="$missing $t"
done
if [ -n "$missing" ]; then
  echo "::error::windows-msi: missing:$missing - icotool is 'icoutils'" >&2
  exit 1
fi
# The dialog bitmaps are composed with Pillow, which python3 does not carry by
# itself. Checked here rather than met as a traceback minutes into a release.
python3 -c 'import PIL' 2>/dev/null || {
  echo "::error::windows-msi: python3 cannot import PIL - install python3-pil" >&2
  exit 1
}

# A development build's suffix is not an MSI version, but its base is: the
# pre-release of 0.0.11 installs as 0.0.11, so the MSI ordering it takes part
# in is the one the tag means. A version that is not X.Y.Z at all is 0.0.0.
msiversion=${version%%-*}
grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' <<<"$msiversion" || msiversion=0.0.0

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

mkdir -p "$extra"
# Windows wants one icon file holding every size, the way macOS wants one
# .icns, built from the committed PNGs so there is no second copy of the mark.
icotool -c -o "$extra/meshbench.ico" \
  "$here/icons/meshbench-16.png" "$here/icons/meshbench-32.png" \
  "$here/icons/meshbench-48.png" "$here/icons/meshbench-64.png" \
  "$here/icons/meshbench-128.png" "$here/icons/meshbench-256.png"
# The two bitmaps the dialogs are drawn on; see dialog-bitmaps.py.
python3 "$here/dialog-bitmaps.py" \
  "$here/../docs/brand/meshbench-card.png" \
  "$here/icons/meshbench-256.png" \
  "$extra"
sed "s/<version>/$version/" "$here/installed-by-msi.txt" \
  > "$extra/installed-by-msi.txt"

files=$(find "$stage" -type f | wc -l)
cat > "$extra/msi.env" <<ENV
MSIVERSION=$msiversion
PRODUCT=$product
UPGRADE=$upgrade
FILES=$files
ENV
echo "windows-msi: $files files from $stage, MSI version $msiversion"
