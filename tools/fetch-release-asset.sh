#!/usr/bin/env bash
# Download one asset from a public GitHub release, using curl.
#
#   tools/fetch-release-asset.sh <owner/repo> <tag|latest> <pattern> <dir>
#
# gh would be shorter, and this used to use it. gh is not part of a bare Ubuntu
# image and is absent from at least one of the runners this has to work on,
# where its absence surfaced eight steps later as "no radioserver-v1 release" -
# naming a release that exists. curl is already used in the same job for the
# emoji font, so this depends on nothing that was not already required.
#
# The pattern is an extended regular expression matched against the asset name,
# not a glob, because that is what selects one build out of a release carrying
# several targets.
#
# GH_TOKEN is used when set, for the rate limit rather than for access: all
# three repositories this fetches from are public.
set -euo pipefail

repo=${1:?owner/repo}
tag=${2:?tag, or "latest"}
pattern=${3:?asset name pattern}
dir=${4:-.}

if [ "$tag" = "latest" ]; then
  api="https://api.github.com/repos/$repo/releases/latest"
else
  api="https://api.github.com/repos/$repo/releases/tags/$tag"
fi

auth=()
[ -n "${GH_TOKEN:-}" ] && auth=(-H "Authorization: Bearer $GH_TOKEN")

release=$(curl -fsSL -H "Accept: application/vnd.github+json" "${auth[@]}" "$api") || {
  echo "fetch-release-asset: $repo has no release $tag (or the API is unreachable)" >&2
  exit 1
}

# One URL per asset, then the pattern picks the build. Parsed with grep rather
# than jq so this needs nothing beyond curl.
url=$(printf '%s' "$release" \
  | grep -o '"browser_download_url": *"[^"]*"' \
  | sed 's/.*"browser_download_url": *"//; s/"$//' \
  | grep -E "$pattern" | head -1) || true

if [ -z "$url" ]; then
  echo "fetch-release-asset: $repo $tag has no asset matching /$pattern/" >&2
  echo "  it carries:" >&2
  printf '%s' "$release" | grep -o '"browser_download_url": *"[^"]*"' \
    | sed 's/.*\///; s/"$//; s/^/    /' >&2
  exit 1
fi

mkdir -p "$dir"
out="$dir/$(basename "$url")"
curl -fsSL "${auth[@]}" -o "$out" "$url"
echo "$out"
