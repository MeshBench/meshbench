#!/usr/bin/env bash
# Assert that an assembled bundle contains the emulators it claims to.
#
#   packaging/verify-bundle.sh <dir> <linux-amd64|darwin-arm64|windows-amd64>
#
# <dir> is the directory MeshBench's own binary sits in, because that is where
# a node looks: beside the executable, then the emulators' own unpacked
# layouts under it. The search below is the same one internal/firmware/emulated
# does, deliberately, so this answers "would a node find it" rather than "is
# there a file somewhere in the tree".
#
# What is required comes from packaging/emulator-pins.env and not from a list
# here, so a platform the forks publish no emulator for is not asserted into
# existence, and the day one is published the assertion arrives with the pin.
#
# The failure this exists to stop: 0.0.1 through 0.0.3 shipped two emulators
# and no radio model, then a fork renamed its assets and three platforms
# shipped no emulators at all. Both were green builds. A bundle that quietly
# lacks an emulator is the thing to design out, not to document.
set -euo pipefail

dir=${1:?the directory the MeshBench binary sits in}
platform=${2:?linux-amd64, darwin-arm64 or windows-amd64}

here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=packaging/emulator-pins.env
. "$here/emulator-pins.env"

case "$platform" in
  linux-amd64)   qemu=$QEMU_ASSET_LINUX_AMD64;  renode=$RENODE_ASSET_LINUX_AMD64;  radio=$RADIOSERVER_ASSET_LINUX_AMD64 ;;
  darwin-arm64)  qemu=$QEMU_ASSET_DARWIN_ARM64; renode=$RENODE_ASSET_DARWIN_ARM64; radio=$RADIOSERVER_ASSET_DARWIN_ARM64 ;;
  windows-amd64) qemu=$QEMU_ASSET_WINDOWS_AMD64; renode=$RENODE_ASSET_WINDOWS_AMD64; radio=$RADIOSERVER_ASSET_WINDOWS_AMD64 ;;
  *) echo "verify-bundle: unknown platform $platform" >&2; exit 2 ;;
esac

# find_tool prints where a node starting from <dir> would find a binary.
#
# A zip cannot carry the symlink the tarball and the app bundle use, so the
# emulators' own unpacked directories are searched too. Renode's carries its
# version, so it is matched on shape.
find_tool() {
  local name=$1 sub cand p
  for sub in "" qemu/bin qemu-meshbench/bin $(cd "$dir" 2>/dev/null && ls -d renode*-portable 2>/dev/null || true); do
    for cand in "$name" "$name.exe"; do
      p=$dir${sub:+/$sub}/$cand
      # -e, so a symlink that points at nothing fails here rather than at a
      # user's first emulated board.
      [ -e "$p" ] && { echo "$p"; return 0; }
    done
  done
  return 1
}

fail=0
require() {
  local name=$1 asset=$2 why=$3 found
  if [ -z "$asset" ]; then
    echo "verify-bundle: no $name is published for $platform, and the bundle does not claim one"
    return 0
  fi
  if found=$(find_tool "$name"); then
    echo "verify-bundle: $name -> $found ($(wc -c <"$found" | tr -d ' ') bytes)"
    return 0
  fi
  echo "::error::$platform bundle has no $name, so $why. Expected $asset from the pinned release" >&2
  fail=1
}

require radioserver "$radio" "no emulated board can start at all"
require qemu-system-xtensa "$qemu" "ESP32 boards cannot emulate"
require renode "$renode" "nRF52 boards cannot emulate"

if [ "$fail" -ne 0 ]; then
  echo "verify-bundle: $dir is not shippable" >&2
  exit 1
fi
echo "verify-bundle: $platform bundle carries everything it claims"
