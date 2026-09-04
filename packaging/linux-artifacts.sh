#!/usr/bin/env bash
#
# The three Linux downloads, from one tree.
#
#   packaging/linux-artifacts.sh <tree> <variant> <version> <outdir>
#
# Called twice by the packaging job: once over the application on its own, and
# again over the same tree once the emulators have been unpacked into it. The
# bundled tree is a strict superset of the compact one, so this is one build and
# two packaging passes rather than two of everything.
#
# It is a script rather than forty lines inlined twice, because the second copy
# is where the two would drift - and a bundled AppImage that quietly lost its
# emulators would look exactly like a working one until somebody started a
# board.
set -euo pipefail

tree=${1:?the assembled tree to package}
variant=${2:?bundled or compact}
version=${3:?the version, for the deb}
out=${4:?where the artifacts go}

case "$variant" in
  bundled|compact) ;;
  *) echo "linux-artifacts: variant must be bundled or compact, got $variant" >&2; exit 2 ;;
esac

here=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"

# The tree says what it is, and the application reads it back. Without this the
# updater has only the filename to go on, and once both variants are published
# every suffix match finds two - so a bundled install could be updated into a
# compact one and lose its emulators, silently, on a machine where emulated
# boards had been working.
echo "$variant" > "$tree/VARIANT"

# --- the archive -------------------------------------------------------------
tar czf "$out/meshbench-linux-x86_64-${variant}.tar.gz" -C "$(dirname "$tree")" "$(basename "$tree")"

# --- the AppImage ------------------------------------------------------------
# An AppDir is the tree an installed application would have. linuxdeploy turns
# it into one file that runs on any distribution above the glibc floor.
app="$out/AppDir-$variant"
rm -rf "$app"
mkdir -p "$app/usr/bin" "$app/usr/share/applications" \
         "$app/usr/share/metainfo" "$app/usr/share/meshbench"

# Everything beside the binary travels: the chip model, the emulators where this
# is the bundled pass, and Renode's support files, which it reads at runtime.
cp "$tree/meshbench" "$app/usr/bin/"
cp "$tree/VARIANT" "$app/usr/bin/"
for extra in libvirtualsx1262.so renode-support qemu qemu-meshbench \
             qemu-system-xtensa renode; do
  [ -e "$tree/$extra" ] && cp -r "$tree/$extra" "$app/usr/bin/"
done
# Renode unpacks into a directory carrying its own version, so it cannot be
# listed above.
for d in "$tree"/renode*-portable; do
  [ -e "$d" ] && cp -r "$d" "$app/usr/bin/"
done
cp -r "$tree/fixtures" "$tree/fonts" "$tree/LICENCES" "$app/usr/share/meshbench/"
cp "$here/packaging/meshbench.desktop" "$app/usr/share/applications/"
cp "$here/packaging/io.github.meshbench.meshbench.metainfo.xml" "$app/usr/share/metainfo/"
for px in 16 24 32 48 64 128 256 512; do
  d="$app/usr/share/icons/hicolor/${px}x${px}/apps"
  mkdir -p "$d"
  cp "$here/packaging/icons/meshbench-${px}.png" "$d/io.github.meshbench.meshbench.png"
done

if [ ! -x /tmp/linuxdeploy ]; then
  curl -fsSL -o /tmp/linuxdeploy \
    https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-x86_64.AppImage
  chmod +x /tmp/linuxdeploy
fi
# No FUSE on the runner, so the tools have to unpack themselves.
export APPIMAGE_EXTRACT_AND_RUN=1
if /tmp/linuxdeploy --appdir "$app" --output appimage \
     -d "$here/packaging/meshbench.desktop" \
     -i "$here/packaging/icons/meshbench-256.png"; then
  mv MeshBench*.AppImage "$out/meshbench-x86_64-${variant}.AppImage"
else
  echo "::warning::the $variant AppImage did not build; its tarball still stands"
fi

# --- the deb -----------------------------------------------------------------
# The package name carries the variant, which is why its filename does not have
# to: apt asks the question and the name answers it. Conflicts and Provides mean
# a machine has exactly one and can swap without uninstalling first.
if [ "$variant" = bundled ]; then
  pkg=meshbench-bundled
  extra_control="Conflicts: meshbench
Provides: meshbench
Replaces: meshbench"
else
  pkg=meshbench
  extra_control="Conflicts: meshbench-bundled"
fi

deb="$out/deb-$variant"
rm -rf "$deb"
mkdir -p "$deb/DEBIAN" "$deb/usr/bin" "$deb/usr/share/applications" \
         "$deb/usr/share/metainfo" "$deb/usr/share/meshbench"
# From packaging/, not from the AppDir: linuxdeploy drops its own copy of the
# icon in there under the source file's name, and that stray meshbench-256.png
# would ride into the deb's icon theme where nothing looks for it.
for px in 16 24 32 48 64 128 256 512; do
  d="$deb/usr/share/icons/hicolor/${px}x${px}/apps"
  mkdir -p "$d"
  cp "$here/packaging/icons/meshbench-${px}.png" "$d/io.github.meshbench.meshbench.png"
done
cp -r "$app/usr/bin/." "$deb/usr/bin/"
cp -r "$app/usr/share/meshbench/." "$deb/usr/share/meshbench/"
cp "$here/packaging/meshbench.desktop" "$deb/usr/share/applications/"
cp "$here/packaging/io.github.meshbench.meshbench.metainfo.xml" "$deb/usr/share/metainfo/"

cat > "$deb/DEBIAN/control" <<CONTROL
Package: ${pkg}
Version: ${version}
Section: science
Priority: optional
Architecture: amd64
Maintainer: MeshBench <noreply@github.com>
Depends: libc6 (>= 2.34), libgl1, libx11-6, libxkbcommon0, libwayland-client0, libvulkan1
${extra_control}
Description: Plan and test MeshCore LoRa networks against real firmware
 MeshBench runs real MeshCore firmware against a sample-accurate LoRa
 channel over real terrain, and says what arrived at the antenna and why.
CONTROL
dpkg-deb --build --root-owner-group "$deb" "$out/${pkg}_${version}_amd64.deb"

rm -rf "$app" "$deb"
echo "linux-artifacts: $variant done"
