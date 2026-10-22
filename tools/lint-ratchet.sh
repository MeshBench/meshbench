#!/usr/bin/env bash
# The ratchet: golangci-lint's count may fall, never rise.
#
# 477 findings appeared the first time the full linter set was run here. Turning
# that into a red build would have meant 477 unreviewed changes to a codebase
# whose selling point is that its numbers can be trusted. So the count is held
# at a baseline instead, and the baseline only ever goes down - a new finding
# fails the build on the pull request that introduces it, while the backlog is
# cleared deliberately, one class at a time.
#
#   tools/lint-ratchet.sh            compare against the baseline
#   tools/lint-ratchet.sh --update   rewrite it, after clearing something
set -uo pipefail
cd "$(dirname "$0")/.."

BASELINE=.golangci-baseline.txt
LINT=${GOLANGCI_LINT:-golangci-lint}

# A failed tool is not a clean tree. The old pipeline discarded the exit
# status and treated a config error, timeout or missing binary as zero counts;
# --update could then erase the entire baseline after lint had checked nothing.
output=$("$LINT" run 2>&1)
status=$?
if [ "$status" -ne 0 ] && [ "$status" -ne 1 ]; then
  printf '%s\n' "$output"
  echo "lint did not complete (exit $status); baseline unchanged"
  exit 1
fi

current=$(printf '%s\n' "$output" | awk '/^\* [a-z][a-z0-9]*: [0-9]+$/ {
  sub(/:$/, "", $2); print $2, $3
}' | sort)
total=$(printf '%s\n' "$output" | sed -nE 's/^([0-9]+) issues[.:]$/\1/p')
if ! [[ "$total" =~ ^[0-9]+$ ]]; then
  printf '%s\n' "$output"
  echo "unrecognised lint output; baseline unchanged"
  exit 1
fi
sum=$(printf '%s\n' "$current" | awk '{n += $2} END {print n+0}')
if [ "$sum" -ne "$total" ] ||
   { [ "$total" -eq 0 ] && [ "$status" -ne 0 ]; } ||
   { [ "$total" -gt 0 ] && [ "$status" -ne 1 ]; }; then
  printf '%s\n' "$output"
  echo "inconsistent lint summary; baseline unchanged"
  exit 1
fi

if [ "${1:-}" = "--update" ]; then
  printf '%s\n' "$current" > "$BASELINE"
  echo "baseline updated:"; cat "$BASELINE"; exit 0
fi

if [ ! -r "$BASELINE" ] || ! awk '
  NF == 0 {next}
  NF != 2 || $1 !~ /^[a-z][a-z0-9]*$/ || $2 !~ /^[0-9]+$/ || seen[$1]++ {exit 1}
' "$BASELINE"; then
  echo "invalid or missing lint baseline"
  exit 1
fi

fail=0
while read -r linter count; do
  [ -z "$linter" ] && continue
  was=$(awk -v l="$linter" '$1==l {print $2}' "$BASELINE")
  was=${was:-0}
  if [ "$count" -gt "$was" ]; then
    echo "RISEN  $linter: $was -> $count"
    fail=1
  elif [ "$count" -lt "$was" ]; then
    echo "fallen $linter: $was -> $count   (run tools/lint-ratchet.sh --update)"
  fi
done <<< "$current"

# A linter that has gone silent entirely still has to be recorded, or the
# baseline keeps a number nothing can ever reach and the ratchet stops biting.
while read -r linter was; do
  [ -z "$linter" ] && continue
  if ! grep -q "^$linter " <<< "$current"; then
    echo "cleared $linter: $was -> 0   (run tools/lint-ratchet.sh --update)"
  fi
done < "$BASELINE"

if [ "$fail" = 1 ]; then
  echo
  echo "New lint findings. Fix them, or if they are genuinely acceptable, say why"
  echo "at the call site with a //nolint comment carrying a reason."
  exit 1
fi
echo "lint ratchet holds"
