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
#
# The exit codes are the point of this script, because the ways it fails are
# not one fact and a caller that reads them as one ships a broken bundle:
#
#   0  the asset is in <dir>, and its path is on stdout
#   1  the request could not be made, or the download failed: no network, a
#      rate limit, a 5xx. Nothing is known about the release
#   2  the repository has no such release. A release that has not been cut yet
#      is a state somebody may legitimately choose to carry on past
#   3  the release exists and carries no asset matching the pattern. Either the
#      fork renamed its assets or the pattern is wrong, and both are bugs
#
# 2 and 3 were one exit code until a fork's newest release renamed every asset
# and dropped every platform but Linux. Every pattern stopped matching, every
# caller's "|| warn" swallowed it, and three platforms' bundles shipped with no
# emulator in them under a green pipeline.
set -euo pipefail

repo=${1:?owner/repo}
tag=${2:?tag, or "latest"}
pattern=${3:?asset name pattern}
dir=${4:-.}

# GITHUB_API_URL is already set to this by Actions itself, so naming it here
# costs nothing on a runner and lets the two failure states be exercised
# against a stub: GitHub cannot be asked to return a 404 on demand.
root=${GITHUB_API_URL:-https://api.github.com}

if [ "$tag" = "latest" ]; then
  api="$root/repos/$repo/releases/latest"
else
  api="$root/repos/$repo/releases/tags/$tag"
fi

auth=()
if [ -n "${GH_TOKEN:-}" ]; then
  auth=(-H "Authorization: Bearer $GH_TOKEN")
fi
# ${auth[@]+"${auth[@]}"} rather than "${auth[@]}" at the two call sites below:
# an empty array expanded under set -u is an unbound variable in the bash the
# macOS runner ships, which is 3.2, and this is the spelling that survives it.

body=$(mktemp "${TMPDIR:-/tmp}/fetch-release-asset.XXXXXX")
trap 'rm -f "$body"' EXIT

# The status separately from the body, because a 404 and a 500 mean opposite
# things: one says the release is not there, the other says we never found out.
code=$(curl -sSL -o "$body" -w '%{http_code}' \
  -H "Accept: application/vnd.github+json" ${auth[@]+"${auth[@]}"} "$api") || code=000

case "$code" in
  200) ;;
  404)
    echo "fetch-release-asset: $repo has no release $tag" >&2
    exit 2
    ;;
  *)
    echo "fetch-release-asset: asking $repo for release $tag returned HTTP $code" >&2
    exit 1
    ;;
esac

# One URL per asset, then the pattern picks the build. Parsed with grep rather
# than jq so this needs nothing beyond curl.
urls=$(grep -o '"browser_download_url": *"[^"]*"' "$body" \
  | sed 's/.*"browser_download_url": *"//; s/"$//') || true
url=$(printf '%s\n' "$urls" | grep -E "$pattern" | head -1) || true

if [ -z "$url" ]; then
  echo "fetch-release-asset: $repo $tag exists and has no asset matching $pattern" >&2
  if [ -z "$urls" ]; then
    echo "  it carries no assets at all" >&2
  else
    echo "  it carries:" >&2
    printf '%s\n' "$urls" | sed 's/.*\///; s/^/    /' >&2
  fi
  exit 3
fi

mkdir -p "$dir"
out="$dir/$(basename "$url")"
curl -fsSL ${auth[@]+"${auth[@]}"} -o "$out" "$url" || {
  echo "fetch-release-asset: downloading $url failed" >&2
  exit 1
}
echo "$out"
