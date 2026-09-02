#!/usr/bin/env bash
# Every shipped fixture, run against its own assertions.
#
# The assertions embedded in fixtures/*.json are the only machine-checkable
# claim this project makes about whether a mesh still delivers, and nothing ever
# ran them: fixture-fife-strict shipped for months exiting 1 on a clean tree,
# because its three companions were sent their `public hello` as console text
# and a companion has no console. Nobody was going to find that by hand.
#
# A script rather than a dozen lines inlined in the workflow, so that what CI
# runs is the same thing a contributor can run before pushing. The inline
# version could only ever be checked by pushing and watching.
#
# Every fixture in the directory, discovered rather than listed, so a new one is
# gated the day it lands instead of the day somebody remembers to add it here.
# Two are skipped by rule rather than by name:
#
#   - a fixture with an emulated node, whose firmware names a board image. It
#     needs QEMU or Renode and radioserver, which the lab runners do not have
#     (docs/development-machines.md), so it would report a missing toolchain
#     rather than a regression.
#   - a fixture bigger than MAX_NODES, where that is set. A national fixture is
#     376 MeshCore processes and minutes of wall clock, which is a nightly's
#     work rather than something to hold a merge behind.
#
#   tools/fixture-check.sh
#   MESHBENCH=/tmp/meshbench MAX_NODES=100 tools/fixture-check.sh
set -uo pipefail
cd "$(dirname "$0")/.."

BIN="${MESHBENCH:-}"
if [ -z "$BIN" ]; then
  BIN="$(mktemp -d)/meshbench"
  go build -o "$BIN" ./cmd/meshbench || exit 1
fi

# A node keeps its identity and its saved preferences between runs, which is how
# hardware behaves and a trap for a gate: a fixture that passes only because the
# last run left something behind is not a fixture that passes.
export MESHBENCH_NODEFS="${MESHBENCH_NODEFS:-$(mktemp -d)/nodefs}"

# What the machine had while each fixture ran.
#
# The nightly has twice died part way through this sweep with "the runner has
# received a shutdown signal" and nothing else - the job is killed, so its own
# logs go with it, and what is left says only that the run stopped. A national
# fixture is 376 MeshCore processes at once, so the number that tells a mesh
# that stopped delivering from a machine that ran out is the least memory that
# was available while it ran. One line per fixture is enough to tell them
# apart, and it costs a background loop reading /proc.
#
# Linux only, and quiet where there is no /proc: this script is run by hand on
# machines that have neither.
mem_available_mb() { awk '/^MemAvailable:/{print int($2/1024)}' /proc/meminfo; }

# The baseline is written by the caller before this is backgrounded, so a
# fixture that ends quickly cannot outrun the first sample and leave the line
# blank.
watch_memory() {
  local out=$1 low cur
  low=$(cat "$out")
  while :; do
    cur=$(mem_available_mb 2>/dev/null) || return
    if [ -n "$cur" ] && [ "$cur" -lt "$low" ]; then
      low=$cur
      printf '%s' "$low" > "$out"
    fi
    sleep 2
  done
}

fail=0
for f in fixtures/fixture-*.json; do
  why=$(python3 -c "
import json, os, sys
nodes = json.load(open(sys.argv[1])).get('nodes') or []
cap = int(os.environ.get('MAX_NODES') or 0)
if any((n.get('Firmware') or {}).get('Board') for n in nodes):
    print('it has an emulated node and this runner has no emulator')
elif cap and len(nodes) > cap:
    print(f'{len(nodes)} nodes is over the {cap} this run allows')
" "$f")
  if [ -n "$why" ]; then
    echo "== $f: skipped, $why"
    continue
  fi
  echo "== $f"
  watcher=""
  low_file=""
  if [ -r /proc/meminfo ]; then
    low_file=$(mktemp)
    mem_available_mb > "$low_file"
    watch_memory "$low_file" &
    watcher=$!
  fi
  if ! "$BIN" test -fixture "$f"; then
    fail=1
  fi
  if [ -n "$watcher" ]; then
    kill "$watcher" 2>/dev/null
    wait "$watcher" 2>/dev/null
    echo "   least memory available while that ran: $(cat "$low_file") MB"
    rm -f "$low_file"
  fi
done

if [ "$fail" -ne 0 ]; then
  echo
  echo "a shipped fixture does not do what it claims to do"
  exit 1
fi
