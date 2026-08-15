#!/usr/bin/env bash
# Post or update the regression-check comment on one PR.
#
# Found by the marker internal/regression.Comment writes into every comment
# body, not by author - a bot account rename must not turn "update in
# place" into "pile up a new comment on every push", which is the exact
# noise this whole feature exists to avoid.
set -euo pipefail

repo=${1:?usage: post-comment.sh <owner/repo> <pr-number> <comment-body-path>}
pr=${2:?}
body_path=${3:?}
marker='<!-- meshbench-regression-check -->'

[ -f "$body_path" ] || { echo "no comment body at $body_path" >&2; exit 1; }

existing_id=$(gh api "repos/$repo/issues/$pr/comments" --paginate \
  --jq "[.[] | select(.body | startswith(\"$marker\"))][0].id // empty")

if [ -n "$existing_id" ]; then
  echo "updating comment $existing_id"
  gh api "repos/$repo/issues/comments/$existing_id" -X PATCH \
    -f body="$(cat "$body_path")" >/dev/null
else
  echo "posting a new comment on #$pr"
  gh api "repos/$repo/issues/$pr/comments" -X POST \
    -f body="$(cat "$body_path")" >/dev/null
fi
