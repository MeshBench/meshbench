#!/usr/bin/env bash
# File length, the mechanical half of CLAUDE.md's limits table: 300 lines soft,
# 500 hard.
#
# A script rather than a dozen lines inlined in the workflow, so that what CI
# runs is the same thing a contributor can run before pushing. The inline
# version could only ever be checked by pushing and watching.
#
# Applied to every source language in the tree, not just Go: hamreach enforced
# Go only and its largest file reached 746 lines before anyone noticed. The C
# and C++ are the host shims MeshCore is compiled against, which is real source
# that ships in the release archive.
#
# A file carrying lint:file-length-exempt passes the hard limit and says why in
# its own header. There is one, the Wireshark dissector, because Wireshark loads
# a dissector as a single plugin file.
#
# The soft limit is reported and never fails. It is a trend, not a gate: the
# useful thing is seeing the count move, and a build that went red at 301 lines
# would only teach people to write 299-line files.
#
#   tools/file-length.sh
set -uo pipefail
cd "$(dirname "$0")/.."

HARD=500
SOFT=300

fail=0
over_soft=()

while IFS= read -r f; do
  n=$(wc -l < "$f")
  if [ "$n" -gt "$HARD" ]; then
    if grep -q "lint:file-length-exempt" "$f"; then
      continue
    fi
    echo "$f is $n lines (limit $HARD, no exemption)"
    fail=1
  elif [ "$n" -gt "$SOFT" ]; then
    over_soft+=("$n $f")
  fi
done < <(find . \( -name '*.go' -o -name '*.wgsl' -o -name '*.lua' \
                -o -name '*.c' -o -name '*.cpp' -o -name '*.h' \) \
              -not -path './vendor/*' -not -path './.git/*' | sort)

if [ "$fail" -eq 0 ]; then
  echo "no file exceeds the $HARD-line hard limit"
fi

echo
echo "${#over_soft[@]} files over the ${SOFT}-line soft limit, largest first:"
printf '%s\n' "${over_soft[@]}" | sort -rn | head -15 | while read -r n f; do
  printf '  %5d  %s\n' "$n" "$f"
done

exit $fail
