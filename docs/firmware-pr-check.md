# Firmware PR check

"Did this PR break something?" — answered where a MeshCore contributor is
already looking, without them installing anything.

`.github/actions/meshbench-regression-check` builds MeshCore at a pull
request's own ref, runs MeshBench's regression set (plan §2.4) against it on
real firmware, and comments on the PR only when a scenario diverges. A clean
run collapses to one line; a regression or a flagged scenario expands, with
the reproduce command an author can run locally.

## Why it lives here, not in MeshCore

The workflow runs in a repository MeshBench does not own. It is built to be
adoptable there, not imposed: nothing about it requires MeshCore's
maintainers to install MeshBench, only to add one job that checks this
action out.

## Using it from another repository's workflow

```yaml
jobs:
  meshbench-regression:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          path: meshcore-pr

      - uses: MeshBench/meshbench/.github/actions/meshbench-regression-check@main
        with:
          meshcore-path: meshcore-pr
          regression-dir: meshcore-pr/.meshbench/regressions
          pr-number: ${{ github.event.pull_request.number }}
          github-token: ${{ secrets.GITHUB_TOKEN }}
```

`regression-dir` is a directory of `*.json` regression cases (plan §2.4) -
export one from the Sweep panel's "export from sweep" button, or the
Regressions panel's own directory, and commit it wherever the consuming
repository wants a fixed set to check against. MeshBench does not publish
one on MeshCore's behalf: which scenarios matter is a decision for whoever
adopts the check, not something this action decides for them.

## The cost trade

Firmware build plus dozens of scenarios times however many seeds each case
asks for. More seeds is a more trustworthy result and a longer wait a
reviewer will not tolerate — stated as a trade rather than discovered one
CI minute at a time:

- `timeout-minutes` (default 15) is a hard wall-clock cap on the whole
  regression set, independent of how many scenarios or seeds are in it.
- Each case's own seed count is fixed when it is exported (plan §2.4's own
  tolerance band already assumes a specific spread), so the budget is
  controlled by *which cases* go in the directory, not by a flag here.

## What it does not do

- **It does not gate the merge.** `meshcoresim verify`'s own exit code is
  non-zero on a hard failure, but this action does not fail the job on
  that exit code — a firmware PR check commenting is the whole feature;
  whether a regression blocks a merge is upstream's own call, made with
  their own required-checks configuration, not this action's.
- **It does not run on a schedule or push to `main`.** It is meant to
  trigger on `pull_request`, against the PR's own ref - a nightly run
  against `main` alone answers a different question than this one.
- **A flagged scenario is not a regression.** It is a stochastic metric
  outside the band its case was captured with, reported so a reviewer can
  look, never used to fail anything on its own - the same reasoning
  behind plan §2.4's three-way verdict.
