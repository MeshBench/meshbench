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
# No Windows runner. wixl reads the WiX source and writes the .msi here, beside
# the mingw cross-build that produced the .exe. What that costs is the
# installer's user interface: wixl builds no dialogs, so msiexec shows a
# progress bar and takes its answers from the command line instead. Those
# answers - a location, per-user, a desktop shortcut - are written out for
# somebody installing this in docs/install.md.
set -euo pipefail

stage=${1:?the directory meshbench.exe sits in}
version=${2:?the release version}
out=${3:?the .msi to write}

here=$(cd "$(dirname "$0")" && pwd)

missing=""
for t in wixl wixl-heat msiinfo msibuild icotool python3; do
  command -v "$t" >/dev/null || missing="$missing $t"
done
if [ -n "$missing" ]; then
  echo "::error::windows-msi: missing:$missing - wixl and wixl-heat are the" \
       "'wixl' package on Debian and Ubuntu rather than 'msitools', which is" \
       "where msiinfo lives; icotool is 'icoutils'" >&2
  exit 1
fi

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

# wixl reads every File Source relative to the directory it is run from, and
# hands an absolute one to g_file_get_child, which rejects it with a GLib
# assertion and then says it cannot find the file. So the two source trees are
# linked into one working directory and named relatively from there.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
extra=$work/extra
mkdir -p "$extra"
ln -s "$(cd "$stage" && pwd)" "$work/stage"
out=$(cd "$(dirname "$out")" && pwd)/$(basename "$out")

# Windows wants one icon file holding every size, the way macOS wants one
# .icns. Built here from the committed PNGs for the same reason macos-app.sh
# builds the .icns there: one set of source images, and no second copy of the
# mark to go stale.
icotool -c -o "$extra/meshbench.ico" \
  "$here/icons/meshbench-16.png" "$here/icons/meshbench-32.png" \
  "$here/icons/meshbench-48.png" "$here/icons/meshbench-64.png" \
  "$here/icons/meshbench-128.png" "$here/icons/meshbench-256.png"

sed "s/<version>/$version/" "$here/installed-by-msi.txt" \
  > "$extra/installed-by-msi.txt"

# Everything in the bundle, as components. Sorted, so the source that goes
# into a build is the same from one run to the next and a diff of two builds
# is about what changed rather than about what order find walked in.
(cd "$stage" && find . -type f | sed 's|^\./||' | LC_ALL=C sort) |
  wixl-heat --var var.Stage --directory-ref INSTALLDIR \
    --component-group Bundle --prefix "" --win64 > "$work/bundle.wxs"

files=$(grep -c '<File ' "$work/bundle.wxs")
echo "windows-msi: $files files from $stage"

# wixl-heat names each directory with a fresh random id every run, and wixl
# derives each component's GUID from that, so the same tree built twice comes
# out as two different installers. Renumbered here in the order they appear -
# which the sorted harvest above makes stable - so a rebuild of a version
# produces the same package, and two builds can be compared for what actually
# changed. Only the directory ids are touched; the component and file ids are
# already hashes of the paths.
python3 - "$work/bundle.wxs" <<'PY'
import re
import sys

path = sys.argv[1]
seen = {}
def stable(m):
    return seen.setdefault(m.group(0), "dir%04d" % (len(seen) + 1))
with open(path) as f:
    src = f.read()
with open(path, "w") as f:
    f.write(re.sub(r"dir[0-9A-F]{32}", stable, src))
PY

(cd "$work" && wixl -a x64 -o "$out" \
  -D "ProductCode=$product" -D "Version=$msiversion" \
  -D Stage=stage -D Extra=extra -D Win64=yes \
  "$here/meshbench.wxs" bundle.wxs)

# A public property set on the msiexec command line does not survive the
# elevation a per-machine install goes through unless SecureCustomProperties
# names it, and INSTALLDIR is the one this package offers. wixl writes that row
# itself, for the upgrade properties, and refuses a second row with the same
# key - so the row it wrote is extended here rather than authored in the .wxs.
# tr, because msiinfo writes the IDT form with DOS line endings and a carriage
# return would ride into the property Windows then reads.
secure=$(msiinfo export "$out" Property | tr -d '\r' |
  awk -F'\t' '$1 == "SecureCustomProperties" { print $2 }')
msibuild "$out" -q "UPDATE \`Property\` SET \`Value\`='$secure;INSTALLDIR;MSIINSTALLPERUSER'
  WHERE \`Property\`='SecureCustomProperties'"

# Apps and Features shows an "Install location", and ours was blank: nothing
# sets ARPINSTALLLOCATION, so Windows has nowhere to read the path from and
# somebody who wants to know where their copy went has to guess. It cannot be
# a plain Property in the .wxs - [INSTALLDIR] is not resolved until the install
# is costed - so it is a type 51 custom action, which sets a property from a
# formatted string, scheduled once costing has settled.
#
# Authored here rather than in the source for the same reason as the row above:
# this is one of the shapes wixl does not build. The table may not exist at all
# in a package with no other custom action, so it is created when it is missing.
if ! msiinfo tables "$out" | tr -d '\r' | grep -qx CustomAction; then
  msibuild "$out" -q "CREATE TABLE \`CustomAction\` (\`Action\` CHAR(72) NOT NULL, \`Type\` INT NOT NULL, \`Source\` CHAR(72), \`Target\` CHAR(255) PRIMARY KEY \`Action\`)"
fi
msibuild "$out" -q "INSERT INTO \`CustomAction\` (\`Action\`, \`Type\`, \`Source\`, \`Target\`) VALUES ('SetInstallLocation', 51, 'ARPINSTALLLOCATION', '[INSTALLDIR]')"

# After CostFinalize, which is where INSTALLDIR stops being a guess. Read
# rather than assumed: the standard action sits at 1000 in a package nobody has
# reordered, and this is not the place to find out that somebody did.
cost=$(msiinfo export "$out" InstallExecuteSequence | tr -d '\r' |
  awk -F'\t' '$1 == "CostFinalize" { print $3 }')
[ -n "$cost" ] || {
  echo "::error::windows-msi: no CostFinalize to schedule the install location after" >&2
  exit 1
}
msibuild "$out" -q "INSERT INTO \`InstallExecuteSequence\` (\`Action\`, \`Sequence\`) VALUES ('SetInstallLocation', $((cost + 1)))"


# Plus the note, which is the one file the installer adds to the bundle.
"$here/verify-msi.sh" "$out" "$msiversion" "$upgrade" "$((files + 1))"
