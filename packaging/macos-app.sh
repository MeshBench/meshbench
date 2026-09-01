#!/bin/bash
#
# Build MeshBench.app and a .dmg. Run from the repository root on a Mac:
#
#     packaging/macos-app.sh 0.1.0
#
# This is deliberately a script rather than only workflow YAML, because
# MeshBench's macOS builds run on a Mac we own: hosted macOS minutes bill at
# ten times the rate on a private repository, and this is the same work either
# way. A self-hosted runner calls this; so can a person.
#
# What it does NOT do is notarise. Ad-hoc signing is enough to run on the
# machine that built it, and not enough for anybody else - Gatekeeper will
# refuse a downloaded copy until there is a Developer ID and a notarisation
# step. That needs an Apple account, so it waits for one.
set -euo pipefail

VER="${1:-0.0.0}"
OUT="${OUT:-dist/macos}"
APP="$OUT/MeshBench.app"
ARCH=$(uname -m)

command -v go >/dev/null || { echo "go is not on PATH" >&2; exit 1; }
[ -f go.mod ] || { echo "run this from the repository root" >&2; exit 1; }

rm -rf "$OUT"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

echo "--- binary"
# The CARTO key comes from the workflow's own environment, the same secret the
# Linux and Windows builds are stamped with. Without it a downloaded macOS
# build had no default basemap key at all, so the one platform nobody checked
# was the one shipping without the thing every other platform ships with.
go build -trimpath \
  -ldflags "-X gioui.org/app.ID=io.github.meshbench.meshbench \
            -X github.com/MeshBench/meshbench/internal/world/basemap.defaultCartoKey=${CARTO_API_KEY} \
            -X github.com/MeshBench/meshbench/internal/app/version.Version=v$VER" \
  -o "$APP/Contents/MacOS/meshbench-bin" ./cmd/meshbench

echo "--- icon"
ICONSET=$(mktemp -d)/meshbench.iconset
mkdir -p "$ICONSET"
# The names iconutil insists on, mapped from the committed sizes.
cp packaging/icons/meshbench-16.png  "$ICONSET/icon_16x16.png"
cp packaging/icons/meshbench-32.png  "$ICONSET/icon_16x16@2x.png"
cp packaging/icons/meshbench-32.png  "$ICONSET/icon_32x32.png"
cp packaging/icons/meshbench-64.png  "$ICONSET/icon_32x32@2x.png"
cp packaging/icons/meshbench-128.png "$ICONSET/icon_128x128.png"
cp packaging/icons/meshbench-256.png "$ICONSET/icon_128x128@2x.png"
cp packaging/icons/meshbench-256.png "$ICONSET/icon_256x256.png"
cp packaging/icons/meshbench-512.png "$ICONSET/icon_256x256@2x.png"
cp packaging/icons/meshbench-512.png "$ICONSET/icon_512x512.png"
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/MeshBench.icns"

echo "--- what travels with it"
cp -r fixtures "$APP/Contents/Resources/fixtures"
go run ./tools/licgen -text "$APP/Contents/Resources/LICENCES" >/dev/null

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>MeshBench</string>
  <key>CFBundleDisplayName</key><string>MeshBench</string>
  <key>CFBundleIdentifier</key><string>io.github.meshbench.meshbench</string>
  <key>CFBundleVersion</key><string>$VER</string>
  <key>CFBundleShortVersionString</key><string>$VER</string>
  <key>CFBundleExecutable</key><string>meshbench</string>
  <key>CFBundleIconFile</key><string>MeshBench</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

# Double-clicking the bundle opens the workbench, but the binary is the whole
# CLI - so the bundle's executable is a two-line wrapper that supplies the
# subcommand, rather than a second copy of a 35 MB binary.
cat > "$APP/Contents/MacOS/meshbench" <<'SH'
#!/bin/sh
here=$(dirname "$0")
exec "$here/meshbench-bin" workbench "$@"
SH
chmod +x "$APP/Contents/MacOS/meshbench"

echo "--- sign"
codesign --force --deep --sign - "$APP"
codesign --verify --verbose=2 "$APP"

echo "--- dmg"
hdiutil create -quiet -volname "MeshBench $VER" -srcfolder "$OUT" \
  -ov -format UDZO "dist/MeshBench-$VER-$ARCH.dmg"

ls -la "dist/MeshBench-$VER-$ARCH.dmg"
"$APP/Contents/MacOS/meshbench-bin" workbench -version
