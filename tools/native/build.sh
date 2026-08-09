#!/usr/bin/env bash
# Builds the native node: real MeshCore compiled for this host.
#
# The output links MeshCore, so it is distributed from a separate repository
# under MeshCore's own MIT licence — not from this one (ADR-0020). What this
# script produces locally is for development and for the emulated/native
# cross-check (MSIM-40).
#
#   MESHCORE=path/to/MeshCore CRYPTO=path/to/arduinolibs/libraries/Crypto \
#     tools/native/build.sh [outdir]
set -euo pipefail

: "${MESHCORE:?set MESHCORE to a MeshCore checkout}"
: "${CRYPTO:?set CRYPTO to arduinolibs/libraries/Crypto}"

root=$(cd "$(dirname "$0")/../.." && pwd)
shim="$root/internal/firmware/shim"
out=${1:-"$root/build/native"}
mkdir -p "$out/obj"

case "$(uname -s)" in
  Linux)  goos=linux ;;
  Darwin) goos=darwin ;;
  *) echo "native node is POSIX-only: it uses BSD sockets directly" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64) goarch=amd64 ;;
  arm64|aarch64) goarch=arm64 ;;
  *) echo "unmapped architecture $(uname -m)" >&2; exit 1 ;;
esac
bin="$out/meshcore-node-$goos-$goarch"

inc=(-I "$MESHCORE/src" -I "$shim" -I "$CRYPTO" -I "$MESHCORE/lib/ed25519")
# -O2, not -Os: this build exists to be fast. It is also the build whose results
# get compared against the emulated one, and optimisation level is exactly the
# kind of difference that would make that comparison meaningless if it drifted —
# so it is pinned here rather than left to whoever runs the script.
cxxflags=(-std=c++17 -O2 -Wall -Wno-unused-parameter)

# Third-party sources are cached by mtime; ours are always rebuilt.
#
# Not a micro-optimisation — an mtime check on our own files is actively wrong.
# These get rsync'd to test machines with -a, which preserves the source's
# timestamp, so an edited file routinely arrives *older* than the object built
# from its predecessor. The result is a binary that silently does not contain
# the change you are testing, which costs far more than the twenty seconds
# recompiling two files saves.
compile() { # compile <compiler> <flags...> -- <sources...>
  local cc=$1; shift
  local flags=()
  while [ "$1" != "--" ]; do flags+=("$1"); shift; done
  shift
  for f in "$@"; do
    o="$out/obj/$(basename "${f%.*}").o"
    if [ ! -f "$o" ] || [ "$f" -nt "$o" ]; then "$cc" "${flags[@]}" -c "$f" -o "$o"; fi
  done
}

compile g++ "${cxxflags[@]}" "${inc[@]}" -- \
  "$MESHCORE"/src/{Utils,Packet,Identity,Mesh,Dispatcher}.cpp \
  "$CRYPTO"/{SHA256,AES128,AESCommon,BlockCipher,Crypto,Ed25519,BigNumberUtil,Curve25519,SHA512,Hash}.cpp
compile gcc -std=c11 -O2 -I "$MESHCORE/lib/ed25519" -- "$MESHCORE"/lib/ed25519/*.c

for f in "$shim/HostRNG.cpp" "$shim/native_main.cpp"; do
  g++ "${cxxflags[@]}" "${inc[@]}" -c "$f" -o "$out/obj/$(basename "${f%.*}").o"
done

g++ -o "$bin" "$out"/obj/*.o
echo "$bin"
