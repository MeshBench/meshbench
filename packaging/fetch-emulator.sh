#!/usr/bin/env bash
# Fetch one pinned emulator asset into a bundle, and decide what its absence
# means.
#
#   packaging/fetch-emulator.sh <owner/repo> <tag> <asset> <label> [dir]
#
# The whole point is the decision, which used to be "|| warn" for every way a
# fetch could fail and is now three different answers:
#
#   no asset named for this platform  the fork publishes no build here. Said
#                                     out loud and carried on past, because the
#                                     bundle is honestly without it
#   the release does not exist        a warning. A release that has not been
#                                     cut yet is a state somebody may be
#                                     building through deliberately
#   the release exists, nothing       a failure. The fork renamed its assets or
#   matches, or the fetch broke       the pin is wrong, and shipping past
#                                     either puts a bundle with no emulator in
#                                     it in front of a user under a green build
#
# Those middle two were one outcome until a fork's newest release renamed every
# asset and dropped every platform but Linux. Nothing failed, and every
# platform shipped without QEMU.
set -euo pipefail

repo=${1:?owner/repo}
tag=${2:?release tag}
asset=${3-}
label=${4:?what a person calls this thing}
dir=${5:-.}

if [ -z "$asset" ]; then
  echo "::notice::no $label is published for this platform; the bundle ships without it, deliberately"
  exit 0
fi

here=$(cd "$(dirname "$0")" && pwd)
# The asset name is exact, so it is anchored to the end of the download URL.
set +e
"$here/../tools/fetch-release-asset.sh" "$repo" "$tag" "/$asset\$" "$dir"
status=$?
set -e

case "$status" in
  0) ;;
  2) echo "::warning::$repo has no release $tag, so the bundle ships without $label" ;;
  *)
    echo "::error::$repo $tag did not yield $asset, so the bundle would ship without $label"
    exit 1
    ;;
esac
