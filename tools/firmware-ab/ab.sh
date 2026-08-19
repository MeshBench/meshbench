#!/usr/bin/env bash
#
# Compare two builds of MeshCore on the same network.
#
#   ab.sh build <arm> <branch>     build one arm from a local MeshCore checkout
#   ab.sh run   <arm> [seeds]      run it and print the numbers
#   ab.sh both  <arm> <branch>     build then run
#
# An "arm" is a named firmware build. The point of the tool is that everything
# except the firmware is held identical between arms, and three things that
# quietly break that are handled here rather than left to whoever is running it.
#
# 1. SAVED NODE STATE BEATS A COMPILED DEFAULT.
#    A node keeps its preferences between runs, as hardware does. A node that
#    has run before loads its old value and never reaches the changed default,
#    so both arms return identical numbers and the change looks inert. It fails
#    silently and in both arms, which is the worst way for a comparison to fail.
#    Every arm therefore gets its own storage through MESHCORESIM_NODEFS.
#
# 2. THE TEST CACHE REPLAYS THE PREVIOUS ARM.
#    Go caches a result against the package, its inputs and the environment
#    variables the test reads - not against the contents of a binary that an
#    environment variable merely points at. Rebuilding an arm and running it
#    again returns the previous arm's numbers. Hence -count=1.
#
# 3. AN ARM NEEDS EVERY ROLE, NOT JUST THE ONE IT CHANGES.
#    MESHCORESIM_NATIVE naming a directory requires a build per role in it. A
#    scenario with companions or room servers fails to resolve otherwise. The
#    roles an arm does not change are copied in from the release cache, so only
#    one thing differs.
#
set -euo pipefail

MESHCORE="${MESHCORE:-$HOME/msim/MeshCore}"
NATIVE="${NATIVE:-$HOME/msim/meshcore-native}"
CRYPTO="${CRYPTO:-$HOME/msim/arduinolibs/libraries/Crypto}"
ARMS="${ARMS:-$HOME/msim/study}"
CACHE="${CACHE:-$HOME/.cache/meshcoresim/firmware/native}"
SIM="${SIM:-$HOME/Documents/projects/meshcoresim}"

usage() { sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

build() {
  local arm="$1" branch="$2"
  local out="$ARMS/$arm"
  mkdir -p "$out"

  git -C "$MESHCORE" checkout -q "$branch"
  echo "arm $arm: $(git -C "$MESHCORE" log --oneline -1)"

  export PATH="$HOME/.platformio/packages/toolchain-gccarmnoneeabi/bin:$PATH"
  MESHCORE="$MESHCORE" CRYPTO="$CRYPTO" "$NATIVE/build.sh" simple_repeater "$out" >/dev/null

  # The roles this arm does not change, so only one variable differs.
  cp -f "$CACHE/companion-v1.17.0/meshcore-companion_radio-linux-amd64" "$out/" 2>/dev/null || true
  cp -f "$CACHE/room-server-v1.17.0/meshcore-simple_room_server-linux-amd64" "$out/" 2>/dev/null || true

  echo "  built: $(ls "$out"/meshcore-* | wc -l) role binaries"
}

run() {
  local arm="$1" seeds="${2:-1,2,3}"
  local out="$ARMS/$arm"
  [ -d "$out" ] || { echo "no such arm: $arm" >&2; exit 1; }

  # Storage per arm, so every node in every arm has never run before.
  local nodefs="$ARMS/.nodefs/$arm"
  rm -rf "$nodefs"; mkdir -p "$nodefs"

  cd "$SIM"
  env MESHCORESIM_LIVE=1 \
      MESHCORESIM_NATIVE="$out" \
      MESHCORESIM_NODEFS="$nodefs" \
      STUDY_SEEDS="$seeds" \
      ${STUDY_SCENARIO:+STUDY_SCENARIO="$STUDY_SCENARIO"} \
    go test -count=1 ./internal/sim/engine/ -run TestStudyArm -v -timeout 1800s 2>&1 \
    | grep -E "loaded |seed |STUDY_RESULT|FAIL" || true
}

case "${1:-}" in
  build) shift; build "$@" ;;
  run)   shift; run "$@" ;;
  both)  shift; build "$1" "$2"; run "$1" "${3:-1,2,3}" ;;
  *)     usage ;;
esac
